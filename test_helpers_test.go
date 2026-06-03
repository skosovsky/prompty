package prompty

import (
	"context"
	"encoding/json"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

type scriptedInvoker struct {
	generate       func(context.Context, *PromptExecution) (*Response, error)
	generateStream func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error]
}

func (s *scriptedInvoker) Execute(ctx context.Context, exec *PromptExecution) (*Response, error) {
	if s.generate == nil {
		return nil, nil
	}
	return s.generate(ctx, exec)
}

func (s *scriptedInvoker) ExecuteStream(ctx context.Context, exec *PromptExecution) iter.Seq2[*ResponseChunk, error] {
	if s.generateStream != nil {
		return s.generateStream(ctx, exec)
	}
	return func(yield func(*ResponseChunk, error) bool) {
		resp, err := s.Execute(ctx, exec)
		if err != nil {
			yield(nil, err)
			return
		}
		if resp == nil {
			yield(nil, nil)
			return
		}
		yield(&ResponseChunk{Content: cloneContentParts(resp.Content), Usage: resp.Usage, IsFinished: true}, nil)
	}
}

type toolValidatorFunc func(name string, argsJSON string) error

func (f toolValidatorFunc) ValidateToolCall(name string, argsJSON string) error {
	return f(name, argsJSON)
}

// stubToolInvoker supports tests that need validate/invoke without a full TypedToolRegistry.
type stubToolInvoker struct {
	validate func(name, argsJSON string) error
	invoke   func(name, argsJSON string) (json.RawMessage, error)
}

func (s stubToolInvoker) ValidateToolCall(name, argsJSON string) error {
	if s.validate != nil {
		return s.validate(name, argsJSON)
	}
	return nil
}

func (s stubToolInvoker) InvokeTool(name, argsJSON string) (json.RawMessage, error) {
	if s.invoke != nil {
		return s.invoke(name, argsJSON)
	}
	return json.RawMessage(`""`), nil
}

func mustTextFromParts(t *testing.T, parts []ContentPart) string {
	t.Helper()
	text, err := StrictTextFromParts(parts)
	require.NoError(t, err)
	return text
}

func collectSeq[T any](seq iter.Seq2[T, error]) ([]T, error) {
	var out []T
	for item, err := range seq {
		if err != nil {
			return out, err
		}
		out = append(out, item)
	}
	return out, nil
}
