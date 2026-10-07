package openai

import (
	"encoding/json"
	"maps"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func responseFunctionInput(
	state prompty.PartState,
	part prompty.ContentPart,
) (responses.ResponseInputItemUnionParam, error) {
	var zero responses.ResponseInputItemUnionParam
	var canonical prompty.ToolCallPart
	switch value := part.(type) {
	case prompty.ToolCallPart:
		canonical = value
	case *prompty.ToolCallPart:
		if value == nil {
			return zero, prompty.ErrProviderState
		}
		canonical = *value
	default:
		return zero, prompty.ErrProviderState
	}
	var item responses.ResponseFunctionToolCall
	var fields map[string]json.RawMessage
	source := state.Envelope.Payload
	if !uniqueResponseJSON(source) || json.Unmarshal(source, &fields) != nil || json.Unmarshal(source, &item) != nil ||
		!item.JSON.CallID.Valid() || !item.JSON.Name.Valid() || !item.JSON.Arguments.Valid() || item.Type != "function_call" ||
		strings.TrimSpace(
			item.CallID,
		) == "" || strings.TrimSpace(item.Name) == "" || !uniqueResponseJSON([]byte(item.Arguments)) ||
		canonical.ID != item.CallID || canonical.Name != item.Name || canonical.Args != item.Arguments ||
		canonical.ArgsChunk != "" || canonical.CachePolicy != nil {
		return zero, prompty.ErrProviderState
	}
	if status, present := fields["status"]; present {
		var value string
		if json.Unmarshal(status, &value) != nil || value != "completed" {
			return zero, prompty.ErrProviderState
		}
	}
	if id, present := fields["id"]; present {
		var value string
		if json.Unmarshal(id, &value) != nil || strings.TrimSpace(value) == "" {
			return zero, prompty.ErrProviderState
		}
	}
	params := item.ToParam()
	return responses.ResponseInputItemUnionParam{OfFunctionCall: &params}, nil
}

type responseToolHistory struct {
	calls   map[string]string
	results map[string]bool
	items   map[string]bool
}

func newResponseToolHistory(previous *prompty.RemoteResponseHandle) *responseToolHistory {
	history := &responseToolHistory{
		calls:   make(map[string]string),
		results: make(map[string]bool),
		items:   make(map[string]bool),
	}
	if previous != nil {
		history.calls = maps.Clone(previous.ToolCalls)
		if history.calls == nil {
			history.calls = make(map[string]string)
		}
	}
	return history
}

func (h *responseToolHistory) register(message prompty.ChatMessage) error {
	if len(message.MessageAnnotations) != 0 {
		manifest, err := decodeHostedHistory(message)
		if err != nil {
			return err
		}
		for _, entry := range manifest.Items {
			if len(entry.Source) == 0 {
				continue
			}
			id := responseHistoryItemID(entry.Source)
			if id == "" || h.items[id] {
				return prompty.ErrProviderState
			}
			h.items[id] = true
		}
	}
	for _, state := range message.ProviderState {
		var fields struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(state.Envelope.Payload, &fields) != nil {
			return prompty.ErrProviderState
		}
		if fields.ID != "" {
			if h.items[fields.ID] {
				return prompty.ErrProviderState
			}
			h.items[fields.ID] = true
		}
	}
	for _, part := range message.Content {
		var call prompty.ToolCallPart
		switch value := part.(type) {
		case prompty.ToolCallPart:
			call = value
		case *prompty.ToolCallPart:
			if value == nil {
				return prompty.ErrProviderState
			}
			call = *value
		default:
			continue
		}
		if h.calls[call.ID] != "" {
			return prompty.ErrRemoteResponse
		}
		h.calls[call.ID] = call.Name
	}
	return nil
}

func (h *responseToolHistory) resultInput(
	message prompty.ChatMessage,
) ([]responses.ResponseInputItemUnionParam, error) {
	if !responseCacheOmitted(message.CachePolicy) {
		return nil, adapter.ErrUnsupportedCapability
	}
	if message.Role != prompty.RoleTool || len(message.ProviderState) != 0 || message.ContinuationUnavailable ||
		len(message.Annotations) != 0 || len(message.MessageAnnotations) != 0 ||
		len(message.Metadata) != 0 || len(message.Content) == 0 {
		return nil, adapter.ErrUnsupportedCapability
	}
	var out []responses.ResponseInputItemUnionParam
	for _, part := range message.Content {
		var result prompty.ToolResultPart
		switch value := part.(type) {
		case prompty.ToolResultPart:
			result = value
		case *prompty.ToolResultPart:
			if value == nil {
				return nil, prompty.ErrRemoteResponse
			}
			result = *value
		default:
			return nil, adapter.ErrUnsupportedCapability
		}
		name := h.calls[result.ToolCallID]
		if name == "" || h.results[result.ToolCallID] || (result.Name != "" && result.Name != name) {
			return nil, prompty.ErrRemoteResponseScope
		}
		if result.IsError || !responseCacheOmitted(result.CachePolicy) {
			return nil, adapter.ErrUnsupportedCapability
		}
		native, err := responseFunctionResultContent(result.Content)
		if err != nil {
			return nil, err
		}
		output := responses.ResponseInputItemParamOfFunctionCallOutput(native)
		output.OfFunctionCallOutput.CallID = param.NewOpt(result.ToolCallID)
		out = append(out, output)
		h.results[result.ToolCallID] = true
	}
	return out, nil
}
