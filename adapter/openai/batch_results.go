package openai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"math"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const (
	batchDefaultMaxLine        = 4 * 1024 * 1024
	batchDefaultMaxBytes int64 = 64 * 1024 * 1024
)

func (b *Batches) resultBindings(
	handle prompty.BatchHandle,
	options prompty.BatchResultsOptions,
) (map[string]*adapter.PreparedBatchResultItem, error) {
	if len(options.Bindings) != len(handle.Items) {
		return nil, prompty.ErrBatchScope
	}
	inputs := make(map[string]*prompty.PromptExecution)
	for _, binding := range options.Bindings {
		if binding.Execution == nil || inputs[binding.ID] != nil {
			return nil, prompty.ErrBatchScope
		}
		inputs[binding.ID] = binding.Execution.Clone()
	}
	prepared := make(map[string]*adapter.PreparedBatchResultItem)
	for _, binding := range handle.Items {
		exec := inputs[binding.ID]
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

//nolint:cyclop,funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *Batches) Results(
	ctx context.Context,
	handle prompty.BatchHandle,
	options prompty.BatchResultsOptions,
) (*prompty.BatchResult, error) {
	handle = prompty.CloneBatchHandle(handle)
	if err := b.fileScope(); err != nil {
		return nil, err
	}
	if err := b.fileCapture(options.Options.Capture); err != nil {
		return nil, err
	}
	reports, err := b.operationReports(handle, prompty.CapabilityBatchResults)
	if err != nil {
		return nil, err
	}
	switch handle.Status {
	case prompty.BatchEnded, prompty.BatchExpired, prompty.BatchCanceled, prompty.BatchFailed:
	case prompty.BatchQueued, prompty.BatchRunning, prompty.BatchCanceling, prompty.BatchUnknown:
		fallthrough
	default:
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
		lineLimit = batchDefaultMaxLine
	}
	if totalLimit == 0 {
		totalLimit = batchDefaultMaxBytes
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
		SubmissionAttempted: false,
		Handle:              &handle,
		Diagnostics:         reports,
		Resources:           append([]prompty.BatchResourceRef(nil), handle.Resources...),
	}
	capture, err := adapter.NewBatchCapture(
		"openai.batch.results",
		"github.com/openai/openai-go/v3",
		handle.Items[0].Identity,
		options.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, handle, handle.Resources); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	bindings := make(map[string]prompty.BatchItemBinding)
	for _, binding := range handle.Items {
		bindings[binding.ID] = binding
	}
	downloaded, fileCount := true, 0
	var failure error
	for _, resource := range handle.Resources {
		if resource.Kind == wireInput {
			continue
		}
		fileCount++
		if totalLimit <= 0 {
			downloaded = false
			failure = prompty.ErrBatchResults
			break
		}
		capture.MarkDispatched()
		file, downloadErr := b.files.Download(ctx, resource)
		if downloadErr != nil || file == nil || file.Body == nil {
			if file != nil && file.Body != nil {
				_ = file.Body.Close()
			}
			downloaded = false
			failure = errors.Join(prompty.ErrBatchResults, prompty.ErrBatchUnavailable, ctx.Err())
			status := 0
			if file != nil {
				status = file.HTTPStatus
			}
			failure = errors.Join(failure, capture.Provider(ctx, nil, status, false))
			break
		}
		lines, consumed, complete, readErr := b.readResultFile(
			ctx,
			file.Body,
			lineLimit,
			totalLimit,
			bindings,
			prepared,
			result,
		)
		totalLimit -= consumed
		if closeErr := file.Body.Close(); closeErr != nil {
			complete = false
			readErr = errors.Join(readErr, prompty.ErrBatchResults)
		}
		payload, _ := json.Marshal(lines)
		providerErr := capture.Provider(ctx, payload, file.HTTPStatus, complete)
		failure = errors.Join(failure, readErr, providerErr)
		if !complete || providerErr != nil {
			downloaded = false
			break
		}
	}
	if fileCount == 0 {
		downloaded = false
		failure = prompty.ErrBatchUnavailable
	}
	result.Completeness = prompty.ReconcileBatchResults(handle, result.Items, downloaded)
	if !result.Completeness.CorrelationComplete {
		failure = errors.Join(failure, prompty.ErrBatchResults)
	}
	return capture.Finish(ctx, result, errors.Join(failure, ctx.Err()))
}

func (b *Batches) readResultFile(
	ctx context.Context,
	body io.Reader,
	lineLimit int,
	totalLimit int64,
	bindings map[string]prompty.BatchItemBinding,
	prepared map[string]*adapter.PreparedBatchResultItem,
	result *prompty.BatchResult,
) ([]json.RawMessage, int64, bool, error) {
	reader := bufio.NewReaderSize(io.LimitReader(body, totalLimit+1), min(lineLimit, batchDefaultMaxLine))
	lines := make([]json.RawMessage, 0)
	var consumed int64
	for {
		line, complete, err := readBatchLine(reader, lineLimit, totalLimit-consumed)
		consumed += int64(len(line))
		if len(bytes.TrimSpace(line)) != 0 {
			if err != nil && !complete {
				return lines, consumed, false, prompty.ErrBatchResults
			}
			if !batchUniqueJSON(line) {
				return lines, consumed, false, prompty.ErrBatchResults
			}
			result.Items = append(result.Items, b.parseItem(line, bindings, prepared))
			lines = append(lines, prompty.CloneJSONDocument(line))
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return lines, consumed, true, nil
			}
			return lines, consumed, false, prompty.ErrBatchResults
		}
		if err = ctx.Err(); err != nil {
			return lines, consumed, false, err
		}
	}
}

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
