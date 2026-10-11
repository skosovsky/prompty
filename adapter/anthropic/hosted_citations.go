package anthropic

import (
	"bytes"
	"encoding/json"
)

func validateHostedText(document json.RawMessage) error {
	fields, err := hostedObject(document, wireType, wireText, "citations")
	_, text := sourceString(fields, wireText, false)
	if err != nil || !sourceEnum(fields, wireType, wireText) || !text {
		return ErrHostedTool
	}
	if citations, present := fields["citations"]; present {
		var values []json.RawMessage
		if json.Unmarshal(citations, &values) != nil || bytes.Equal(bytes.TrimSpace(citations), []byte("null")) {
			return ErrHostedTool
		}
		for _, citation := range values {
			if err := validateHostedCitation(citation); err != nil {
				return err
			}
		}
	}
	return nil
}

func sourceIndex(fields map[string]json.RawMessage, key string) (int64, bool) {
	var value *int64
	if json.Unmarshal(fields[key], &value) != nil || value == nil || *value < 0 {
		return 0, false
	}
	return *value, true
}

func validateHostedCitation(document json.RawMessage) error {
	var initial map[string]json.RawMessage
	if json.Unmarshal(document, &initial) != nil {
		return ErrHostedTool
	}
	kind, _ := sourceString(initial, wireType, true)
	allowed := []string{wireType, "cited_text"}
	index, title, start, end := "", "", "", ""
	switch kind {
	case "web_search_result_location":
		allowed = append(allowed, "url", "title", "encrypted_index")
	case "char_location":
		index, title, start, end = wireDocumentIndex, wireDocumentTitle, "start_char_index", "end_char_index"
		allowed = append(allowed, "file_id")
	case "page_location":
		index, title, start, end = wireDocumentIndex, wireDocumentTitle, "start_page_number", "end_page_number"
		allowed = append(allowed, "file_id")
	case "content_block_location":
		index, title, start, end = wireDocumentIndex, wireDocumentTitle, "start_block_index", "end_block_index"
		allowed = append(allowed, "file_id")
	case "search_result_location":
		index, title, start, end = "search_result_index", "title", "start_block_index", "end_block_index"
		allowed = append(allowed, "source")
	default:
		return ErrHostedTool
	}
	if index != "" {
		allowed = append(allowed, index, title, start, end)
	}
	fields, err := hostedObject(document, allowed...)
	_, cited := sourceString(fields, "cited_text", false)
	if err != nil || !cited {
		return ErrHostedTool
	}
	if kind == "web_search_result_location" {
		_, encrypted := sourceString(fields, "encrypted_index", true)
		_, hasTitle := fields["title"]
		if !sourceURL(fields, "url") || !encrypted || !hasTitle || !sourceOptionalString(fields, "title", true) {
			return ErrHostedTool
		}
		return nil
	}
	_, validIndex := sourceIndex(fields, index)
	first, validStart := sourceIndex(fields, start)
	last, validEnd := sourceIndex(fields, end)
	_, hasTitle := fields[title]
	if !validIndex || !validStart || !validEnd || !hasTitle || !sourceOptionalString(fields, title, true) ||
		!sourceOptionalString(fields, "file_id", true) {
		return ErrHostedTool
	}
	if kind == "page_location" {
		if first < 1 || last < first {
			return ErrHostedTool
		}
	} else if last <= first {
		return ErrHostedTool
	}
	if kind == "search_result_location" {
		if _, valid := sourceString(fields, "source", true); !valid {
			return ErrHostedTool
		}
	}
	return nil
}
