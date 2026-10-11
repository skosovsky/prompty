package prompty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

const ProviderStateFormat = "prompty.provider-state"

var (
	ErrProviderState    = errors.New("prompty: invalid provider state")
	ErrStateDestination = errors.New("prompty: incompatible provider state destination")
	ErrStateExpired     = errors.New("prompty: expired provider state")
	ErrStateUnavailable = errors.New("prompty: required continuation state unavailable")
)

// ProviderEnvelope carries provider-defined continuation bytes, not credentials.
// Codec and payload interpretation belong to the provider module. The host owns
// retention and account authorization; scope never grants access to an account.
type ProviderEnvelope struct {
	Format    string          `json:"format"`
	Scope     ProfileIdentity `json:"scope"`
	Codec     string          `json:"codec"`
	Payload   []byte          `json:"payload"`
	Required  bool            `json:"required"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
}

// PartState binds one envelope to an exact content part; indices are never inferred.
type PartState struct {
	PartIndex int              `json:"part_index"`
	Envelope  ProviderEnvelope `json:"envelope"`
}

// PartAnnotation preserves provider-exposed citations/metadata with their association.
type PartAnnotation struct {
	PartIndex int          `json:"part_index"`
	Kind      string       `json:"kind"`
	Value     JSONDocument `json:"value"`
}

// MessageAnnotation preserves metadata associated with the whole message.
// Stream updates are ordered, not merged snapshots. Provider-relative indices
// inside Value are not canonical content-part indices and are never inferred.
// Payload interpretation and redaction belong to the provider module and host.
// Scope binds opaque metadata to one destination. Required marks continuation
// data that a receiving adapter must understand and preserve; it needs Scope.
type MessageAnnotation struct {
	Kind     string           `json:"kind"`
	Value    JSONDocument     `json:"value"`
	Scope    *ProfileIdentity `json:"scope,omitempty"`
	Required bool             `json:"required,omitempty"`
}

// HasScopedMessageAnnotations identifies opaque message-level dependencies
// without decoding provider payloads. It does not validate shape or grant support.
func HasScopedMessageAnnotations(annotations []MessageAnnotation) bool {
	return slices.ContainsFunc(annotations, func(annotation MessageAnnotation) bool { return annotation.Scope != nil })
}

func cloneMessageAnnotations(annotations []MessageAnnotation) []MessageAnnotation {
	out := slices.Clone(annotations)
	for i := range out {
		out[i].Value = CloneJSONDocument(annotations[i].Value)
		if annotations[i].Scope != nil {
			scope := *annotations[i].Scope
			out[i].Scope = &scope
		}
	}
	return out
}

// ValidateMessageAnnotations validates the envelope, not provider payloads.
func ValidateMessageAnnotations(annotations []MessageAnnotation) error {
	for _, annotation := range annotations {
		if strings.TrimSpace(annotation.Kind) == "" || !json.Valid(annotation.Value) {
			return fmt.Errorf("%w: invalid message annotation", ErrProviderState)
		}
		if annotation.Required && annotation.Scope == nil {
			return fmt.Errorf("%w: required message annotation has no scope", ErrProviderState)
		}
		if scope := annotation.Scope; scope != nil {
			if ValidateEndpointIdentity(scope.Endpoint) != nil || strings.TrimSpace(scope.Provider) == "" ||
				strings.TrimSpace(scope.Endpoint) == "" ||
				(strings.TrimSpace(scope.Model) == "" && strings.TrimSpace(scope.ModelFamily) == "") {
				return fmt.Errorf("%w: invalid message annotation scope", ErrProviderState)
			}
		}
	}
	return nil
}

func clonePartStates(states []PartState) []PartState {
	out := slices.Clone(states)
	for i, state := range out {
		out[i].Envelope.Payload = bytes.Clone(state.Envelope.Payload)
		if state.Envelope.ExpiresAt != nil {
			value := *state.Envelope.ExpiresAt
			out[i].Envelope.ExpiresAt = &value
		}
	}
	return out
}

func clonePartAnnotations(annotations []PartAnnotation) []PartAnnotation {
	out := slices.Clone(annotations)
	for i := range out {
		out[i].Value = CloneJSONDocument(annotations[i].Value)
	}
	return out
}

// ValidatePartAssociations rejects corrupt envelopes without interpreting their payload.
func ValidatePartAssociations(parts []ContentPart, states []PartState, annotations []PartAnnotation) error {
	seen := make(map[int]bool)
	for _, state := range states {
		if state.PartIndex < 0 || state.PartIndex >= len(parts) || seen[state.PartIndex] {
			return fmt.Errorf("%w: invalid or repeated part association", ErrProviderState)
		}
		seen[state.PartIndex] = true
		envelope := state.Envelope
		if err := ValidateEndpointIdentity(envelope.Scope.Endpoint); err != nil {
			return fmt.Errorf("%w: invalid endpoint identity", ErrProviderState)
		}
		if envelope.Format != ProviderStateFormat || strings.TrimSpace(envelope.Scope.Provider) == "" ||
			strings.TrimSpace(envelope.Scope.Endpoint) == "" ||
			(strings.TrimSpace(envelope.Scope.Model) == "" && strings.TrimSpace(envelope.Scope.ModelFamily) == "") ||
			strings.TrimSpace(envelope.Codec) == "" || len(envelope.Payload) == 0 {
			return fmt.Errorf("%w: envelope format/scope/codec/payload", ErrProviderState)
		}
	}
	for _, annotation := range annotations {
		if annotation.PartIndex < 0 || annotation.PartIndex >= len(parts) || strings.TrimSpace(annotation.Kind) == "" ||
			!json.Valid(annotation.Value) {
			return fmt.Errorf("%w: invalid annotation association/value", ErrProviderState)
		}
	}
	return nil
}

// ValidateContinuation is a local scope/expiry gate, not provider account authorization.
func ValidateContinuation(messages []ChatMessage, destination ProfileIdentity, now time.Time) error {
	for _, message := range messages {
		if err := ValidateMessageAnnotations(message.MessageAnnotations); err != nil {
			return err
		}
		if message.ContinuationUnavailable {
			return ErrStateUnavailable
		}
		if err := validateAnnotationDestination(message.MessageAnnotations, destination); err != nil {
			return err
		}
		if err := ValidatePartAssociations(message.Content, message.ProviderState, message.Annotations); err != nil {
			return err
		}
		for _, state := range message.ProviderState {
			scope := state.Envelope.Scope
			if scope.Provider != destination.Provider || scope.Endpoint != destination.Endpoint ||
				!stateModelMatches(scope, destination) {
				return ErrStateDestination
			}
			if state.Envelope.ExpiresAt != nil && !now.Before(*state.Envelope.ExpiresAt) {
				return ErrStateExpired
			}
		}
	}
	return nil
}

func validateAnnotationDestination(annotations []MessageAnnotation, destination ProfileIdentity) error {
	for _, annotation := range annotations {
		if scope := annotation.Scope; scope != nil &&
			(scope.Provider != destination.Provider || scope.Endpoint != destination.Endpoint || !stateModelMatches(*scope, destination)) {
			return ErrStateDestination
		}
	}
	return nil
}

func stateModelMatches(scope, destination ProfileIdentity) bool {
	if scope.ModelFamily != "" {
		return scope.ModelFamily == destination.ModelFamily
	}
	return scope.Model == destination.Model
}
