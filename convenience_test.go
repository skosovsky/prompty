package prompty

import (
	"context"
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
