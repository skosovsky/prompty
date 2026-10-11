package anthropic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"time"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty"
)

const thinkingCodec = "anthropic.thinking.json"

// Pointer fields distinguish an explicitly empty thinking string from missing data.
type thinkingPayload struct {
	Type      string  `json:"type"`
	Thinking  *string `json:"thinking,omitempty"`
	Signature *string `json:"signature,omitempty"`
	Data      *string `json:"data,omitempty"`
}

func decodeThinking(payload []byte) (thinkingPayload, error) {
	var block thinkingPayload
	fields, err := thinkingFields(payload)
	if err != nil {
		return block, err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&block); err != nil {
		return block, fmt.Errorf("%w: thinking payload shape", prompty.ErrProviderState)
	}
	switch block.Type {
	case wireThinking:
		if len(fields) != 3 || !fields[wireThinking] || !fields[wireSignature] ||
			block.Thinking == nil || block.Signature == nil || *block.Signature == "" {
			return block, prompty.ErrProviderState
		}
	case wireRedactedThinking:
		if len(fields) != 2 || !fields["data"] || block.Data == nil || *block.Data == "" {
			return block, prompty.ErrProviderState
		}
	default:
		return block, prompty.ErrProviderState
	}
	return block, nil
}

// A struct decoder alone accepts case aliases, duplicate keys and null fields
// from the other variant. None may be silently dropped by SDK reconstruction.
func thinkingFields(payload []byte) (map[string]bool, error) {
	if !json.Valid(payload) {
		return nil, prompty.ErrProviderState
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	opening, err := decoder.Token()
	if err != nil || opening != json.Delim('{') {
		return nil, prompty.ErrProviderState
	}
	fields := make(map[string]bool)
	for decoder.More() {
		token, tokenErr := decoder.Token()
		if tokenErr != nil {
			return nil, prompty.ErrProviderState
		}
		key, ok := token.(string)
		if !ok || fields[key] {
			return nil, prompty.ErrProviderState
		}
		switch key {
		case wireType, wireThinking, wireSignature, "data":
		default:
			return nil, prompty.ErrProviderState
		}
		fields[key] = true
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return nil, prompty.ErrProviderState
		}
	}
	return fields, nil
}

func thinkingState(
	block anthropic.ContentBlockUnion,
	index int,
	destination prompty.ProfileIdentity,
) (prompty.PartState, prompty.ReasoningPart, error) {
	payload := []byte(block.RawJSON())
	if len(payload) == 0 {
		value := thinkingPayload{Thinking: nil, Signature: nil, Data: nil, Type: block.Type}
		if block.Type == wireThinking {
			value.Thinking, value.Signature = &block.Thinking, &block.Signature
		} else {
			value.Data = &block.Data
		}
		var err error
		payload, err = json.Marshal(value)
		if err != nil {
			return prompty.PartState{}, prompty.ReasoningPart{}, err
		}
	}
	decoded, err := decodeThinking(payload)
	if err != nil {
		return prompty.PartState{}, prompty.ReasoningPart{}, err
	}
	part := prompty.ReasoningPart{Text: "", CachePolicy: nil}
	if decoded.Thinking != nil {
		part.Text = *decoded.Thinking
	}
	state := prompty.PartState{PartIndex: index, Envelope: prompty.ProviderEnvelope{ExpiresAt: nil,
		Format: prompty.ProviderStateFormat, Scope: destination, Codec: thinkingCodec,
		Payload: bytes.Clone(payload), Required: true,
	}}
	return state, part, nil
}

// ValidateProviderState checks scope and exact signed text before any SDK request.
//
//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (a *Adapter) ValidateProviderState(exec *prompty.PromptExecution, destination prompty.ProfileIdentity) error {
	if err := prompty.ValidateContinuation(exec.Messages, destination, time.Now()); err != nil {
		return err
	}
	for _, message := range exec.Messages {
		if len(message.MessageAnnotations) > 0 {
			if _, err := a.hostedHistoryMessage(message, destination); err != nil {
				return err
			}
		}
		if len(message.ProviderState) > 0 && prompty.ValidateCachePolicy(message.CachePolicy) != nil {
			return prompty.ErrProviderState
		}
		states := make(map[int]prompty.PartState, len(message.ProviderState))
		for _, state := range message.ProviderState {
			if message.Role != prompty.RoleAssistant || state.Envelope.Codec != thinkingCodec ||
				!state.Envelope.Required {
				return prompty.ErrProviderState
			}
			payload, err := decodeThinking(state.Envelope.Payload)
			if err != nil {
				return err
			}
			part, ok := reasoningValue(message.Content[state.PartIndex])
			if !ok {
				return prompty.ErrProviderState
			}
			policy := part.CachePolicy
			if policy == nil {
				policy = message.CachePolicy
			}
			if !cacheAbsentOnThinking(policy) {
				return prompty.ErrProviderState
			}
			text := ""
			if payload.Thinking != nil {
				text = *payload.Thinking
			}
			if part.Text != text {
				return fmt.Errorf("%w: signed reasoning changed", prompty.ErrProviderState)
			}
			states[state.PartIndex] = state
		}
		for index, part := range message.Content {
			if _, ok := reasoningValue(part); ok {
				if _, exists := states[index]; !exists {
					return prompty.ErrStateUnavailable
				}
			}
		}
	}
	return nil
}

// Explicit omission changes no signed SDK field; real cache intent is forbidden.
func cacheAbsentOnThinking(policy *prompty.CachePolicy) bool {
	return policy == nil || (policy.Mechanism == prompty.CacheOmit && prompty.ValidateCachePolicy(policy) == nil)
}

func reasoningValue(part prompty.ContentPart) (prompty.ReasoningPart, bool) {
	switch value := part.(type) {
	case prompty.ReasoningPart:
		return value, true
	case *prompty.ReasoningPart:
		if value != nil {
			return *value, true
		}
	}
	return prompty.ReasoningPart{}, false
}

func (a *Adapter) assistantMessageWithState(message prompty.ChatMessage) (anthropic.MessageParam, error) {
	if len(message.MessageAnnotations) > 0 {
		manifest, err := decodeHostedHistory(message)
		if err != nil {
			return anthropic.MessageParam{}, err
		}
		return a.hostedHistoryMessage(message, manifest.Scope)
	}
	states := make(map[int]prompty.PartState, len(message.ProviderState))
	for _, state := range message.ProviderState {
		states[state.PartIndex] = state
	}
	var blocks []anthropic.ContentBlockParamUnion
	for index, part := range message.Content {
		if state, exists := states[index]; exists {
			payload, err := decodeThinking(state.Envelope.Payload)
			if err != nil {
				return anthropic.MessageParam{}, err
			}
			if payload.Type == wireThinking {
				blocks = append(blocks, anthropic.NewThinkingBlock(*payload.Signature, *payload.Thinking))
			} else {
				blocks = append(blocks, anthropic.NewRedactedThinkingBlock(*payload.Data))
			}
			continue
		}
		converted, err := a.assistantMessage([]prompty.ContentPart{part}, message.CachePolicy)
		if err != nil {
			return anthropic.MessageParam{}, err
		}
		blocks = append(blocks, converted.Content...)
	}
	return anthropic.NewAssistantMessage(blocks...), nil
}

func (a *Adapter) destinationIdentity(exec *prompty.PromptExecution) prompty.ProfileIdentity {
	desc := a.RuntimeDescriptor()
	identity := prompty.ProfileIdentity{
		ModelFamily: "",
		Provider:    desc.Name,
		Endpoint:    desc.Endpoint,
		Model:       desc.DefaultModel,
	}
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		identity.Model = exec.ModelOptions.Model
	}
	for _, profile := range desc.Profiles {
		if profile.Identity.Provider == identity.Provider && profile.Identity.Endpoint == identity.Endpoint &&
			profile.Identity.Model == identity.Model {
			identity.ModelFamily = profile.Identity.ModelFamily
			break
		}
	}
	if override := exec.Requirements.Override; override != nil && override.Identity.Provider == identity.Provider &&
		override.Identity.Endpoint == identity.Endpoint && override.Identity.Model == identity.Model {
		identity.ModelFamily = override.Identity.ModelFamily
	}
	return identity
}

func anthropicOutcome(reason anthropic.StopReason) prompty.Outcome {
	switch reason {
	case anthropic.StopReasonEndTurn, anthropic.StopReasonStopSequence:
		return prompty.OutcomeCompleted
	case anthropic.StopReasonToolUse:
		return prompty.OutcomeToolCalls
	case anthropic.StopReasonRefusal:
		return prompty.OutcomeRefusal
	case anthropic.StopReasonPauseTurn:
		return prompty.OutcomePaused
	case anthropic.StopReasonMaxTokens, anthropic.StopReasonModelContextWindowExceeded:
		fallthrough
	default:
		return prompty.OutcomeIncomplete
	}
}
