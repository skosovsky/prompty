package openai

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Responses is a separate caller-driven remote response adapter. It does not
// embed Chat Completions execution or inherit its capability profiles.
type Responses struct {
	base           *Adapter
	accountBinding string
}

func NewResponses(accountBinding string, opts ...Option) *Responses {
	return &Responses{base: New(opts...), accountBinding: accountBinding}
}

func (r *Responses) RuntimeDescriptor() adapter.RuntimeDescriptor {
	desc := r.base.RuntimeDescriptor()
	desc.Name = "openai.responses"
	desc.ReasoningSettings = []string{wireReasoningEffort, "reasoning_summary"}
	desc.HostedToolCapabilities = map[string]string{HostedWebSearch: prompty.CapabilityHostedWebSearch,
		HostedFileSearch: prompty.CapabilityHostedFileSearch}
	desc.MessageAnnotationCapabilities = map[string][]string{
		hostedHistoryAnnotation: {
			prompty.CapabilityProviderState,
		},
		hostedItemAnnotation: {prompty.CapabilityProviderState},
	}
	return desc
}

var responseResourceID = regexp.MustCompile(`^resp_[A-Za-z0-9_-]+$`)

func (r *Responses) preflightMode(
	exec *prompty.PromptExecution, mode prompty.StreamMode, capabilities ...string,
) (prompty.ExecutionDiagnostics, error) {
	var zero prompty.ExecutionDiagnostics
	if r.base.client == nil {
		return zero, adapter.ErrNoClient
	}
	if strings.TrimSpace(r.accountBinding) == "" {
		return zero, prompty.ErrRemoteResponseScope
	}
	if err := adapter.ValidateExecution(exec); err != nil {
		return zero, err
	}
	desc := r.RuntimeDescriptor()
	if strings.TrimSpace(desc.Endpoint) == "" {
		return zero, adapter.ErrNoRuntimeDescriptor
	}
	working := exec.Clone()
	working.Requirements.Capabilities = append(working.Requirements.Capabilities, capabilities...)
	settings, err := prompty.ProviderSettingsMap(working.ModelOptions)
	if err != nil {
		return zero, err
	}
	for _, key := range desc.ReasoningSettings {
		if _, exists := settings[key]; exists {
			working.Requirements.Capabilities = append(working.Requirements.Capabilities, prompty.CapabilityReasoning)
			break
		}
	}
	return adapter.Preflight(desc, working, mode, mode == prompty.StreamNative)
}

func (r *Responses) validateHandle(handle prompty.RemoteResponseHandle, profile prompty.ProfileIdentity) error {
	if err := prompty.ValidateRemoteResponseHandle(handle); err != nil {
		return err
	}
	if !responseResourceID.MatchString(handle.ProviderID) {
		return prompty.ErrRemoteResponse
	}
	if handle.AccountBinding != r.accountBinding || handle.Identity != profile {
		return prompty.ErrRemoteResponseScope
	}
	return nil
}

type preparedResponseCreate struct {
	execution *prompty.PromptExecution
	canonical prompty.RemoteResponseRequest
	report    prompty.ExecutionDiagnostics
	request   preparedRemoteResponse
	handle    prompty.RemoteResponseHandle
}

// prepareCreate owns all mutable request inputs and performs no provider dispatch.
//
//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (r *Responses) prepareCreate(
	input prompty.RemoteResponseRequest,
	mode prompty.StreamMode,
) (*preparedResponseCreate, error) {
	if mode == prompty.StreamNative && input.Background {
		return nil, adapter.ErrUnsupportedCombination
	}
	if input.Store == nil {
		return nil, prompty.ErrRemoteResponse
	}
	if err := adapter.ValidateExecution(input.Execution); err != nil {
		return nil, err
	}
	exec := input.Execution.Clone()
	store := *input.Store
	input.Store, input.Execution = &store, exec
	if input.Previous != nil {
		previous := prompty.CloneRemoteResponseHandle(*input.Previous)
		input.Previous = &previous
	}
	capabilities := []string{prompty.CapabilityResponseCreate}
	if input.StrictTools {
		if len(exec.Tools) == 0 {
			return nil, prompty.ErrRemoteResponse
		}
		capabilities = append(capabilities, prompty.CapabilityStrictTools)
	}
	for _, message := range exec.Messages {
		if len(message.MessageAnnotations) != 0 {
			items, historyErr := HostedSearchItems(
				&prompty.Response{
					Model:                   "",
					Content:                 nil,
					ProviderState:           nil,
					Annotations:             nil,
					ContinuationUnavailable: false,
					Usage: prompty.Usage{
						PromptTokens:              0,
						CompletionTokens:          0,
						TotalTokens:               0,
						PromptTokensCached:        0,
						PromptTokensCacheCreation: 0,
						CompletionTokensReasoning: 0,
						Known:                     nil,
						Mode:                      "",
					},
					FinishReason: "",
					Outcome:      "",
					Diagnostics: prompty.ExecutionDiagnostics{
						Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
						Snapshot:   "",
						Provenance: "",
						Override:   false,
						Checks:     nil,
						Schemas:    nil,
						Capture:    nil,
						Cache:      nil,
					},
					MessageAnnotations: message.MessageAnnotations,
				},
			)
			if historyErr != nil {
				return nil, historyErr
			}
			for _, item := range items {
				if item.Kind == HostedWebSearch {
					capabilities = append(capabilities, prompty.CapabilityHostedWebSearch)
				} else {
					capabilities = append(capabilities, prompty.CapabilityHostedFileSearch)
				}
			}
		}
		for _, part := range message.Content {
			switch part.(type) {
			case prompty.ToolCallPart, *prompty.ToolCallPart, prompty.ToolResultPart, *prompty.ToolResultPart:
				capabilities = append(capabilities, prompty.CapabilityTools)
			}
		}
	}
	hosted, hostedCaps, err := cloneHostedRequests(exec.HostedTools)
	if err != nil {
		return nil, err
	}
	exec.HostedTools = hosted
	// Canonical capture owns the caller's intent before checked preparation
	// drops advisory controls from the independent dispatch execution.
	canonicalInput := input
	canonicalInput.Execution = exec.Clone()
	capabilities = append(capabilities, hostedCaps...)
	if input.Previous != nil {
		capabilities = append(capabilities, prompty.CapabilityResponseContinuation)
	}
	if input.Background {
		capabilities = append(capabilities, prompty.CapabilityResponseBackground)
	}
	report, err := r.preflightMode(exec, mode, capabilities...)
	if err != nil {
		return nil, err
	}
	report.Cache, err = adapter.PrepareCacheControls(r, exec, capabilities)
	if err != nil {
		return nil, err
	}
	if input.Previous != nil {
		if err = r.validateHandle(*input.Previous, report.Profile); err != nil {
			return nil, err
		}
		if input.Previous.Status != prompty.RemoteResponseCompleted {
			return nil, prompty.ErrRemoteResponseUnavailable
		}
	}
	prepared, err := r.prepareRequest(exec, input, &report)
	if err != nil {
		return nil, err
	}
	handle := prompty.RemoteResponseHandle{ProviderID: "", OutputSchemaDigest: "", HostedTools: nil, ToolCalls: nil,
		Format: prompty.RemoteResponseWireFormat, Identity: report.Profile, AccountBinding: r.accountBinding,
		Status: prompty.RemoteResponseUnknown, Background: input.Background, Store: *input.Store,
		ProviderStateRequired: responseRequiresOutputState(exec),
		ToolSchemaDigests:     prepared.tools.digests, ForcedTool: prepared.tools.forced,
	}
	for _, tool := range exec.HostedTools {
		handle.HostedTools = append(handle.HostedTools, tool.Kind)
	}
	if prepared.schema != nil {
		handle.OutputSchemaDigest = responseSchemaBindingDigest(prepared.schema)
	}
	return &preparedResponseCreate{execution: exec, canonical: canonicalInput, report: report,
		request: prepared, handle: handle}, nil
}

func (r *Responses) Create(
	ctx context.Context,
	input prompty.RemoteResponseRequest,
) (*prompty.RemoteResponseResult, error) {
	creation, err := r.prepareCreate(input, "")
	if err != nil {
		return nil, err
	}
	exec, report := creation.execution, creation.report
	canonicalInput, prepared, handle := creation.canonical, creation.request, creation.handle
	result := &prompty.RemoteResponseResult{Handle: nil, Response: nil, Diagnostics: report}
	capture, err := newResponseCapture("response.create", report, exec.Requirements.Capture, exec.Capture)
	if err != nil {
		return result, err
	}
	if err = capture.before(ctx, canonicalInput, prepared.params); err != nil {
		return capture.finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.finish(ctx, result, err)
	}
	if err = prompty.ValidateContinuation(exec.Messages, report.Profile, time.Now()); err != nil {
		return capture.finish(ctx, result, err)
	}
	capture.dispatched = true
	source := new(responseSource)
	capture.source = source
	raw, callErr := r.base.client.Responses.New(
		ctx,
		prepared.params,
		option.WithMaxRetries(0),
		option.WithMiddleware(source.observe),
	)
	if callErr != nil {
		retainUnknownResponseIdentity(source, handle, result)
		return capture.finish(ctx, result, errors.Join(prompty.ErrRemoteResponseUnknown, ctx.Err()))
	}
	result, err = parseRemoteResponse(raw, handle, result, prepared.schema, prepared.tools)
	return capture.finish(ctx, result, err)
}

func (r *Responses) Get(
	ctx context.Context,
	handle prompty.RemoteResponseHandle,
	options ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	return r.observe(ctx, handle, false, options...)
}

func (r *Responses) Cancel(
	ctx context.Context,
	handle prompty.RemoteResponseHandle,
	options ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	return r.observe(ctx, handle, true, options...)
}

//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (r *Responses) observe(
	ctx context.Context,
	handle prompty.RemoteResponseHandle,
	cancel bool,
	options ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	handle = prompty.CloneRemoteResponseHandle(handle)
	if len(options) > 1 {
		return nil, prompty.ErrRemoteResponse
	}
	var selected prompty.RemoteResponseOperationOptions
	if len(options) == 1 {
		selected = options[0]
	}
	if err := prompty.ValidateRemoteResponseHandle(handle); err != nil {
		return nil, err
	}
	exec := prompty.SimplePrompt("")
	exec.ModelOptions = &prompty.ModelOptions{
		Temperature:      nil,
		MaxTokens:        nil,
		TopP:             nil,
		Stop:             nil,
		ProviderSettings: nil,
		Model:            handle.Identity.Model,
	}
	exec.Validator = selected.Validator
	exec.Requirements.Capture, exec.Capture = selected.Capture, selected.Binding
	capability, operation := prompty.CapabilityResponseRetrieve, "response.retrieve"
	if cancel {
		capability, operation = prompty.CapabilityResponseCancel, "response.cancel"
		if !handle.Background {
			return nil, adapter.ErrUnsupportedCapability
		}
	}
	include, hostedCaps, err := hostedIncludes(handle.HostedTools)
	if err != nil {
		return nil, err
	}
	if handle.ProviderStateRequired && !cancel {
		hostedCaps = append(hostedCaps, prompty.CapabilityProviderState)
		include = append(include, responses.ResponseIncludableReasoningEncryptedContent)
	}
	if r.base.client == nil {
		return nil, adapter.ErrNoClient
	}
	if strings.TrimSpace(r.accountBinding) == "" {
		return nil, prompty.ErrRemoteResponseScope
	}
	report, err := adapter.PreflightOperation(r.RuntimeDescriptor(), handle.Identity.Model,
		append([]string{capability}, hostedCaps...), nil)
	if err != nil {
		return nil, err
	}
	if err = r.validateHandle(handle, report.Profile); err != nil {
		return nil, err
	}
	// Inspect accepted output without reauthorizing native generation support.
	// Rebind the original schema only after operation capability validation.
	exec.ResponseFormat = selected.OutputSchema
	exec.Tools = selected.Tools
	tools, err := r.restoreTools(exec, handle, &report)
	if err != nil {
		return nil, err
	}
	schema, err := r.restoreOutputSchema(exec, handle, &report)
	if err != nil {
		return nil, err
	}
	result := &prompty.RemoteResponseResult{Response: nil, Handle: &handle, Diagnostics: report}
	capture, err := newResponseCapture(operation, report, selected.Capture, selected.Binding)
	if err != nil {
		return result, err
	}
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	query := responses.ResponseGetParams{
		Include: include,
	}
	translated := map[string]any{"response_id": handle.ProviderID}
	if !cancel {
		translated["include"] = include
	}
	if err = capture.before(ctx, handle, translated); err != nil {
		return capture.finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.finish(ctx, result, err)
	}
	capture.dispatched = true
	source := new(responseSource)
	capture.source = source
	var raw *responses.Response
	if cancel {
		raw, err = r.base.client.Responses.Cancel(
			ctx,
			handle.ProviderID,
			option.WithMaxRetries(0),
			option.WithMiddleware(source.observe),
		)
	} else {
		raw, err = r.base.client.Responses.Get(
			ctx, handle.ProviderID, query,
			option.WithMaxRetries(0), option.WithMiddleware(source.observe),
		)
	}
	if err != nil {
		var apiErr *sdk.Error
		if errors.As(err, &apiErr) &&
			(apiErr.StatusCode == http.StatusNotFound || apiErr.StatusCode == http.StatusGone) {
			return capture.finish(ctx, result, prompty.ErrRemoteResponseUnavailable)
		}
		if cancel {
			handle.Status = prompty.RemoteResponseUnknown
			return capture.finish(ctx, result, errors.Join(prompty.ErrRemoteResponseUnknown, ctx.Err()))
		}
		return capture.finish(ctx, result, errors.Join(prompty.ErrRemoteResponseUnavailable, ctx.Err()))
	}
	result, err = parseRemoteResponse(raw, handle, result, schema, tools)
	return capture.finish(ctx, result, err)
}

var _ prompty.RemoteResponseManager = (*Responses)(nil)
