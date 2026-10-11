package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const batchInlineMaxBytes = 20_000_000

// Batches performs explicit Gemini Developer API native batch operations.
// The host owns handle persistence, polling, credentials and retry decisions.
type Batches struct {
	base       *Adapter
	account    string
	files      prompty.BatchFileTransport
	fileLimits BatchFileOptions
	fileMode   bool
}

func NewBatches(account string, opts ...Option) *Batches {
	return &Batches{
		base:       New(opts...),
		account:    account,
		files:      nil,
		fileLimits: BatchFileOptions{MaxInputBytes: 0, MaxLineBytes: 0},
		fileMode:   false,
	}
}

func (b *Batches) Get(
	ctx context.Context,
	handle prompty.BatchHandle,
	options ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	if len(options) > 1 {
		return nil, prompty.ErrBatch
	}
	var option prompty.BatchOperationOptions
	if len(options) == 1 {
		option = options[0]
	}
	return b.base.batchOperation(ctx, b.account, handle, option, false)
}

func (b *Batches) Cancel(
	ctx context.Context,
	handle prompty.BatchHandle,
	options ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	if len(options) > 1 {
		return nil, prompty.ErrBatch
	}
	var option prompty.BatchOperationOptions
	if len(options) == 1 {
		option = options[0]
	}
	return b.base.batchOperation(ctx, b.account, handle, option, true)
}

func (b *Batches) prepareItem(
	exec *prompty.PromptExecution,
	capability string,
) (*adapter.PreparedBatchItem[*Request], *prompty.PromptExecution, error) {
	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, nil, err
	}
	if strings.TrimSpace(b.account) == "" {
		return nil, nil, prompty.ErrBatchScope
	}
	if b.base.responseClient == nil {
		return nil, nil, adapter.ErrNoClient
	}
	if b.base.responseClient.ClientConfig().Backend != genai.BackendGeminiAPI {
		return nil, nil, adapter.ErrUnsupportedCapability
	}
	original := exec.Clone()
	if original.ModelOptions == nil {
		original.ModelOptions = &prompty.ModelOptions{
			Model:            "",
			Temperature:      nil,
			MaxTokens:        nil,
			TopP:             nil,
			Stop:             nil,
			ProviderSettings: nil,
		}
	}
	if original.ModelOptions.Model == "" {
		original.ModelOptions.Model = b.base.defaultModel
	}
	working := original.Clone()
	working.Requirements.Capabilities = append(working.Requirements.Capabilities, capability)
	prepared, err := adapter.PrepareBatchItem(b.base, working)
	if err != nil {
		return nil, nil, err
	}
	if prepared.Request.Config.HTTPOptions != nil {
		return nil, nil, adapter.ErrUnsupportedCapability
	}
	return prepared, original, nil
}

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *Batches) Submit(ctx context.Context, input prompty.BatchSubmitRequest) (*prompty.BatchResult, error) {
	if b.fileMode {
		return b.submitFile(ctx, input)
	}
	if len(input.Items) == 0 || b.base.responseLimit <= 0 {
		return nil, prompty.ErrBatch
	}
	handle := prompty.BatchHandle{
		ProviderID:     "",
		ProviderStatus: "",
		Items:          nil,
		Resources:      nil,
		Format:         prompty.BatchWireFormat,
		Provider:       wireGemini,
		Endpoint:       b.base.RuntimeDescriptor().Endpoint,
		AccountBinding: b.account,
		Status:         prompty.BatchUnknown,
	}
	result := new(prompty.BatchResult)
	requests := make([]*genai.InlinedRequest, 0, len(input.Items))
	var canonical []prompty.PromptExecutionWire
	var originals []*prompty.PromptExecution
	seen := make(map[string]bool)
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
			AttemptID: item.AttemptID, ParentAttemptID: item.ParentAttemptID,
			Identity: prepared.Diagnostics.Profile, ExecutionDigest: digest})
		result.Diagnostics = append(result.Diagnostics, prepared.Diagnostics)
		requests = append(
			requests,
			//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
			&genai.InlinedRequest{
				Contents: prepared.Request.Contents,
				Config:   prepared.Request.Config,
				Metadata: map[string]string{"key": item.ID},
			},
		)
		wire, err := prompty.MarshalExecution(original, prompty.WirePolicy{State: prompty.StatePreserve})
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, wire)
		originals = append(originals, original)
	}
	// Validate even lineage fields before the resource identity exists.
	validation := prompty.CloneBatchHandle(handle)
	validation.ProviderID = "batches/preflight"
	if err := prompty.ValidateBatchHandle(validation); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(requests)
	if err != nil || len(encoded) >= batchInlineMaxBytes {
		return nil, prompty.ErrBatch
	}
	capture, err := adapter.NewBatchCapture(
		"gemini.batch.submit",
		"google.golang.org/genai",
		handle.Items[0].Identity,
		input.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, canonical, requests); err != nil {
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
	capture.MarkDispatched()
	encoding := batchExactContents(requests)
	source, operationErr := observeBatchOperation(ctx, b.base.responseLimit, func(callCtx context.Context) error {
		_, callErr := b.base.responseClient.Batches.Create(
			callCtx,
			handle.Items[0].Identity.Model,
			//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
			&genai.BatchJobSource{
				InlinedRequests: requests,
			},
			&genai.CreateBatchJobConfig{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
				//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
				HTTPOptions: &genai.HTTPOptions{
					ExtrasRequestProvider: encoding.provider,
				},
			},
		)
		return callErr
	})
	if encoding.rejected {
		result.SubmissionAttempted = false
		return capture.Finish(ctx, result, prompty.ErrBatch)
	}
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
	providerErr := capture.Provider(ctx, batchSourceJSON(source), source.status, source.complete && !source.overflow)
	return capture.Finish(ctx, result, errors.Join(operationErr, providerErr))
}

// The SDK deepMarshal step rounds interface numbers. Restore already checked
// content DTOs after native protocol conversion, before the actual JSON encoder.
type batchContentEncoding struct {
	provider genai.ExtrasRequestProvider
	rejected bool
}

func batchExactContents(requests []*genai.InlinedRequest) *batchContentEncoding {
	encoding := new(batchContentEncoding)
	exact := make([]any, len(requests))
	for i, request := range requests {
		encoded, _ := encodeExactGeminiContents(request.Contents) // Prepared canonical content is serializable.
		exact[i] = geminiJSONValue(encoded)
	}
	encoding.provider = func(body map[string]any) map[string]any {
		batch, _ := body["batch"].(map[string]any)
		input, _ := batch["inputConfig"].(map[string]any)
		group, _ := input["requests"].(map[string]any)
		var items []map[string]any
		switch values := group["requests"].(type) {
		case []map[string]any:
			items = values // Native SDK converters return a typed map slice.
		case []any:
			for _, value := range values {
				entry, ok := value.(map[string]any)
				if !ok {
					encoding.rejected = true
					return map[string]any{wireInvalidBatchEncoding: json.Number("invalid")}
				}
				items = append(items, entry)
			}
		}
		if len(items) != len(exact) {
			encoding.rejected = true
			return map[string]any{wireInvalidBatchEncoding: json.Number("invalid")}
		}
		for i, item := range items {
			request, _ := item["request"].(map[string]any)
			if request == nil {
				encoding.rejected = true
				return map[string]any{wireInvalidBatchEncoding: json.Number("invalid")}
			}
			request["contents"] = exact[i]
		}
		encoded, err := json.Marshal(body)
		if err != nil || len(encoded)+1 >= batchInlineMaxBytes {
			encoding.rejected = true
			return map[string]any{wireInvalidBatchEncoding: json.Number("invalid")}
		}
		return body
	}
	return encoding
}
