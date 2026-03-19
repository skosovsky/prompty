package prompty

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateText(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.Len(t, exec.Messages, 1)
			assert.Equal(t, RoleUser, exec.Messages[0].Role)
			assert.Equal(t, "hello", exec.Messages[0].Content[0].(TextPart).Text)
			assert.Equal(t, "test-id", exec.Metadata.ID)
			return NewResponse([]ContentPart{TextPart{Text: "world"}}), nil
		},
	}

	got, err := GenerateText(context.Background(), invoker, "hello", func(exec *PromptExecution) {
		exec.Metadata.ID = "test-id"
	})
	require.NoError(t, err)
	assert.Equal(t, "world", got)
}

func TestGenerateText_NilInvoker(t *testing.T) {
	t.Parallel()

	_, err := GenerateText(context.Background(), nil, "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invoker is nil")
}

func TestGenerateText_NilResponse(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) {
			return nil, nil
		},
	}

	_, err := GenerateText(context.Background(), invoker, "hello")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil response")
}

func TestGenerateStructured_WithRetries(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	callNum := 0
	var seen []*PromptExecution
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			seen = append(seen, clonePromptExecution(exec))
			if callNum == 1 {
				return NewResponse([]ContentPart{TextPart{Text: `{invalid`}}), nil
			}
			return NewResponse([]ContentPart{TextPart{Text: `{"name":"Alice"}`}}), nil
		},
	}

	got, err := GenerateStructured[result](context.Background(), invoker, "extract name", WithRetries(1))
	require.NoError(t, err)
	assert.Equal(t, "Alice", got.Name)
	assert.Equal(t, 2, callNum)
	require.Len(t, seen, 2)
	require.Len(t, seen[1].Messages, 3)
	assert.Equal(t, RoleAssistant, seen[1].Messages[1].Role)
	assert.Equal(t, RoleUser, seen[1].Messages[2].Role)
}

func TestGenerateStructured_PointerResult(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			return NewResponse([]ContentPart{TextPart{Text: `{"name":"Bob"}`}}), nil
		},
	}

	got, err := GenerateStructured[*result](context.Background(), invoker, "extract name")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "Bob", got.Name)
}

func TestGenerateStructured_DefaultRetriesAreDisabled(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			callNum++
			return NewResponse([]ContentPart{TextPart{Text: `{invalid`}}), nil
		},
	}

	got, err := GenerateStructured[result](context.Background(), invoker, "extract name")
	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	assert.Zero(t, got)
	assert.Equal(t, 1, callNum)
}

func TestGenerateStructured_NilInvoker(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	got, err := GenerateStructured[result](context.Background(), nil, "extract name", WithRetries(1))
	require.Error(t, err)
	assert.Zero(t, got)
	assert.Contains(t, err.Error(), "invoker is nil")
}

func TestGenerateStructured_PropagatesNonRetryableError(t *testing.T) {
	t.Parallel()

	type result struct {
		Name string `json:"name"`
	}

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			return nil, errors.New("boom")
		},
	}

	got, err := GenerateStructured[result](context.Background(), invoker, "extract name", WithRetries(2))
	require.Error(t, err)
	assert.Zero(t, got)
	assert.Contains(t, err.Error(), "boom")
}
