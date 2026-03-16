package prompty

import (
	"context"

	"iter"
)

// Invoker defines the minimal contract for an LLM client (sync + stream).
type Invoker interface {
	Generate(ctx context.Context, exec *PromptExecution) (*Response, error)
	GenerateStream(ctx context.Context, exec *PromptExecution) iter.Seq2[*ResponseChunk, error]
}

// LLMClient is the public client interface.
// At the code level it is equivalent to Invoker but separated by meaning.
type LLMClient = Invoker
