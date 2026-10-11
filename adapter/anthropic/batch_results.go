package anthropic

import (
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

// Generic IO evidence; no JSONL framing, decoding or request mutation.
type batchResultsSource struct {
	status    int
	complete  bool
	failure   bool
	remaining int64
}
type batchResultsBody struct {
	io.ReadCloser

	source *batchResultsSource
}

func (b *batchResultsBody) Read(dst []byte) (int, error) {
	if b.source.failure || b.source.remaining < 0 {
		return 0, prompty.ErrBatchResults
	}
	if b.source.remaining < int64(len(dst)) {
		dst = dst[:b.source.remaining+1]
	}
	n, err := b.ReadCloser.Read(dst)
	b.source.remaining -= int64(n)
	if b.source.remaining < 0 {
		b.source.failure = true
		return n, prompty.ErrBatchResults
	}
	if errors.Is(err, io.EOF) {
		b.source.complete = true
	} else if err != nil {
		b.source.failure = true
	}
	return n, err
}
func (b *batchResultsBody) Close() error {
	err := b.ReadCloser.Close()
	if err != nil {
		b.source.failure = true
	}
	return err
}

func (s *batchResultsSource) observe(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	response, err := next(request)
	if err == nil && response != nil {
		s.status = response.StatusCode
		if response.Body != nil {
			response.Body = &batchResultsBody{ReadCloser: response.Body, source: s}
		}
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

//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
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
	result := &prompty.BatchResult{
		Items: nil,
		Completeness: prompty.BatchCompleteness{
			DownloadComplete:    false,
			CorrelationComplete: false,
			MissingIDs:          nil,
			DuplicateIDs:        nil,
			UnexpectedIDs:       nil,
		},
		Capture:             nil,
		Resources:           nil,
		SubmissionAttempted: false,
		Handle:              &handle,
		Diagnostics:         reports,
	}
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
	source := &batchResultsSource{remaining: totalLimit, status: 0, complete: false, failure: false}
	var params sdk.MessageBatchResultsParams
	stream := b.base.client.Messages.Batches.ResultsStreaming(
		ctx,
		handle.ProviderID,
		params,
		option.WithMaxRetries(0),
		option.WithMiddleware(source.observe),
	)
	if err = stream.Err(); err != nil {
		providerErr := capture.Provider(ctx, nil, source.status, false)
		return capture.Finish(ctx, result, errors.Join(prompty.ErrBatchUnavailable, ctx.Err(), providerErr))
	}
	// The pinned SDK exposes scanner failures through Next/Err and owns JSONL
	// framing. Generic IO evidence and policy limits remain separate from parsing.
	bindings := make(map[string]prompty.BatchItemBinding, len(handle.Items))
	for _, binding := range handle.Items {
		bindings[binding.ID] = binding
	}
	var consumed int64
	sourceLines := make([]json.RawMessage, 0)
	var readErr error
	for stream.Next() {
		record := stream.Current()
		line := []byte(record.RawJSON())
		consumed += int64(len(line))
		if len(line) > lineLimit || consumed > totalLimit || !uniqueSyntheticJSON(line) {
			readErr = prompty.ErrBatchResults
			break
		}
		item := b.parseResult(line, bindings, prepared)
		result.Items = append(result.Items, item)
		sourceLines = append(sourceLines, prompty.CloneJSONDocument(line))
		if err = ctx.Err(); err != nil {
			readErr = err
			break
		}
	}
	if stream.Err() != nil {
		readErr = errors.Join(readErr, prompty.ErrBatchResults)
	}
	// Require SDK success and clean generic IO evidence; correlation is separate.
	result.Completeness.DownloadComplete = readErr == nil && source.complete && !source.failure
	if !result.Completeness.DownloadComplete {
		readErr = errors.Join(readErr, prompty.ErrBatchResults)
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

//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *MessageBatches) parseResult(
	raw []byte,
	bindings map[string]prompty.BatchItemBinding,
	prepared map[string]*adapter.PreparedBatchResultItem,
) prompty.BatchItemResult {
	item := prompty.BatchItemResult{
		ID:         "",
		Binding:    nil,
		Response:   nil,
		ErrorKind:  "",
		Outcome:    prompty.BatchItemUnknown,
		SourceJSON: prompty.CloneJSONDocument(raw),
	}
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
	if json.Unmarshal(fields["result"], &resultFields) != nil || json.Unmarshal(resultFields[wireType], &kind) != nil {
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
		case prompty.OutcomeCompleted, prompty.OutcomeToolCalls, prompty.OutcomePaused:
			fallthrough
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
