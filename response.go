package prompty

import "strings"

// TextFromParts concatenates all text parts into a single string, ignoring non-text parts.
func TextFromParts(parts []ContentPart) string {
	var b strings.Builder
	for _, p := range parts {
		if t, ok := p.(TextPart); ok {
			b.WriteString(t.Text)
		}
	}
	return b.String()
}

// Usage contains token statistics for the model response.
type Usage struct {
	PromptTokens              int
	CompletionTokens          int
	TotalTokens               int
	PromptTokensCached        int
	PromptTokensCacheCreation int
	CompletionTokensReasoning int
}

// Response is the canonical full model response for sync calls.
type Response struct {
	Content      []ContentPart
	Usage        Usage
	FinishReason string // provider stop reason (e.g. "stop", "length") for telemetry
}

// NewResponse creates a Response from content parts. Usage remains zero.
func NewResponse(parts []ContentPart) *Response {
	if parts == nil {
		parts = []ContentPart{}
	}
	return &Response{Content: parts}
}

// Text concatenates all text parts of the response into a single string.
// Convenient for simple cases when multimodality is not needed.
func (r *Response) Text() string {
	if r == nil {
		return ""
	}
	return TextFromParts(r.Content)
}

// ResponseChunk is one chunk of the stream.
// In streaming providers Usage and FinishReason are typically populated only in the final chunk.
type ResponseChunk struct {
	Content      []ContentPart
	Usage        Usage
	IsFinished   bool
	FinishReason string // provider stop reason (e.g. "stop", "length") for telemetry
}
