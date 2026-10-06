package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"slices"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const batchFileMaxBytes int64 = 2_000_000_000
const batchFileDefaultMaxBytes int64 = 64 * 1024 * 1024

// BatchFileOptions bounds native input encoding; zero selects bounded defaults.
type BatchFileOptions struct {
	MaxInputBytes int64
	MaxLineBytes  int
}

// NewFileBatches selects file-backed submission explicitly. The injected port
// owns file transport/auth/egress; no file store or cleanup worker is installed.
func NewFileBatches(
	account string,
	files prompty.BatchFileTransport,
	limits BatchFileOptions,
	opts ...Option,
) *Batches {
	return &Batches{base: New(opts...), account: account, files: files, fileLimits: limits, fileMode: true}
}

func (b *Batches) fileScope(capture prompty.CaptureRequirement) error {
	if b.files == nil {
		return adapter.ErrUnsupportedCapability
	}
	if b.files.Binding() != (prompty.BatchFileBinding{Provider: "gemini", Endpoint: b.base.RuntimeDescriptor().Endpoint, AccountBinding: b.account}) {
		return prompty.ErrBatchScope
	}
	if slices.Contains(capture.Stages, prompty.CaptureProviderResponse) {
		describer, ok := b.files.(prompty.BatchFileCaptureDescriber)
		if !ok {
			return prompty.ErrCaptureUnsupported
		}
		accuracy := describer.CaptureAccuracy()
		if accuracy != prompty.CaptureSemantic && (accuracy != prompty.CaptureMetadata ||
			capture.Accuracy == prompty.CaptureSemantic || capture.Accuracy == prompty.CaptureBytes) {
			return prompty.ErrCaptureUnsupported
		}
	}
	return nil
}

func (b *Batches) submitFile(ctx context.Context, input prompty.BatchSubmitRequest) (*prompty.BatchResult, error) {
	if err := b.fileScope(input.Options.Capture); err != nil {
		return nil, err
	}
	maxBytes, maxLine := b.fileLimits.MaxInputBytes, b.fileLimits.MaxLineBytes
	if maxBytes == 0 {
		maxBytes = batchFileDefaultMaxBytes
	}
	if maxLine == 0 {
		maxLine = defaultResponsePayloadLimit
	}
	if maxBytes < 1 || maxBytes > batchFileMaxBytes || maxLine < 1 || int64(maxLine) > maxBytes ||
		len(input.Items) == 0 || b.base.responseLimit <= 0 {
		return nil, prompty.ErrBatch
	}
	handle := prompty.BatchHandle{Format: prompty.BatchWireFormat, Provider: "gemini",
		Endpoint: b.base.RuntimeDescriptor().Endpoint, AccountBinding: b.account, Status: prompty.BatchUnknown}
	result := new(prompty.BatchResult)
	seen := make(map[string]bool)
	var lines []json.RawMessage
	var readers []io.Reader
	var originals []*prompty.PromptExecution
	var canonical []prompty.PromptExecutionWire
	var size int64
	for _, item := range input.Items {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] {
			return nil, prompty.ErrBatch
		}
		seen[item.ID] = true
		prepared, original, err := b.prepareItem(item.Execution, prompty.CapabilityBatchSubmit)
		if err != nil {
			return nil, err
		}
		if len(handle.Items) > 0 && prepared.Diagnostics.Profile.Model != handle.Items[0].Identity.Model {
			return nil, prompty.ErrBatchScope
		}
		if item.OriginalID == "" {
			item.OriginalID = item.ID
		}
		digest, err := prompty.BatchExecutionDigest(original)
		if err != nil {
			return nil, err
		}
		handle.Items = append(handle.Items, prompty.BatchItemBinding{ID: item.ID, OriginalID: item.OriginalID,
			AttemptID: item.AttemptID, ParentAttemptID: item.ParentAttemptID, Identity: prepared.Diagnostics.Profile,
			ExecutionDigest: digest})
		result.Diagnostics = append(result.Diagnostics, prepared.Diagnostics)
		request, err := encodeFileGeneration(ctx, b.base.responseClient, prepared.Request, maxLine)
		if err != nil {
			return nil, err
		}
		line, err := json.Marshal(struct {
			Key     string          `json:"key"`
			Request json.RawMessage `json:"request"`
		}{
			Key: item.ID, Request: request,
		})
		if err != nil || len(line) > maxLine || int64(len(line))+1 > maxBytes-size {
			return nil, prompty.ErrBatch
		}
		size += int64(len(line)) + 1
		lines = append(lines, line)
		readers = append(readers, bytes.NewReader(line), strings.NewReader("\n"))
		wire, err := prompty.MarshalExecution(original, prompty.WirePolicy{State: prompty.StatePreserve})
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, wire)
		originals = append(originals, original)
	}
	validation := prompty.CloneBatchHandle(handle)
	validation.ProviderID = "batches/preflight"
	if err := prompty.ValidateBatchHandle(validation); err != nil {
		return nil, err
	}
	capture, err := adapter.NewBatchCapture(
		"gemini.batch.file_submit",
		"google.golang.org/genai",
		handle.Items[0].Identity,
		input.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, canonical, lines); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	for i, original := range originals {
		if err = adapter.ValidateDispatchState(b.base, original, handle.Items[i].Identity); err != nil {
			return capture.Finish(ctx, result, err)
		}
		if original.CacheResource != nil {
			if err = b.base.ValidateCacheResource(*original.CacheResource, handle.Items[i].Identity); err != nil {
				return capture.Finish(ctx, result, err)
			}
		}
	}
	capture.MarkDispatched()
	uploaded, uploadErr := b.files.Upload(ctx, prompty.BatchFileUpload{Content: io.MultiReader(readers...),
		Filename: "batch.jsonl", MediaType: "application/jsonl", Purpose: "batch"})
	var uploadSource prompty.JSONDocument
	var status int
	if uploaded != nil {
		uploadSource, status = uploaded.SourceJSON, uploaded.HTTPStatus
		if uploaded.Resource.Kind == "input" && batchFileName.MatchString(uploaded.Resource.ProviderID) {
			result.Resources = []prompty.BatchResourceRef{uploaded.Resource}
		}
	}
	providerErr := capture.Provider(ctx, uploadSource, status, json.Valid(uploadSource))
	invalidStatus := status != 0 && (status < http.StatusOK || status >= http.StatusMultipleChoices)
	if uploadErr != nil || len(result.Resources) == 0 || invalidStatus {
		kind := prompty.ErrBatchFileUnknown
		if errors.Is(uploadErr, prompty.ErrBatchRejected) ||
			(status >= http.StatusBadRequest && status < http.StatusInternalServerError &&
				status != http.StatusRequestTimeout && status != http.StatusTooManyRequests) {
			kind = prompty.ErrBatchRejected
		}
		return capture.Finish(ctx, result, errors.Join(kind, providerErr, ctx.Err()))
	}
	if providerErr != nil {
		return capture.Finish(ctx, result, providerErr)
	}
	handle.Resources = append([]prompty.BatchResourceRef(nil), result.Resources...)
	params := &genai.BatchJobSource{FileName: result.Resources[0].ProviderID}
	if err = capture.Emit(ctx, prompty.CaptureTranslatedRequest, params); err != nil {
		return capture.Finish(ctx, result, err)
	}
	for i, original := range originals {
		if err = adapter.ValidateDispatchState(b.base, original, handle.Items[i].Identity); err != nil {
			return capture.Finish(ctx, result, err)
		}
		if original.CacheResource != nil {
			if err = b.base.ValidateCacheResource(*original.CacheResource, handle.Items[i].Identity); err != nil {
				return capture.Finish(ctx, result, err)
			}
		}
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	result.SubmissionAttempted = true
	source, operationErr := observeBatchOperation(ctx, b.base.responseLimit, func(callCtx context.Context) error {
		_, callErr := b.base.responseClient.Batches.Create(callCtx, handle.Items[0].Identity.Model, params, nil)
		return callErr
	})
	observed, parseErr := parseBatchSource(source, handle)
	if observed.ProviderID != "" {
		result.Handle = &observed
	}
	if operationErr != nil || parseErr != nil {
		kind := prompty.ErrBatchUnknown
		if source.status >= http.StatusBadRequest && source.status < http.StatusInternalServerError &&
			source.status != http.StatusRequestTimeout && source.status != http.StatusTooManyRequests {
			kind = prompty.ErrBatchRejected
		}
		operationErr = errors.Join(kind, operationErr, parseErr)
		if result.Handle != nil {
			result.Handle.Status = prompty.BatchUnknown
		}
	}
	providerErr = capture.Provider(ctx, batchSourceJSON(source), source.status, source.complete && !source.overflow)
	return capture.Finish(ctx, result, errors.Join(operationErr, providerErr))
}

type batchRequestEncoder struct {
	payload []byte
	limit   int
}

func (e *batchRequestEncoder) RoundTrip(request *http.Request) (*http.Response, error) {
	defer func() { _ = request.Body.Close() }()
	payload, err := io.ReadAll(io.LimitReader(request.Body, int64(e.limit)+1))
	if err != nil || len(payload) > e.limit {
		return nil, prompty.ErrBatch
	}
	e.payload = payload
	return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header),
		Body: io.NopCloser(strings.NewReader(`{}`)), Request: request}, nil
}

// Invoke only the SDK's local protocol encoder, with an isolated no-egress
// RoundTripper. No real generation request is sent to construct a JSONL record.
func encodeFileGeneration(
	ctx context.Context,
	client *genai.Client,
	request *Request,
	maxLine int,
) (json.RawMessage, error) {
	config := client.ClientConfig()
	encoder := &batchRequestEncoder{limit: maxLine, payload: nil}
	httpClient := *config.HTTPClient
	httpClient.Transport, httpClient.Jar = encoder, nil
	config.HTTPClient = &httpClient
	config.HTTPOptions.Headers = config.HTTPOptions.Headers.Clone()
	isolated, err := genai.NewClient(ctx, &config)
	if err != nil {
		return nil, errors.Join(prompty.ErrBatch, ctx.Err())
	}
	contents, err := encodeExactGeminiContents(request.Contents)
	if err != nil {
		return nil, prompty.ErrBatch
	}
	cfg := *request.Config
	cfg.HTTPOptions = &genai.HTTPOptions{ExtrasRequestProvider: func(body map[string]any) map[string]any {
		body["contents"] = geminiJSONValue(contents)
		return body
	}}
	_, err = isolated.Models.GenerateContent(ctx, request.Model, request.Contents, &cfg)
	if err != nil || !json.Valid(encoder.payload) {
		return nil, errors.Join(prompty.ErrBatch, ctx.Err())
	}
	return encoder.payload, nil
}
