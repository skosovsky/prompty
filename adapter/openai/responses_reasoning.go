package openai

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const responsesReasoningCodec = "openai.responses.reasoning.json"

// Decode source fields rather than accepting SDK zero values for absent fields.
// Unknown provider fields stay in the original opaque bytes, never normalized.
func decodeResponseReasoning(source []byte) (responses.ResponseReasoningItem, string, error) {
	var item responses.ResponseReasoningItem
	var fields map[string]json.RawMessage
	if !uniqueResponseJSON(source) || json.Unmarshal(source, &fields) != nil ||
		json.Unmarshal(source, &item) != nil || item.Type != wireReasoning || strings.TrimSpace(item.ID) == "" {
		return item, "", prompty.ErrProviderState
	}
	var id, kind string
	if json.Unmarshal(fields["id"], &id) != nil || id != item.ID ||
		json.Unmarshal(fields[wireType], &kind) != nil || kind != wireReasoning {
		return item, "", prompty.ErrProviderState
	}
	var summaries []struct {
		Type string  `json:"type"`
		Text *string `json:"text"`
	}
	if summary := fields["summary"]; len(summary) == 0 || bytes.Equal(bytes.TrimSpace(summary), []byte("null")) ||
		json.Unmarshal(summary, &summaries) != nil {
		return item, "", prompty.ErrProviderState
	}
	texts := make([]string, 0, len(summaries))
	for _, summary := range summaries {
		if summary.Type != wireSummaryText || summary.Text == nil {
			return item, "", prompty.ErrProviderState
		}
		texts = append(texts, *summary.Text)
	}
	if status, present := fields["status"]; present {
		var value string
		if json.Unmarshal(status, &value) != nil ||
			(value != wireCompleted && value != wireInProgress && value != wireIncomplete) {
			return item, "", prompty.ErrProviderState
		}
	}
	if encrypted, present := fields["encrypted_content"]; present &&
		!bytes.Equal(bytes.TrimSpace(encrypted), []byte("null")) {
		var value string
		if json.Unmarshal(encrypted, &value) != nil {
			return item, "", prompty.ErrProviderState
		}
	}
	return item, strings.Join(texts, "\n\n"), nil
}

func preserveResponseReasoning(source []byte, response *prompty.Response, profile prompty.ProfileIdentity) error {
	item, summary, err := decodeResponseReasoning(source)
	if err != nil {
		return err
	}
	index := len(response.Content)
	response.Content = append(response.Content, prompty.ReasoningPart{CachePolicy: nil, Text: summary})
	if item.Status != "" && item.Status != wireCompleted &&
		(response.Outcome == prompty.OutcomeCompleted || response.Outcome == prompty.OutcomeToolCalls) {
		response.Outcome = prompty.OutcomeIncomplete
	}
	if item.EncryptedContent == "" {
		response.ContinuationUnavailable = true
		return nil
	}
	response.ProviderState = append(response.ProviderState, prompty.PartState{
		PartIndex: index,
		Envelope: prompty.ProviderEnvelope{ExpiresAt: nil,
			Format: prompty.ProviderStateFormat, Scope: profile, Codec: responsesReasoningCodec,
			Required: true, Payload: bytes.Clone(source),
		},
	})
	return nil
}

// Required message metadata proves incoming history support, not a request for
// new encrypted reasoning. Retain the existing explicit/part-state guarantee.
func responseRequiresOutputState(exec *prompty.PromptExecution) bool {
	if slices.Contains(exec.Requirements.Capabilities, prompty.CapabilityProviderState) {
		return true
	}
	for _, message := range exec.Messages {
		if len(message.ProviderState) > 0 {
			return true
		}
	}
	return false
}

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func responseMessageInput(
	message prompty.ChatMessage,
	profile prompty.ProfileIdentity,
) ([]responses.ResponseInputItemUnionParam, error) {
	if err := prompty.ValidateContinuation([]prompty.ChatMessage{message}, profile, time.Now()); err != nil {
		return nil, err
	}
	if len(message.MessageAnnotations) != 0 {
		return responseHostedHistoryInput(message, profile)
	}
	if len(message.ProviderState) == 0 {
		content, err := responseInputParts(message)
		if err != nil {
			return nil, err
		}
		return []responses.ResponseInputItemUnionParam{responseTextInput(message.Role, content)}, nil
	}
	if message.Role != prompty.RoleAssistant || message.CachePolicy != nil || len(message.Annotations) != 0 ||
		len(message.MessageAnnotations) != 0 || len(message.Metadata) != 0 {
		return nil, adapter.ErrUnsupportedCapability
	}
	states := make(map[int]prompty.PartState)
	for _, state := range message.ProviderState {
		if !state.Envelope.Required ||
			(state.Envelope.Codec != responsesReasoningCodec && state.Envelope.Codec != responsesFunctionCodec) {
			return nil, prompty.ErrProviderState
		}
		states[state.PartIndex] = state
	}
	var out []responses.ResponseInputItemUnionParam
	seen := make(map[string]bool)
	var textParts []prompty.ContentPart
	flush := func() error {
		if len(textParts) == 0 {
			return nil
		}
		content, err := responseInputParts(
			prompty.ChatMessage{
				ProviderState:           nil,
				Annotations:             nil,
				MessageAnnotations:      nil,
				ContinuationUnavailable: false,
				CachePolicy:             nil,
				Provenance:              nil,
				Metadata:                nil,
				LayerKind:               "",
				Role:                    message.Role,
				Content:                 textParts,
			},
		)
		if err != nil {
			return err
		}
		out = append(out, responseTextInput(message.Role, content))
		textParts = nil
		return nil
	}
	for index, part := range message.Content {
		state, present := states[index]
		if !present {
			textParts = append(textParts, part)
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
		if state.Envelope.Codec == responsesFunctionCodec {
			item, err := responseFunctionInput(state, part)
			if err != nil {
				return nil, err
			}
			out = append(out, item)
			continue
		}
		var canonical prompty.ReasoningPart
		switch value := part.(type) {
		case prompty.ReasoningPart:
			canonical = value
		case *prompty.ReasoningPart:
			if value == nil {
				return nil, prompty.ErrProviderState
			}
			canonical = *value
		default:
			return nil, prompty.ErrProviderState
		}
		item, summary, err := decodeResponseReasoning(state.Envelope.Payload)
		if err != nil || canonical.Text != summary || canonical.CachePolicy != nil || item.EncryptedContent == "" ||
			(item.Status != "" && item.Status != wireCompleted) || seen[item.ID] {
			return nil, prompty.ErrProviderState
		}
		seen[item.ID] = true
		params := item.ToParam()
		out = append(
			out,
			//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
			responses.ResponseInputItemUnionParam{OfReasoning: &params},
		)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

func responseTextInput(
	role prompty.Role,
	content responses.ResponseInputMessageContentListParam,
) responses.ResponseInputItemUnionParam {
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	return responses.ResponseInputItemUnionParam{
		OfMessage: &responses.EasyInputMessageParam{ //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
			Role: responses.EasyInputMessageRole(role),
			//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
			Content: responses.EasyInputMessageContentUnionParam{
				OfInputItemContentList: content,
			},
		},
	}
}
