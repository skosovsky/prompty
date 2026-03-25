package prompty

import (
	"context"

	"iter"
)

// Invoker defines the minimal contract for a model invoker (sync + stream).
type Invoker interface {
	Generate(ctx context.Context, exec *PromptExecution) (*Response, error)
	GenerateStream(ctx context.Context, exec *PromptExecution) iter.Seq2[*ResponseChunk, error]
}
