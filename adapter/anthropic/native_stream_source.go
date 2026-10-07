package anthropic

import (
	"bytes"
	"encoding/json"
	"slices"

	"github.com/skosovsky/prompty"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty/adapter"
)

// Validate consumed source fields before SDK conveniences can coerce a value.
// Optional provider fields remain allowed; unknown semantic variants are rejected
// by the stream state machine rather than an accidental source-field whitelist.
func validateNativeEventSource(event anthropic.MessageStreamEventUnion) error {
	raw := []byte(event.RawJSON())
	var fields map[string]json.RawMessage
	if !uniqueSyntheticJSON(raw) || json.Unmarshal(raw, &fields) != nil || fields == nil {
		return adapter.ErrInvalidResponse
	}
	kind, ok := sourceString(fields, "type", true)
	if !ok || kind != event.Type {
		return adapter.ErrInvalidResponse
	}
	switch kind {
	case "content_block_start", "content_block_delta", "content_block_stop":
		index, validIndex := sourceIndex(fields, "index")
		if !validIndex || index != event.Index {
			return adapter.ErrInvalidResponse
		}
	}
	switch kind {
	case "message_start":
		return validateNativeMessageStart(fields)
	case "message_delta":
		return validateNativeMessageDelta(fields)
	case "content_block_start":
		return validateNativeBlockStart(fields)
	}
	if kind != "content_block_delta" {
		return nil
	}
	var delta map[string]json.RawMessage
	if json.Unmarshal(fields["delta"], &delta) != nil || delta == nil {
		return adapter.ErrInvalidResponse
	}
	deltaType, ok := sourceString(delta, "type", true)
	if !ok || deltaType != event.Delta.Type {
		return adapter.ErrInvalidResponse
	}
	var key string
	switch deltaType {
	case "text_delta":
		key = "text"
	case "input_json_delta":
		key = "partial_json"
	case "thinking_delta":
		key = "thinking"
	case "signature_delta":
		key = "signature"
	case "citations_delta":
		return validateHostedCitation(delta["citation"])
	default:
		return nil // Existing state machine rejects unsupported variants/context.
	}
	if _, ok := sourceString(delta, key, false); !ok {
		return adapter.ErrInvalidResponse
	}
	return nil
}

func nativeSourceObject(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || fields == nil {
		return nil, adapter.ErrInvalidResponse
	}
	return fields, nil
}

func validateNativeMessageStart(fields map[string]json.RawMessage) error {
	message, err := nativeSourceObject(fields["message"])
	if err != nil {
		return err
	}
	for _, key := range []string{"id", "model", "type", "role"} {
		if _, ok := sourceString(message, key, true); !ok {
			return adapter.ErrInvalidResponse
		}
	}
	var content []json.RawMessage
	if json.Unmarshal(message["content"], &content) != nil || content == nil {
		return adapter.ErrInvalidResponse
	}
	return validateNativeUsageSource(message["usage"])
}

func validateNativeMessageDelta(fields map[string]json.RawMessage) error {
	delta, err := nativeSourceObject(fields["delta"])
	if err != nil {
		return err
	}
	if raw, present := delta["stop_reason"]; present && !bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		reason, ok := sourceString(delta, "stop_reason", true)
		if !ok ||
			!slices.Contains(
				[]string{"end_turn", "max_tokens", "stop_sequence", "tool_use", "pause_turn", "refusal"},
				reason,
			) {
			return adapter.ErrInvalidResponse
		}
	}
	return validateNativeUsageSource(fields["usage"])
}

func validateNativeUsageSource(raw json.RawMessage) error {
	usage, err := nativeSourceObject(raw)
	if err != nil {
		return prompty.ErrUsage
	}
	for _, key := range []string{"input_tokens", "output_tokens", "cache_read_input_tokens", "cache_creation_input_tokens"} {
		if _, present := usage[key]; present {
			if _, ok := sourceIndex(usage, key); !ok {
				return prompty.ErrUsage
			}
		}
	}
	return nil
}

func validateNativeBlockStart(fields map[string]json.RawMessage) error {
	block, err := nativeSourceObject(fields["content_block"])
	if err != nil {
		return err
	}
	kind, ok := sourceString(block, "type", true)
	if !ok {
		return adapter.ErrInvalidResponse
	}
	switch kind {
	case "tool_use":
		for _, key := range []string{"id", "name"} {
			if _, ok := sourceString(block, key, true); !ok {
				return adapter.ErrInvalidResponse
			}
		}
	case "thinking":
		if _, ok := sourceString(block, "thinking", false); !ok {
			return adapter.ErrInvalidResponse
		}
	case "redacted_thinking":
		if _, ok := sourceString(block, "data", true); !ok {
			return adapter.ErrInvalidResponse
		}
	}
	return nil
}
