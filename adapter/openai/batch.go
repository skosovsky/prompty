package openai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"slices"
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const (
	batchChatEndpoint  = "/v1/chat/completions"
	batchMaxItems      = 50000
	batchMaxInputBytes = 200 * 1024 * 1024
)

var batchPathID = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// Batches is the native file-backed OpenAI Batch port for Chat Completions.
// Responses/background are separate contracts, not silently substituted here.
type Batches struct {
	base    *Adapter
	account string
	files   prompty.BatchFileTransport
}

func NewBatches(accountBinding string, files prompty.BatchFileTransport, opts ...Option) *Batches {
	return &Batches{base: New(opts...), account: accountBinding, files: files}
}

func (b *Batches) fileScope() error {
	if b.files == nil {
		return adapter.ErrUnsupportedCapability
	}
	if b.files.Binding() != (prompty.BatchFileBinding{Provider: wireOpenai, Endpoint: b.base.endpoint, AccountBinding: b.account}) {
		return prompty.ErrBatchScope
	}
	return nil
}

func (b *Batches) fileCapture(requirement prompty.CaptureRequirement) error {
	if !slices.Contains(requirement.Stages, prompty.CaptureProviderResponse) {
		return nil
	}
	describer, ok := b.files.(prompty.BatchFileCaptureDescriber)
	if !ok {
		return prompty.ErrCaptureUnsupported
	}
	accuracy := describer.CaptureAccuracy()
	if accuracy != prompty.CaptureSemantic &&
		(accuracy != prompty.CaptureMetadata || requirement.Accuracy == prompty.CaptureSemantic || requirement.Accuracy == prompty.CaptureBytes) {
		return prompty.ErrCaptureUnsupported
	}
	return nil
}

func (b *Batches) itemExecution(exec *prompty.PromptExecution, capability string) (*prompty.PromptExecution, error) {
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
		working.ModelOptions = &prompty.ModelOptions{
			Model:            "",
			Temperature:      nil,
			MaxTokens:        nil,
			TopP:             nil,
			Stop:             nil,
			ProviderSettings: nil,
		}
	}
	if working.ModelOptions.Model == "" {
		working.ModelOptions.Model = b.base.defaultModel
	}
	working.Requirements.Capabilities = append(working.Requirements.Capabilities, capability)
	return working, nil
}

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *Batches) Submit(ctx context.Context, input prompty.BatchSubmitRequest) (*prompty.BatchResult, error) {
	if len(input.Items) == 0 || len(input.Items) > batchMaxItems {
		return nil, prompty.ErrBatch
	}
	if err := b.fileScope(); err != nil {
		return nil, err
	}
	if err := b.fileCapture(input.Options.Capture); err != nil {
		return nil, err
	}
	handle := prompty.BatchHandle{ProviderID: "", ProviderStatus: "", Items: nil, Resources: nil,
		Format:         prompty.BatchWireFormat,
		Provider:       wireOpenai,
		Endpoint:       b.base.endpoint,
		AccountBinding: b.account,
		Status:         prompty.BatchUnknown,
	}
	result := new(prompty.BatchResult)
	var content bytes.Buffer
	var lines []json.RawMessage
	type canonicalItem struct {
		Binding   prompty.BatchItemBinding
		Execution prompty.PromptExecutionWire
	}
	var canonical []canonicalItem
	var originals []*prompty.PromptExecution
	seen := make(map[string]bool)
	for _, item := range input.Items {
		if strings.TrimSpace(item.ID) == "" || seen[item.ID] || strings.TrimSpace(item.AttemptID) == "" ||
			item.AttemptID == item.ParentAttemptID ||
			(item.ParentAttemptID != "" && strings.TrimSpace(item.ParentAttemptID) == "") {
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
		if len(handle.Items) != 0 && prepared.Diagnostics.Profile.Model != handle.Items[0].Identity.Model {
			return nil, prompty.ErrBatchScope
		}
		original := item.Execution.Clone()
		original.ModelOptions = exec.ModelOptions
		originals = append(originals, original)
		digest, err := prompty.BatchExecutionDigest(original)
		if err != nil {
			return nil, err
		}
		binding := prompty.BatchItemBinding{
			ID:              item.ID,
			OriginalID:      item.OriginalID,
			AttemptID:       item.AttemptID,
			ParentAttemptID: item.ParentAttemptID,
			Identity:        prepared.Diagnostics.Profile,
			ExecutionDigest: digest,
		}
		handle.Items = append(handle.Items, binding)
		result.Diagnostics = append(result.Diagnostics, prepared.Diagnostics)
		encoded, err := json.Marshal(struct {
			ID     string                       `json:"custom_id"`
			Method string                       `json:"method"`
			URL    string                       `json:"url"`
			Body   *sdk.ChatCompletionNewParams `json:"body"`
		}{
			ID: item.ID, Method: http.MethodPost, URL: batchChatEndpoint, Body: prepared.Request,
		})
		if err != nil || len(encoded)+1 > batchMaxInputBytes-content.Len() {
			return nil, prompty.ErrBatch
		}
		_, _ = content.Write(encoded)
		_ = content.WriteByte('\n')
		lines = append(lines, encoded)
		wire, err := prompty.MarshalExecution(original, prompty.WirePolicy{State: prompty.StatePreserve})
		if err != nil {
			return nil, err
		}
		canonical = append(canonical, canonicalItem{Binding: binding, Execution: wire})
	}
	capture, err := adapter.NewBatchCapture(
		"openai.batch.submit",
		"github.com/openai/openai-go/v3",
		handle.Items[0].Identity,
		input.Options,
	)
	if err != nil {
		return result, err
	}
	planned := struct {
		Purpose          string
		Filename         string
		MediaType        string
		Lines            []json.RawMessage
		Endpoint         string
		CompletionWindow string
	}{
		Purpose: wireBatch, Filename: wireBatchJsonl, MediaType: wireApplicationJsonl,
		Lines: lines, Endpoint: batchChatEndpoint, CompletionWindow: "24h",
	}
	if err = capture.Before(ctx, canonical, planned); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = b.validateSubmitState(originals, handle.Items); err != nil {
		return capture.Finish(ctx, result, err)
	}
	capture.MarkDispatched()
	uploaded, uploadErr := b.files.Upload(
		ctx,
		prompty.BatchFileUpload{
			Content:   bytes.NewReader(content.Bytes()),
			Filename:  wireBatchJsonl,
			MediaType: wireApplicationJsonl,
			Purpose:   wireBatch,
		},
	)
	var providerErr error
	if uploaded != nil {
		if uploaded.Resource.ProviderID != "" {
			result.Resources = append(result.Resources, uploaded.Resource)
		}
		providerErr = capture.Provider(ctx, uploaded.SourceJSON, uploaded.HTTPStatus, json.Valid(uploaded.SourceJSON))
	} else {
		providerErr = capture.Provider(ctx, nil, 0, false)
	}
	if uploadErr != nil || providerErr != nil {
		return capture.Finish(ctx, result, errors.Join(uploadErr, providerErr))
	}
	if uploaded == nil || uploaded.Resource.Kind != wireInput ||
		!batchPathID.MatchString(uploaded.Resource.ProviderID) {
		return capture.Finish(ctx, result, prompty.ErrBatchFileUnknown)
	}
	handle.Resources = append(handle.Resources, uploaded.Resource)
	params := sdk.BatchNewParams{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
		CompletionWindow: sdk.BatchNewParamsCompletionWindow24h,
		Endpoint:         sdk.BatchNewParamsEndpoint(batchChatEndpoint),
		InputFileID:      uploaded.Resource.ProviderID,
	}
	if err = capture.Emit(ctx, prompty.CaptureTranslatedRequest, params); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	source := new(responseSource)
	if err = b.validateSubmitState(originals, handle.Items); err != nil {
		return capture.Finish(ctx, result, err)
	}
	result.SubmissionAttempted = true
	raw, callErr := b.base.client.Batches.New(
		ctx,
		params,
		option.WithMaxRetries(0),
		option.WithMiddleware(source.observe),
	)
	providerErr = capture.Provider(ctx, source.data, source.status, source.complete)
	if callErr != nil {
		if identity := batchAcceptedIdentity(source); identity != "" {
			handle.ProviderID = identity
			result.Handle = &handle
		}
		cause := prompty.ErrBatchUnknown
		if batchMutationRejected(source.status) {
			cause = prompty.ErrBatchRejected
		}
		return capture.Finish(ctx, result, errors.Join(cause, ctx.Err(), providerErr))
	}
	err = parseOpenAIBatch(raw, &handle)
	if err != nil && handle.ProviderID == "" {
		handle.ProviderID = batchAcceptedIdentity(source)
	}
	if handle.ProviderID != "" {
		result.Handle = &handle
	}
	result.Resources = append([]prompty.BatchResourceRef(nil), handle.Resources...)
	return capture.Finish(ctx, result, errors.Join(err, providerErr))
}

func (b *Batches) validateSubmitState(originals []*prompty.PromptExecution, items []prompty.BatchItemBinding) error {
	for i, original := range originals {
		if err := adapter.ValidateDispatchState(b.base, original, items[i].Identity); err != nil {
			return err
		}
	}
	return nil
}

func (b *Batches) validScope(handle prompty.BatchHandle) error {
	if err := prompty.ValidateBatchHandle(handle); err != nil {
		return err
	}
	if b.base.client == nil {
		return adapter.ErrNoClient
	}
	if handle.Provider != wireOpenai || handle.Endpoint != b.base.endpoint || handle.AccountBinding != b.account ||
		strings.TrimSpace(b.account) == "" {
		return prompty.ErrBatchScope
	}
	if !batchPathID.MatchString(handle.ProviderID) {
		return prompty.ErrBatch
	}
	for _, resource := range handle.Resources {
		if !batchPathID.MatchString(resource.ProviderID) ||
			(resource.Kind != wireInput && resource.Kind != wireOutput && resource.Kind != wireError) {
			return prompty.ErrBatch
		}
	}
	return nil
}

func (b *Batches) operationReports(
	handle prompty.BatchHandle,
	capability string,
) ([]prompty.ExecutionDiagnostics, error) {
	if err := b.validScope(handle); err != nil {
		return nil, err
	}
	var reports []prompty.ExecutionDiagnostics
	for _, item := range handle.Items {
		report, err := adapter.PreflightOperation(b.base.RuntimeDescriptor(), item.Identity.Model,
			[]string{capability}, nil)
		if err != nil {
			return nil, err
		}
		if report.Profile != item.Identity || item.Identity.Model != handle.Items[0].Identity.Model {
			return nil, prompty.ErrBatchScope
		}
		reports = append(reports, report)
	}
	return reports, nil
}

func (b *Batches) Get(
	ctx context.Context,
	handle prompty.BatchHandle,
	options ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return b.observe(ctx, handle, false, options...)
}

func (b *Batches) Cancel(
	ctx context.Context,
	handle prompty.BatchHandle,
	options ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return b.observe(ctx, handle, true, options...)
}

func (b *Batches) observe(
	ctx context.Context,
	handle prompty.BatchHandle,
	cancel bool,
	options ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	handle = prompty.CloneBatchHandle(handle)
	if len(options) > 1 {
		return nil, prompty.ErrBatch
	}
	var selected prompty.BatchOperationOptions
	if len(options) == 1 {
		selected = options[0]
	}
	capability, operation := prompty.CapabilityBatchInspect, "openai.batch.inspect"
	if cancel {
		capability, operation = prompty.CapabilityBatchCancel, "openai.batch.cancel"
	}
	reports, err := b.operationReports(handle, capability)
	if err != nil {
		return nil, err
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
		operation,
		"github.com/openai/openai-go/v3",
		handle.Items[0].Identity,
		selected,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, handle, map[string]string{"batch_id": handle.ProviderID}); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	capture.MarkDispatched()
	source := new(responseSource)
	var raw *sdk.Batch
	if cancel {
		raw, err = b.base.client.Batches.Cancel(
			ctx,
			handle.ProviderID,
			option.WithMaxRetries(0),
			option.WithMiddleware(source.observe),
		)
	} else {
		raw, err = b.base.client.Batches.Get(
			ctx, handle.ProviderID, option.WithMaxRetries(0), option.WithMiddleware(source.observe),
		)
	}
	providerErr := capture.Provider(ctx, source.data, source.status, source.complete)
	if err != nil {
		switch {
		case cancel && batchMutationRejected(source.status):
			err = prompty.ErrBatchRejected
		case cancel:
			handle.Status = prompty.BatchUnknown
			err = prompty.ErrBatchUnknown
		default:
			err = prompty.ErrBatchUnavailable
		}
		return capture.Finish(ctx, result, errors.Join(err, ctx.Err(), providerErr))
	}
	err = parseOpenAIBatch(raw, &handle)
	if cancel && errors.Is(err, prompty.ErrBatchUnknown) {
		handle.Status = prompty.BatchUnknown
	}
	result.Resources = append([]prompty.BatchResourceRef(nil), handle.Resources...)
	return capture.Finish(ctx, result, errors.Join(err, providerErr))
}

var _ prompty.BatchManager = (*Batches)(nil)

func batchMutationRejected(status int) bool {
	switch status {
	case http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusForbidden,
		http.StatusNotFound,
		http.StatusRequestEntityTooLarge,
		http.StatusUnsupportedMediaType,
		http.StatusUnprocessableEntity,
		http.StatusTooManyRequests:
		return true
	default:
		return false
	}
}
