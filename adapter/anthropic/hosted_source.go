package anthropic

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/url"
	"slices"
	"strings"
	"time"
)

func sourceString(fields map[string]json.RawMessage, key string, nonblank bool) (string, bool) {
	var value *string
	if json.Unmarshal(fields[key], &value) != nil || value == nil || nonblank && strings.TrimSpace(*value) == "" {
		return "", false
	}
	return *value, true
}

func sourceEnum(fields map[string]json.RawMessage, key string, values ...string) bool {
	value, ok := sourceString(fields, key, false)
	return ok && slices.Contains(values, value)
}

func sourceOptionalString(fields map[string]json.RawMessage, key string, nullable bool) bool {
	document, present := fields[key]
	if !present {
		return true
	}
	if nullable && bytes.Equal(bytes.TrimSpace(document), []byte("null")) {
		return true
	}
	_, ok := sourceString(fields, key, false)
	return ok
}

func sourceURL(fields map[string]json.RawMessage, key string) bool {
	value, ok := sourceString(fields, key, true)
	parsed, err := url.Parse(value)
	return ok && err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") &&
		parsed.Hostname() != "" && parsed.User == nil
}

func sourceCaller(fields map[string]json.RawMessage) bool {
	document, present := fields["caller"]
	if !present {
		return true
	}
	caller, err := hostedObject(document, wireType)
	return err == nil && sourceEnum(caller, wireType, "direct")
}

func validateHostedSource(document json.RawMessage) (string, string, string, error) {
	fields, err := hostedObject(document, wireType, "id", "name", "input", "caller", "tool_use_id", "content")
	if err != nil {
		return "", "", "", err
	}
	kind, _ := sourceString(fields, wireType, true)
	if !sourceCaller(fields) {
		return "", "", "", ErrHostedTool
	}
	switch kind {
	case wireServerToolUse:
		fields, err = hostedObject(document, wireType, "id", "name", "input", "caller")
		id, validID := sourceString(fields, "id", true)
		name, _ := sourceString(fields, "name", true)
		// The provider validates model-generated arguments. Preserve erroneous
		// object inputs too: invalid_tool_input is a legitimate hosted result.
		var input map[string]json.RawMessage
		object := uniqueSyntheticJSON(fields["input"]) && json.Unmarshal(fields["input"], &input) == nil && input != nil
		if err != nil || !validID || !slices.Contains([]string{wireWebSearch, wireWebFetch}, name) || !object {
			return "", "", "", ErrHostedTool
		}
		return kind, id, name, nil
	case wireWebSearchToolResult, wireWebFetchToolResult:
		fields, err = hostedObject(document, wireType, "tool_use_id", "content", "caller")
		id, validID := sourceString(fields, "tool_use_id", true)
		if err != nil || !validID {
			return "", "", "", ErrHostedTool
		}
		name := wireWebSearch
		if kind == wireWebSearchToolResult {
			err = validateSearchContent(fields["content"])
		} else {
			name = wireWebFetch
			err = validateFetchContent(fields["content"])
		}
		return kind, id, name, err
	default:
		return "", "", "", ErrHostedTool
	}
}

func validateSearchContent(document json.RawMessage) error {
	if len(bytes.TrimSpace(document)) > 0 && bytes.TrimSpace(document)[0] == '[' {
		var results []json.RawMessage
		if json.Unmarshal(document, &results) != nil {
			return ErrHostedTool
		}
		for _, result := range results {
			fields, err := hostedObject(result, wireType, "url", "title", "encrypted_content", "page_age")
			_, title := sourceString(fields, "title", false)
			_, encrypted := sourceString(fields, "encrypted_content", true)
			if err != nil || !sourceEnum(fields, wireType, "web_search_result") || !sourceURL(fields, "url") ||
				!title || !encrypted || !sourceOptionalString(fields, "page_age", false) {
				return ErrHostedTool
			}
		}
		return nil
	}
	return validateHostedError(document, "web_search_tool_result_error", []string{
		"invalid_tool_input",
		"unavailable",
		"max_uses_exceeded",
		"too_many_requests",
		"query_too_long",
		"request_too_large",
	})
}

func validateHostedError(document json.RawMessage, kind string, codes []string) error {
	fields, err := hostedObject(document, wireType, "error_code")
	if err != nil || !sourceEnum(fields, wireType, kind) || !sourceEnum(fields, "error_code", codes...) {
		return ErrHostedTool
	}
	return nil
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func validateFetchContent(document json.RawMessage) error {
	fields, err := hostedObject(document, wireType, "url", "content", "retrieved_at", "error_code")
	if err != nil {
		return err
	}
	if sourceEnum(fields, wireType, "web_fetch_tool_result_error") {
		return validateHostedError(document, "web_fetch_tool_result_error", []string{
			"invalid_tool_input", "url_too_long", "url_not_allowed", "url_not_accessible", "unsupported_content_type",
			"too_many_requests", "max_uses_exceeded", "unavailable", "url_not_in_prior_context",
		})
	}
	fields, err = hostedObject(document, wireType, "url", "content", "retrieved_at")
	if err != nil || !sourceEnum(fields, wireType, "web_fetch_result") || !sourceURL(fields, "url") {
		return ErrHostedTool
	}
	if _, present := fields["retrieved_at"]; present {
		value, ok := sourceString(fields, "retrieved_at", false)
		if _, timestampErr := time.Parse(time.RFC3339, value); !ok || timestampErr != nil {
			return ErrHostedTool
		}
	}
	doc, err := hostedObject(fields["content"], wireType, "source", "title", "citations")
	if err != nil || !sourceEnum(doc, wireType, "document") || !sourceOptionalString(doc, "title", true) {
		return ErrHostedTool
	}
	if citation, present := doc["citations"]; present {
		values, citationErr := hostedObject(citation, "enabled")
		var enabled *bool
		if citationErr != nil || json.Unmarshal(values["enabled"], &enabled) != nil || enabled == nil {
			return ErrHostedTool
		}
	}
	source, err := hostedObject(doc["source"], wireType, "media_type", "data")
	data, validData := sourceString(source, "data", false)
	if err != nil || !validData {
		return ErrHostedTool
	}
	switch {
	case sourceEnum(source, wireType, wireText) && sourceEnum(source, "media_type", "text/plain"):
		return nil
	case sourceEnum(source, wireType, "base64") && sourceEnum(source, "media_type", "application/pdf"):
		_, err := base64.StdEncoding.Strict().DecodeString(data)
		if err == nil {
			return nil
		}
	}
	return ErrHostedTool
}
