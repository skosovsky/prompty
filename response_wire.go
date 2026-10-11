package prompty

import (
	"encoding/json"
	"errors"
	"slices"
)

const ResponseWireFormat = "prompty.response"

var (
	ErrWireFormat = errors.New("prompty: unsupported wire format; explicit migration required")
	ErrWirePolicy = errors.New("prompty: explicit state projection policy required")
)

type StateProjection string

const (
	StatePreserve StateProjection = "preserve"
	StateOmit     StateProjection = "omit"
)

// WirePolicy requires an explicit host choice about opaque-state persistence.
// Omit is safe for logging state, but is not a secrets filter for prompt/media.
type WirePolicy struct{ State StateProjection }

type ResponseWire struct {
	Model                   string               `json:"model,omitempty"`
	Format                  string               `json:"format"`
	Content                 []ContentPartWire    `json:"content"`
	ProviderState           []PartState          `json:"provider_state,omitempty"`
	Annotations             []PartAnnotation     `json:"annotations,omitempty"`
	MessageAnnotations      []MessageAnnotation  `json:"message_annotations,omitempty"`
	ContinuationUnavailable bool                 `json:"continuation_unavailable,omitempty"`
	Usage                   Usage                `json:"usage"`
	FinishReason            string               `json:"finish_reason,omitempty"`
	Outcome                 Outcome              `json:"outcome"`
	Diagnostics             ExecutionDiagnostics `json:"diagnostics"`
}

func projectMessages(messages []ChatMessage, policy WirePolicy) ([]ChatMessage, error) {
	if policy.State != StatePreserve && policy.State != StateOmit {
		return nil, ErrWirePolicy
	}
	out := cloneMessages(messages)
	for i := range out {
		message := &out[i]
		if err := ValidateMessageAnnotations(message.MessageAnnotations); err != nil {
			return nil, err
		}
		if err := ValidatePartAssociations(message.Content, message.ProviderState, message.Annotations); err != nil {
			return nil, err
		}
		if policy.State == StateOmit {
			for _, state := range message.ProviderState {
				message.ContinuationUnavailable = message.ContinuationUnavailable || state.Envelope.Required
			}
			message.ProviderState = nil
			message.MessageAnnotations = slices.DeleteFunc(
				message.MessageAnnotations,
				func(annotation MessageAnnotation) bool {
					if annotation.Scope == nil {
						return false
					}
					message.ContinuationUnavailable = message.ContinuationUnavailable || annotation.Required
					return true
				},
			)
		}
	}
	return out, nil
}

// AssistantMessage projects an isolated response into history without losing state.
func (r *Response) AssistantMessage() (ChatMessage, error) {
	if r == nil {
		return ChatMessage{}, ErrProviderState
	}
	if err := ValidateMessageAnnotations(r.MessageAnnotations); err != nil {
		return ChatMessage{}, err
	}
	if err := ValidatePartAssociations(r.Content, r.ProviderState, r.Annotations); err != nil {
		return ChatMessage{}, err
	}
	return ChatMessage{CachePolicy: nil, Provenance: nil, Metadata: nil, LayerKind: "",
		Role: RoleAssistant, Content: cloneContentParts(r.Content),
		ProviderState: clonePartStates(r.ProviderState), Annotations: clonePartAnnotations(r.Annotations),
		MessageAnnotations:      cloneMessageAnnotations(r.MessageAnnotations),
		ContinuationUnavailable: r.ContinuationUnavailable,
	}, nil
}

// MarshalResponse requires explicit preservation/omission of provider state.
func MarshalResponse(response *Response, policy WirePolicy) (ResponseWire, error) {
	if response == nil {
		return ResponseWire{}, ErrProviderState
	}
	if err := ValidateResponseMetadata(response); err != nil {
		return ResponseWire{}, err
	}
	message, err := response.AssistantMessage()
	if err != nil {
		return ResponseWire{}, err
	}
	messages, err := projectMessages([]ChatMessage{message}, policy)
	if err != nil {
		return ResponseWire{}, err
	}
	content, err := contentPartsToWire(messages[0].Content)
	if err != nil {
		return ResponseWire{}, err
	}
	usage := response.Usage
	usage.Known = slices.Clone(usage.Known)
	return ResponseWire{
		Format: ResponseWireFormat, Model: response.Model, Content: content, ProviderState: messages[0].ProviderState,
		Annotations: messages[0].Annotations, ContinuationUnavailable: messages[0].ContinuationUnavailable,
		MessageAnnotations: messages[0].MessageAnnotations,
		Usage:              usage, Outcome: response.Outcome, FinishReason: response.FinishReason,
		Diagnostics: CloneExecutionDiagnostics(response.Diagnostics),
	}, nil
}

// LoggingResponse excludes opaque state and marks unavailable continuation.
// Canonical prompt/media/annotations still require the caller's redaction policy.
func LoggingResponse(response *Response) (ResponseWire, error) {
	return MarshalResponse(response, WirePolicy{State: StateOmit})
}

// LoggingExecution is a non-continuable logging projection, not persistence wire.
func LoggingExecution(exec *PromptExecution) (PromptExecutionWire, error) {
	return MarshalExecution(exec, WirePolicy{State: StateOmit})
}

// UnmarshalResponse rejects legacy/corrupt wire and validates state associations.
func UnmarshalResponse(wire ResponseWire) (*Response, error) {
	if wire.Format != ResponseWireFormat {
		return nil, ErrWireFormat
	}
	if err := validateDiagnosticsEndpointIdentities(wire.Diagnostics); err != nil {
		return nil, err
	}
	if err := wire.Usage.Validate(); err != nil {
		return nil, err
	}
	if wire.Diagnostics.Capture != nil {
		if err := wire.Diagnostics.Capture.Validate(); err != nil {
			return nil, err
		}
	}
	serialized, encodeErr := json.Marshal(wire)
	if encodeErr != nil {
		return nil, encodeErr
	}
	if schemaErr := ValidateWireJSON(serialized, ResponseWireFormat); schemaErr != nil {
		return nil, schemaErr
	}
	content, err := contentPartsFromWire(wire.Content)
	if err != nil {
		return nil, err
	}
	if err := ValidatePartAssociations(content, wire.ProviderState, wire.Annotations); err != nil {
		return nil, err
	}
	if !validOutcome(wire.Outcome) {
		return nil, ErrResponseOutcome
	}
	if err := ValidateMessageAnnotations(wire.MessageAnnotations); err != nil {
		return nil, err
	}
	usage := wire.Usage
	usage.Known = slices.Clone(usage.Known)
	return &Response{
		Model:                   wire.Model,
		Content:                 content,
		ProviderState:           clonePartStates(wire.ProviderState),
		Annotations:             clonePartAnnotations(wire.Annotations),
		MessageAnnotations:      cloneMessageAnnotations(wire.MessageAnnotations),
		ContinuationUnavailable: wire.ContinuationUnavailable,
		Usage:                   usage,
		Outcome:                 wire.Outcome,
		FinishReason:            wire.FinishReason,
		Diagnostics:             CloneExecutionDiagnostics(wire.Diagnostics),
	}, nil
}
