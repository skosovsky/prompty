package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// ErrSyntheticOutput identifies a structural synthetic-output contract failure.
var ErrSyntheticOutput = errors.New("anthropic: synthetic output contract")

// SyntheticOutputError carries a stable reason without provider payloads.
type SyntheticOutputError struct{ Reason string }

func (e *SyntheticOutputError) Error() string {
	return fmt.Sprintf("%s: %s", ErrSyntheticOutput, e.Reason)
}
func (*SyntheticOutputError) Unwrap() error { return ErrSyntheticOutput }

// SyntheticOutputAdapter is an explicit received-result-validation profile.
// It never silently falls back from native output or executes an output handler.
// Checked execution through adapter.NewClient validates the original schema.
type SyntheticOutputAdapter struct{ base *Adapter }

// SyntheticRequest owns one request's output binding. SDK payloads are private
// so later caller mutation cannot change the expected output name.
type SyntheticRequest struct {
	params     *anthropic.MessageNewParams
	outputName string
}

// SyntheticResponse retains the provider message and its request-bound name.
// It is not an ordinary tool batch. ParseResponse alone does not validate schema.
type SyntheticResponse struct {
	message    *anthropic.Message
	outputName string
}

// MarshalJSON captures the semantic SDK request, not internal binding metadata.
func (r *SyntheticRequest) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.params)
}

// MarshalJSON captures the semantic provider message, not transport bytes.
func (r *SyntheticResponse) MarshalJSON() ([]byte, error) {
	if r == nil {
		return []byte("null"), nil
	}
	return json.Marshal(r.message)
}

// RawJSON preserves the SDK-observed provider payload for semantic capture.
// It is not a promise of original transport bytes.
func (r *SyntheticResponse) RawJSON() string {
	if r == nil || r.message == nil {
		return ""
	}
	return r.message.RawJSON()
}

// NewSyntheticOutput requires explicit OutputSyntheticTool selection on every
// execution and caller-owned capability knowledge for the actual destination.
func NewSyntheticOutput(opts ...Option) *SyntheticOutputAdapter {
	return &SyntheticOutputAdapter{base: New(opts...)}
}

func (a *SyntheticOutputAdapter) RuntimeDescriptor() adapter.RuntimeDescriptor {
	return a.base.RuntimeDescriptor()
}

// CaptureCoverage identifies the SDK used for semantic sync/buffered capture.
// This profile has no native stream hook, transport-byte or physical-attempt coverage.
func (*SyntheticOutputAdapter) CaptureCoverage(prompty.StreamMode) prompty.CaptureCoverage {
	return prompty.CaptureCoverage{
		SDKBuild: adapter.DependencyBuildIdentity("github.com/anthropics/anthropic-sdk-go"),
	}
}

// SupportsSyntheticOutput declares an actual dedicated output-tool mapper.
func (*SyntheticOutputAdapter) SupportsSyntheticOutput() bool { return true }

// SupportsForcedTool declares the native named choice used for output extraction.
func (*SyntheticOutputAdapter) SupportsForcedTool() bool { return true }

// ValidateProviderState delegates exact incoming Messages continuation checks.
func (a *SyntheticOutputAdapter) ValidateProviderState(
	exec *prompty.PromptExecution, destination prompty.ProfileIdentity,
) error {
	return a.base.ValidateProviderState(exec, destination)
}

// ValidateCacheControl preserves the ordinary Messages request-cache port.
func (a *SyntheticOutputAdapter) ValidateCacheControl(policy *prompty.CachePolicy) error {
	return a.base.ValidateCacheControl(policy)
}

// MapSchema preserves canonical object schema semantics for the output tool.
func (a *SyntheticOutputAdapter) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	return a.base.MapSchema(request)
}

var syntheticToolName = regexp.MustCompile(`^[a-zA-Z0-9_-]{1,128}$`)

// Translate rejects conflicts without dropping caller tools or reasoning.
func (a *SyntheticOutputAdapter) Translate(exec *prompty.PromptExecution) (*SyntheticRequest, error) {
	if err := adapter.ValidateExecution(exec); err != nil {
		return nil, err
	}
	if exec.Requirements.OutputMode != prompty.OutputSyntheticTool {
		return nil, prompty.ErrOutputMode
	}
	if len(exec.Tools) != 0 || exec.ForcedTool != "" {
		return nil, &SyntheticOutputError{Reason: "real_tools_conflict"}
	}
	if !syntheticToolName.MatchString(exec.ResponseFormat.Name) {
		return nil, &SyntheticOutputError{Reason: "invalid_output_name"}
	}
	schema, err := prompty.SchemaDocumentAsMap(exec.ResponseFormat.Schema)
	if err != nil {
		return nil, err
	}
	if schema["type"] != "object" {
		return nil, &SyntheticOutputError{Reason: "object_schema_required"}
	}
	working := exec.Clone()
	working.ResponseFormat = nil
	working.Requirements.OutputMode = ""
	params, err := a.base.Translate(working)
	if err != nil {
		return nil, err
	}
	if params.Thinking.OfEnabled != nil {
		return nil, &SyntheticOutputError{Reason: "forced_choice_thinking_conflict"}
	}
	name := exec.ResponseFormat.Name
	params.Tools = []anthropic.ToolUnionParam{
		anthropic.ToolUnionParamOfTool(toolSchemaFromParameters(sdkSchemaMapNumbers(schema)), name),
	}
	params.ToolChoice = anthropic.ToolChoiceParamOfTool(name)
	return &SyntheticRequest{params: params, outputName: name}, nil
}

// Execute makes one SDK attempt; unknown acceptance is not retried automatically.
func (a *SyntheticOutputAdapter) Execute(ctx context.Context, req *SyntheticRequest) (*SyntheticResponse, error) {
	if req == nil || req.params == nil || req.outputName == "" {
		return nil, adapter.ErrInvalidResponse
	}
	if a.base.client == nil {
		return nil, adapter.ErrNoClient
	}
	message, err := a.base.client.Messages.New(ctx, *req.params, option.WithMaxRetries(0))
	if message == nil {
		return nil, err
	}
	return &SyntheticResponse{message: message, outputName: req.outputName}, err
}

// ParseResponse extracts JSON text, never a handler-eligible ToolCallPart.
// Original schema validation is performed by the checked client's finalizer.
func (a *SyntheticOutputAdapter) ParseResponse(raw *SyntheticResponse) (*prompty.Response, error) {
	desc := a.RuntimeDescriptor()
	destination := prompty.ProfileIdentity{Provider: desc.Name, Endpoint: desc.Endpoint, Model: desc.DefaultModel}
	if raw != nil && raw.message != nil {
		destination.Model = string(raw.message.Model)
	}
	return a.ParseResponseForProfile(raw, destination)
}

// ParseResponseForProfile retains unexpected signed reasoning as scoped partial
// state before rejecting this base output profile's non-output content.
func (*SyntheticOutputAdapter) ParseResponseForProfile(
	raw *SyntheticResponse,
	destination prompty.ProfileIdentity,
) (*prompty.Response, error) {
	if raw == nil || raw.message == nil || raw.outputName == "" {
		return nil, adapter.ErrInvalidResponse
	}
	message := raw.message
	response := prompty.NewResponse(nil)
	response.Outcome = anthropicOutcome(message.StopReason)
	response.FinishReason = string(message.StopReason)
	usage, usageErr := usageFromAnthropic(message.Usage)
	response.Usage = usage
	count := 0
	reason := ""
	for _, block := range message.Content {
		switch block.Type {
		case "tool_use":
			count++
			response.Content = append(response.Content, prompty.TextPart{Text: string(block.Input)})
			if block.Name != raw.outputName {
				reason = "wrong_output_name"
			}
			if block.ID == "" {
				reason = "missing_call_id"
			}
			if object, err := prompty.SchemaDocumentAsMap(
				block.Input,
			); err != nil || object == nil ||
				!uniqueSyntheticJSON(block.Input) {
				reason = "invalid_arguments_object"
			}
		case "thinking", "redacted_thinking":
			state, part, err := thinkingState(block, len(response.Content), destination)
			if err != nil {
				return response, errors.Join(err, usageErr)
			}
			response.ProviderState = append(response.ProviderState, state)
			response.Content = append(response.Content, part)
			reason = "unexpected_content"
		case "text":
			index := len(response.Content)
			response.Content = append(response.Content, prompty.TextPart{Text: block.Text})
			for _, citation := range block.Citations {
				payload := prompty.JSONDocument(citation.RawJSON())
				if len(payload) == 0 {
					var err error
					payload, err = prompty.MarshalJSONDocument(citation)
					if err != nil {
						return response, errors.Join(err, usageErr)
					}
				}
				response.Annotations = append(response.Annotations, prompty.PartAnnotation{
					PartIndex: index, Kind: "anthropic.citation", Value: payload,
				})
			}
			reason = "unexpected_content"
		default:
			reason = "unexpected_content"
		}
	}
	if response.Outcome == prompty.OutcomeRefusal || response.Outcome == prompty.OutcomeIncomplete {
		return response, errors.Join(
			&prompty.OutcomeError{Outcome: response.Outcome, FinishReason: response.FinishReason},
			usageErr,
		)
	}
	if count == 0 {
		reason = "missing_output_call"
	}
	if count > 1 {
		reason = "multiple_output_calls"
	}
	if response.Outcome != prompty.OutcomeToolCalls && reason == "" {
		reason = "invalid_finish"
	}
	if reason != "" {
		return response, errors.Join(&SyntheticOutputError{Reason: reason}, usageErr)
	}
	response.Outcome = prompty.OutcomeCompleted
	return response, usageErr
}

var _ adapter.ProviderAdapter[*SyntheticRequest, *SyntheticResponse] = (*SyntheticOutputAdapter)(nil)
