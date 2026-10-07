package anthropic

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"
	"net/http"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const defaultBatchResultsMaxBytes int64 = 64 * 1024 * 1024

type batchResultsSource struct {
	body   io.ReadCloser
	status int
}

func (s *batchResultsSource) observe(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	response, err := next(request)
	if err == nil && response != nil {
		s.body, s.status = response.Body, response.StatusCode
	}
	return response, err
}

func (b *MessageBatches) resultBindings(
	handle prompty.BatchHandle,
	options prompty.BatchResultsOptions,
) (map[string]*adapter.PreparedBatchResultItem, error) {
	if len(options.Bindings) != len(handle.Items) {
		return nil, prompty.ErrBatchScope
	}
	bindings := make(map[string]*prompty.PromptExecution)
	for _, binding := range options.Bindings {
		if bindings[binding.ID] != nil || binding.Execution == nil {
			return nil, prompty.ErrBatchScope
		}
		bindings[binding.ID] = binding.Execution.Clone()
	}
	prepared := make(map[string]*adapter.PreparedBatchResultItem)
	for _, binding := range handle.Items {
		exec := bindings[binding.ID]
		if exec == nil {
			return nil, prompty.ErrBatchScope
		}
		working, err := b.itemExecution(exec, prompty.CapabilityBatchResults)
		if err != nil {
			return nil, err
		}
		exec.ModelOptions = working.ModelOptions
		digest, err := prompty.BatchExecutionDigest(exec)
		if err != nil {
			return nil, err
		}
		if digest != binding.ExecutionDigest {
			return nil, prompty.ErrBatchScope
		}
		item, err := adapter.PrepareBatchResultItem(b.base, working)
		if err != nil {
			return nil, err
		}
		if item.Diagnostics.Profile != binding.Identity {
			return nil, prompty.ErrBatchScope
		}
		prepared[binding.ID] = item
	}
	return prepared, nil
}

func (b *MessageBatches) Results(
	ctx context.Context,
	handle prompty.BatchHandle,
	options prompty.BatchResultsOptions,
) (*prompty.BatchResult, error) {
	handle = prompty.CloneBatchHandle(handle)
	reports, err := b.operationReport(handle, prompty.CapabilityBatchResults)
	if err != nil {
		return nil, err
	}
	if handle.Status != prompty.BatchEnded {
		return nil, prompty.ErrBatchUnavailable
	}
	prepared, err := b.resultBindings(handle, options)
	if err != nil {
		return nil, err
	}
	if options.MaxLineBytes < 0 || options.MaxBytes < 0 || options.MaxBytes == math.MaxInt64 {
		return nil, prompty.ErrBatch
	}
	lineLimit, totalLimit := options.MaxLineBytes, options.MaxBytes
	if lineLimit == 0 {
		lineLimit = batchMetadataMaxBytes
	}
	if totalLimit == 0 {
		totalLimit = defaultBatchResultsMaxBytes
	}
	result := &prompty.BatchResult{Handle: &handle, Diagnostics: reports}
	capture, err := adapter.NewBatchCapture(
		"anthropic.batch.results",
		"github.com/anthropics/anthropic-sdk-go",
		handle.Items[0].Identity,
		options.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, handle, map[string]string{"message_batch_id": handle.ProviderID}); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	capture.MarkDispatched()
	source := new(batchResultsSource)
	var params sdk.MessageBatchResultsParams
	stream := b.base.client.Messages.Batches.ResultsStreaming(
		ctx,
		handle.ProviderID,
		params,
		option.WithMaxRetries(0),
		option.WithMiddleware(source.observe),
	)
	if err = stream.Err(); err != nil || source.body == nil {
		providerErr := capture.Provider(ctx, nil, source.status, false)
		return capture.Finish(ctx, result, errors.Join(prompty.ErrBatchUnavailable, ctx.Err(), providerErr))
	}
	// Do not use SDK Next/Err: its JSONL Err omits scanner.Err, accepting truncation.
	// SDK dispatch/auth remain unchanged; own bounded source decoder sees actual EOF.
	reader := bufio.NewReaderSize(io.LimitReader(source.body, totalLimit+1), min(lineLimit, batchMetadataMaxBytes))
	bindings := make(map[string]prompty.BatchItemBinding, len(handle.Items))
	for _, binding := range handle.Items {
		bindings[binding.ID] = binding
	}
	var consumed int64
	sourceLines := make([]json.RawMessage, 0)
	var readErr error
	for {
		line, complete, lineErr := readBatchLine(reader, lineLimit, totalLimit-consumed)
		consumed += int64(len(line))
		if len(bytes.TrimSpace(line)) != 0 {
			if lineErr != nil && !complete {
				readErr = prompty.ErrBatchResults
				break
			}
			if !uniqueSyntheticJSON(line) {
				readErr = prompty.ErrBatchResults
				break
			}
			item := b.parseResult(line, bindings, prepared)
			result.Items = append(result.Items, item)
			sourceLines = append(sourceLines, prompty.CloneJSONDocument(line))
		}
		if lineErr != nil {
			if errors.Is(lineErr, io.EOF) {
				result.Completeness.DownloadComplete = true
			} else {
				readErr = prompty.ErrBatchResults
			}
			break
		}
		if err = ctx.Err(); err != nil {
			readErr = err
			break
		}
	}
	if closeErr := stream.Close(); closeErr != nil {
		readErr = errors.Join(readErr, prompty.ErrBatchResults)
		result.Completeness.DownloadComplete = false
	}
	result.Completeness = prompty.ReconcileBatchResults(handle, result.Items, result.Completeness.DownloadComplete)
	if !result.Completeness.CorrelationComplete {
		readErr = errors.Join(readErr, prompty.ErrBatchResults)
	}
	providerJSON, _ := json.Marshal(sourceLines)
	providerErr := capture.Provider(ctx, providerJSON, source.status, result.Completeness.DownloadComplete)
	return capture.Finish(ctx, result, errors.Join(readErr, ctx.Err(), providerErr))
}

// ReadSlice bounds allocation before accumulating one original line. Clean EOF
// without a trailing newline is valid; a transport error never substitutes EOF.
func readBatchLine(reader *bufio.Reader, lineLimit int, remaining int64) ([]byte, bool, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > lineLimit-len(line) || int64(len(fragment)) > remaining-int64(len(line)) {
			return line, false, prompty.ErrBatchResults
		}
		line = append(line, fragment...)
		if errors.Is(err, bufio.ErrBufferFull) {
			continue
		}
		return line, err == nil || errors.Is(err, io.EOF), err
	}
}

func (b *MessageBatches) parseResult(
	raw []byte,
	bindings map[string]prompty.BatchItemBinding,
	prepared map[string]*adapter.PreparedBatchResultItem,
) prompty.BatchItemResult {
	item := prompty.BatchItemResult{Outcome: prompty.BatchItemUnknown, SourceJSON: prompty.CloneJSONDocument(raw)}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(fields["custom_id"], &item.ID) != nil ||
		!batchCallerID.MatchString(item.ID) {
		item.ErrorKind = "invalid_correlation"
		return item
	}
	if binding, exists := bindings[item.ID]; exists {
		item.Binding = &binding
	}
	if item.Binding == nil {
		item.ErrorKind = "unexpected_correlation"
		return item
	}
	var resultFields map[string]json.RawMessage
	var kind string
	if json.Unmarshal(fields["result"], &resultFields) != nil || json.Unmarshal(resultFields["type"], &kind) != nil {
		item.ErrorKind = "invalid_result"
		return item
	}
	switch kind {
	case "canceled":
		item.Outcome = prompty.BatchItemCanceled
	case "expired":
		item.Outcome = prompty.BatchItemExpired
	case "errored":
		var providerError map[string]json.RawMessage
		if json.Unmarshal(resultFields["error"], &providerError) != nil || providerError == nil {
			item.ErrorKind = "invalid_provider_error"
			return item
		}
		item.Outcome, item.ErrorKind = prompty.BatchItemError, "provider_error"
	case "succeeded":
		var message sdk.Message
		if json.Unmarshal(resultFields["message"], &message) != nil || !message.JSON.Model.Valid() ||
			!message.JSON.ID.Valid() || message.ID == "" || message.Role != "assistant" || message.Type != "message" ||
			message.Model != item.Binding.Identity.Model {
			item.ErrorKind = "invalid_message"
			return item
		}
		response, err := adapter.ParseBatchResultItem(b.base, prepared[item.ID], &message)
		item.Response = response
		if response == nil {
			item.ErrorKind = "invalid_message"
			return item
		}
		switch response.Outcome {
		case prompty.OutcomeRefusal:
			item.Outcome = prompty.BatchItemRefusal
		case prompty.OutcomeIncomplete:
			item.Outcome = prompty.BatchItemIncomplete
		default:
			item.Outcome = prompty.BatchItemSuccess
		}
		if err != nil {
			item.ErrorKind = "validation_failed"
			if item.Outcome != prompty.BatchItemRefusal {
				item.Outcome = prompty.BatchItemIncomplete
			}
		}
	default:
		item.ErrorKind = "unknown_result"
	}
	return item
}
