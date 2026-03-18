package prompty

import (
	"context"
	"iter"
)

type scriptedInvoker struct {
	generate       func(context.Context, *PromptExecution) (*Response, error)
	generateStream func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error]
}

func (s *scriptedInvoker) Generate(ctx context.Context, exec *PromptExecution) (*Response, error) {
	if s.generate == nil {
		return nil, nil
	}
	return s.generate(ctx, exec)
}

func (s *scriptedInvoker) GenerateStream(ctx context.Context, exec *PromptExecution) iter.Seq2[*ResponseChunk, error] {
	if s.generateStream != nil {
		return s.generateStream(ctx, exec)
	}
	return func(yield func(*ResponseChunk, error) bool) {
		resp, err := s.Generate(ctx, exec)
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
