package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const maxBatchRequests = 100000

var batchCallerID = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,64}$`)
var batchProviderID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// MessageBatches implements explicit native batch operations, not an Invoker
// or a scheduler. Existing Messages profiles need independent batch capabilities.
type MessageBatches struct {
	base    *Adapter
	account string
}

func NewMessageBatches(accountBinding string, opts ...Option) *MessageBatches {
	return &MessageBatches{base: New(opts...), account: accountBinding}
}

func (b *MessageBatches) validScope(handle prompty.BatchHandle) error {
	if err := prompty.ValidateBatchHandle(handle); err != nil {
		return err
	}
	desc := b.base.RuntimeDescriptor()
	if b.base.client == nil {
		return adapter.ErrNoClient
	}
	if strings.TrimSpace(b.account) == "" || handle.Provider != desc.Name || handle.Endpoint != desc.Endpoint ||
		handle.AccountBinding != b.account {
		return prompty.ErrBatchScope
	}
	if !batchProviderID.MatchString(handle.ProviderID) {
		return prompty.ErrBatch
	}
	return nil
}

func (b *MessageBatches) itemExecution(
	exec *prompty.PromptExecution,
	capability string,
) (*prompty.PromptExecution, error) {
	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, err
	}
	if b.base.client == nil {
		return nil, adapter.ErrNoClient
	}
	if strings.TrimSpace(b.account) == "" {
		return nil, prompty.ErrBatchScope
	}
	working := exec.Clone()
	if working.ModelOptions == nil {
		working.ModelOptions = &prompty.ModelOptions{}
	}
	if working.ModelOptions.Model == "" {
		working.ModelOptions.Model = b.base.defaultModel
	}
	working.Requirements.Capabilities = append(working.Requirements.Capabilities, capability)
	return working, nil
}

func (b *MessageBatches) Submit(ctx context.Context, input prompty.BatchSubmitRequest) (*prompty.BatchResult, error) {
	if len(input.Items) == 0 || len(input.Items) > maxBatchRequests {
		return nil, prompty.ErrBatch
	}
	var params sdk.MessageBatchNewParams
	handle := prompty.BatchHandle{Format: prompty.BatchWireFormat, Provider: "anthropic", Endpoint: b.base.endpoint,
		AccountBinding: b.account, Status: prompty.BatchUnknown}
	result := new(prompty.BatchResult)
	seen := make(map[string]bool)
	type canonicalItem struct {
		Binding   prompty.BatchItemBinding
		Execution prompty.PromptExecutionWire
	}
	var canonical []canonicalItem
	var originals []*prompty.PromptExecution
	for _, item := range input.Items {
		if !batchCallerID.MatchString(item.ID) || seen[item.ID] || strings.TrimSpace(item.AttemptID) == "" ||
			item.ParentAttemptID == item.AttemptID || (item.ParentAttemptID != "" && strings.TrimSpace(item.ParentAttemptID) == "") {
			return nil, prompty.ErrBatch
		}
		seen[item.ID] = true
		if item.OriginalID == "" {
			item.OriginalID = item.ID
		}
		if strings.TrimSpace(item.OriginalID) == "" {
			return nil, prompty.ErrBatch
		}
		exec, err := b.itemExecution(item.Execution, prompty.CapabilityBatchSubmit)
		if err != nil {
			return nil, err
		}
		prepared, err := adapter.PrepareBatchItem(b.base, exec)
		if err != nil {
			return nil, err
		}
		if prepared.Request.MaxTokens < 1 {
			return nil, prompty.ErrBatch
		}
		original := item.Execution.Clone()
		original.ModelOptions = exec.ModelOptions
		originals = append(originals, original)
		digest, err := prompty.BatchExecutionDigest(original)
		if err != nil {
			return nil, err
		}
		binding := prompty.BatchItemBinding{ID: item.ID, OriginalID: item.OriginalID, AttemptID: item.AttemptID,
			ParentAttemptID: item.ParentAttemptID, Identity: prepared.Diagnostics.Profile, ExecutionDigest: digest}
		handle.Items = append(handle.Items, binding)
		result.Diagnostics = append(result.Diagnostics, prepared.Diagnostics)
		encoded, err := json.Marshal(prepared.Request)
		if err != nil {
			return nil, prompty.ErrBatch
		}
		params.Requests = append(params.Requests, sdk.MessageBatchNewParamsRequest{CustomID: item.ID,
			Params: param.Override[sdk.MessageBatchNewParamsRequestParams](json.RawMessage(encoded))})
		wire, err := prompty.MarshalExecution(original, prompty.WirePolicy{State: prompty.StatePreserve})
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, canonicalItem{Binding: binding, Execution: wire})
	}
	capture, err := adapter.NewBatchCapture(
		"anthropic.batch.submit",
		"github.com/anthropics/anthropic-sdk-go",
		handle.Items[0].Identity,
		input.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, canonical, params); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	for i, original := range originals {
		if err = adapter.ValidateDispatchState(b.base, original, handle.Items[i].Identity); err != nil {
			return capture.Finish(ctx, result, err)
		}
	}
	capture.MarkDispatched()
	source := new(batchSource)
	result.SubmissionAttempted = true
	raw, callErr := b.base.client.Messages.Batches.New(
		ctx,
		params,
		option.WithMaxRetries(0),
		option.WithMiddleware(source.observe),
	)
	providerErr := capture.Provider(ctx, source.data, source.status, source.complete)
	if callErr != nil {
		if identity := source.acceptedIdentity(); identity != "" {
			handle.ProviderID = identity
			result.Handle = &handle
		}
		cause := prompty.ErrBatchUnknown
		if batchMutationRejected(source.status) {
			cause = prompty.ErrBatchRejected
		}
		return capture.Finish(ctx, result, errors.Join(cause, ctx.Err(), providerErr))
	}
	err = parseBatchHandle(raw, &handle)
	if err != nil && handle.ProviderID == "" {
		handle.ProviderID = source.acceptedIdentity()
	}
	if handle.ProviderID != "" {
		result.Handle = &handle
	}
	return capture.Finish(ctx, result, errors.Join(err, providerErr))
}

func (b *MessageBatches) Get(
	ctx context.Context,
	handle prompty.BatchHandle,
	opts ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return b.observe(ctx, handle, false, opts...)
}

func (b *MessageBatches) Cancel(
	ctx context.Context,
	handle prompty.BatchHandle,
	opts ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return b.observe(ctx, handle, true, opts...)
}

func (b *MessageBatches) operationReport(
	handle prompty.BatchHandle,
	capability string,
) ([]prompty.ExecutionDiagnostics, error) {
	if err := b.validScope(handle); err != nil {
		return nil, err
	}
	var reports []prompty.ExecutionDiagnostics
	for _, binding := range handle.Items {
		report, err := adapter.PreflightOperation(b.base.RuntimeDescriptor(), binding.Identity.Model,
			[]string{capability}, nil)
		if err != nil {
			return nil, err
		}
		if report.Profile != binding.Identity {
			return nil, prompty.ErrBatchScope
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func (b *MessageBatches) observe(
	ctx context.Context,
	handle prompty.BatchHandle,
	cancel bool,
	opts ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	handle = prompty.CloneBatchHandle(handle)
	if len(opts) > 1 {
		return nil, prompty.ErrBatch
	}
	var selected prompty.BatchOperationOptions
	if len(opts) == 1 {
		selected = opts[0]
	}
	capability, operation := prompty.CapabilityBatchInspect, "anthropic.batch.inspect"
	if cancel {
		capability, operation = prompty.CapabilityBatchCancel, "anthropic.batch.cancel"
	}
	reports, err := b.operationReport(handle, capability)
	if err != nil {
		return nil, err
	}
	result := &prompty.BatchResult{Handle: &handle, Diagnostics: reports}
	capture, err := adapter.NewBatchCapture(
		operation,
		"github.com/anthropics/anthropic-sdk-go",
		handle.Items[0].Identity,
		selected,
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
	source := new(batchSource)
	var raw *sdk.MessageBatch
	if cancel {
		var params sdk.MessageBatchCancelParams
		raw, err = b.base.client.Messages.Batches.Cancel(
			ctx,
			handle.ProviderID,
			params,
			option.WithMaxRetries(0),
			option.WithMiddleware(source.observe),
		)
	} else {
		var params sdk.MessageBatchGetParams
		raw, err = b.base.client.Messages.Batches.Get(
			ctx, handle.ProviderID, params, option.WithMaxRetries(0), option.WithMiddleware(source.observe),
		)
	}
	providerErr := capture.Provider(ctx, source.data, source.status, source.complete)
	if err != nil {
		switch {
		case cancel && batchMutationRejected(source.status):
			err = prompty.ErrBatchRejected
		case cancel && source.status != http.StatusNotFound && source.status != http.StatusGone:
			handle.Status = prompty.BatchUnknown
			err = prompty.ErrBatchUnknown
		default:
			err = prompty.ErrBatchUnavailable
		}
		return capture.Finish(ctx, result, errors.Join(err, ctx.Err(), providerErr))
	}
	err = parseBatchHandle(raw, &handle)
	if cancel && errors.Is(err, prompty.ErrBatchUnknown) {
		handle.Status = prompty.BatchUnknown
	}
	return capture.Finish(ctx, result, errors.Join(err, providerErr))
}

var _ prompty.BatchManager = (*MessageBatches)(nil)

func batchMutationRejected(status int) bool {
	switch status {
	case http.StatusBadRequest, http.StatusUnauthorized, http.StatusForbidden, http.StatusNotFound,
		http.StatusRequestEntityTooLarge, http.StatusUnsupportedMediaType, http.StatusUnprocessableEntity, http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}
