package prompty

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
	return &Response{
		Usage: Usage{
			PromptTokens:              0,
			CompletionTokens:          0,
			TotalTokens:               0,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 0,
			CompletionTokensReasoning: 0,
		},
		FinishReason: "",
		Content:      parts,
	}
}

// ResponseChunk is one chunk of the stream.
// In streaming providers Usage and FinishReason are typically populated only in the final chunk.
type ResponseChunk struct {
	Content      []ContentPart
	Usage        Usage
	IsFinished   bool
	FinishReason string // provider stop reason (e.g. "stop", "length") for telemetry
}
