package prompty

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"strings"
	"text/template"
	"unicode/utf8"
)

// defaultFuncMap returns the template.FuncMap used for ChatPromptTemplate rendering.
func defaultFuncMap(tc TokenCounter) template.FuncMap {
	if tc == nil {
		tc = &CharFallbackCounter{}
	}
	return template.FuncMap{
		"truncate_chars":       truncateChars,
		"truncate_tokens":      makeTruncateTokens(tc),
		"render_tools_as_xml":  renderToolsAsXML,
		"render_tools_as_json": renderToolsAsJSON,
	}
}

// truncateChars truncates text to at most maxChars runes.
// Uses RuneCountInString for early exit to avoid allocating []rune when no truncation is needed.
func truncateChars(text string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if utf8.RuneCountInString(text) <= maxChars {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxChars])
}

// makeTruncateTokens returns a function that truncates text to at most maxTokens using the given TokenCounter.
func makeTruncateTokens(tc TokenCounter) func(string, int) (string, error) {
	return func(text string, maxTokens int) (string, error) {
		if maxTokens <= 0 {
			return "", nil
		}
		n, err := tc.Count(text)
		if err != nil {
			return "", err
		}
		if n <= maxTokens {
			return text, nil
		}
		runes := []rune(text)
		lo, hi := 0, len(runes)
		for lo < hi {
			mid := (lo + hi + 1) / 2
			n, _ = tc.Count(string(runes[:mid]))
			if n <= maxTokens {
				lo = mid
			} else {
				hi = mid - 1
			}
		}
		return string(runes[:lo]), nil
	}
}

// xmlTool is used for deterministic XML marshalling of tools.
type xmlTool struct {
	XMLName     xml.Name `xml:"tool"`
	Name        string   `xml:"name"`
	Description string   `xml:"description"`
	Parameters  string   `xml:"parameters"` // JSON string
}

// renderToolsAsXML returns a deterministic XML representation of tools (one <tool> per definition).
func renderToolsAsXML(tools any) (string, error) {
	list, ok := asToolSlice(tools)
	if !ok {
		return "", fmt.Errorf("render_tools_as_xml: expected []ToolDefinition, got %T", tools)
	}
	var sb strings.Builder
	sb.WriteString("<tools>\n")
	for _, t := range list {
		params := ""
		if len(t.Parameters) > 0 {
			b, _ := json.Marshal(t.Parameters)
			params = string(b)
		}
		tx := xmlTool{Name: t.Name, Description: t.Description, Parameters: params}
		out, err := xml.MarshalIndent(tx, "  ", "  ")
		if err != nil {
			return "", err
		}
		sb.Write(out)
		sb.WriteString("\n")
	}
	sb.WriteString("</tools>")
	return sb.String(), nil
}

// renderToolsAsJSON returns a deterministic JSON representation of tools.
// Nil input returns "[]" (empty array), consistent with renderToolsAsXML.
func renderToolsAsJSON(tools any) (string, error) {
	list, ok := asToolSlice(tools)
	if !ok {
		return "", fmt.Errorf("render_tools_as_json: expected []ToolDefinition, got %T", tools)
	}
	if list == nil {
		return "[]", nil
	}
	b, err := json.Marshal(list)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

func asToolSlice(tools any) ([]ToolDefinition, bool) {
	if tools == nil {
		return nil, true
	}
	switch v := tools.(type) {
	case []ToolDefinition:
		return v, true
	case *[]ToolDefinition:
		if v == nil {
			return nil, true
		}
		return *v, true
	default:
		return nil, false
	}
}
