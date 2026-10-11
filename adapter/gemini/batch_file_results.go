package gemini

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

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *Batches) fileResults(
	ctx context.Context, result *prompty.BatchResult, options prompty.BatchResultsOptions,
	bindings map[string]prompty.BatchItemBinding, prepared map[string]*adapter.PreparedBatchResultItem,
) (*prompty.BatchResult, error) {
	if err := b.fileScope(options.Options.Capture); err != nil {
		return result, err
	}
	var output prompty.BatchResourceRef
	for _, resource := range result.Handle.Resources {
		if resource.Kind == wireOutput {
			output = resource
		}
	}
	if output.ProviderID == "" {
		result.Completeness = prompty.ReconcileBatchResults(*result.Handle, nil, false)
		return result, prompty.ErrBatchUnavailable
	}
	maxBytes, maxLine := options.MaxBytes, options.MaxLineBytes
	if maxBytes == 0 {
		maxBytes = batchFileDefaultMaxBytes
	}
	if maxLine == 0 {
		maxLine = defaultResponsePayloadLimit
	}
	if maxBytes <= 0 || maxBytes == math.MaxInt64 || maxLine < 1 {
		return result, prompty.ErrBatch
	}
	capture, err := adapter.NewBatchCapture(
		"gemini.batch.file_results",
		"google.golang.org/genai",
		result.Handle.Items[0].Identity,
		options.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, result.Handle, output); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	capture.MarkDispatched()
	download, downloadErr := b.files.Download(ctx, output)
	if downloadErr != nil || download == nil || download.Body == nil {
		if download != nil && download.Body != nil {
			_ = download.Body.Close()
		}
		result.Completeness = prompty.ReconcileBatchResults(*result.Handle, nil, false)
		return capture.Finish(ctx, result, errors.Join(prompty.ErrBatchUnavailable, ctx.Err()))
	}
	if download.HTTPStatus != 0 && (download.HTTPStatus < 200 || download.HTTPStatus >= 300) {
		_ = download.Body.Close()
		result.Completeness = prompty.ReconcileBatchResults(*result.Handle, nil, false)
		providerErr := capture.Provider(ctx, nil, download.HTTPStatus, false)
		return capture.Finish(ctx, result, errors.Join(prompty.ErrBatchUnavailable, providerErr, ctx.Err()))
	}
	reader := bufio.NewReader(io.LimitReader(download.Body, maxBytes+1))
	complete := false
	var operationErr error
	var observed int64
	for {
		if ctx.Err() != nil {
			operationErr = errors.Join(prompty.ErrBatchResults, ctx.Err())
			break
		}
		line, readErr := readBatchFileLine(reader, maxLine)
		observed += int64(len(line))
		if observed > maxBytes {
			operationErr = prompty.ErrBatchResults
			break
		}
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			operationErr = prompty.ErrBatchResults
			break
		}
		payload := bytes.TrimSpace(line)
		if len(payload) > 0 {
			item, parseErr := b.parseFileItem(payload, bindings, prepared)
			if item.ID != "" {
				result.Items = append(result.Items, item)
			}
			if parseErr != nil {
				operationErr = parseErr
				break
			}
		} else if len(line) > 0 {
			operationErr = prompty.ErrBatchResults
			break
		}
		if errors.Is(readErr, io.EOF) {
			complete = true
			break
		}
	}
	if closeErr := download.Body.Close(); closeErr != nil {
		operationErr, complete = prompty.ErrBatchResults, false
	}
	if ctx.Err() != nil {
		operationErr, complete = errors.Join(prompty.ErrBatchResults, ctx.Err()), false
	}
	result.Completeness = prompty.ReconcileBatchResults(*result.Handle, result.Items, complete)
	sources := make([]prompty.JSONDocument, 0, len(result.Items))
	for _, item := range result.Items {
		sources = append(sources, item.SourceJSON)
	}
	payload, marshalErr := json.Marshal(sources)
	providerErr := capture.Provider(ctx, payload, download.HTTPStatus, complete)
	if marshalErr != nil {
		operationErr = errors.Join(operationErr, prompty.ErrBatchResults)
	}
	return capture.Finish(ctx, result, errors.Join(operationErr, providerErr))
}

func (b *Batches) parseFileItem(payload []byte, bindings map[string]prompty.BatchItemBinding,
	prepared map[string]*adapter.PreparedBatchResultItem,
) (prompty.BatchItemResult, error) {
	if !json.Valid(payload) || validateBatchJSON(payload) != nil {
		return prompty.BatchItemResult{}, prompty.ErrBatchResults
	}
	var record struct {
		Key      string          `json:"key"`
		Response json.RawMessage `json:"response"`
		Error    json.RawMessage `json:"error"`
	}
	if json.Unmarshal(payload, &record) != nil || record.Key == "" {
		return prompty.BatchItemResult{}, prompty.ErrBatchResults
	}
	wrapped, err := json.Marshal(struct {
		Metadata map[string]string `json:"metadata"`
		Response json.RawMessage   `json:"response,omitempty"`
		Error    json.RawMessage   `json:"error,omitempty"`
	}{Metadata: map[string]string{"key": record.Key}, Response: record.Response, Error: record.Error})
	if err != nil {
		return prompty.BatchItemResult{}, prompty.ErrBatchResults
	}
	item, parseErr := b.parseInlineItem(wrapped, bindings, prepared)
	item.SourceJSON = prompty.CloneJSONDocument(payload)
	return item, parseErr
}

func readBatchFileLine(reader *bufio.Reader, limit int) ([]byte, error) {
	var line []byte
	for {
		fragment, err := reader.ReadSlice('\n')
		if len(fragment) > limit-len(line) {
			return line, prompty.ErrBatchResults
		}
		line = append(line, fragment...)
		if !errors.Is(err, bufio.ErrBufferFull) {
			return line, err
		}
	}
}
