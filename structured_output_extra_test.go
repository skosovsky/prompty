package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type valueSchemaResult struct {
	Answer string `json:"answer"`
}

func (valueSchemaResult) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
	}
}

type pointerSchemaResult struct {
	Answer string `json:"answer"`
}

func (*pointerSchemaResult) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
	}
}

type semanticRetryResult struct {
	Answer string `json:"answer"`
}

func (semanticRetryResult) JSONSchema() map[string]any {
	return valueSchemaResult{}.JSONSchema()
}

func (r semanticRetryResult) Validate() error {
	if r.Answer == "" {
		return assert.AnError
	}
	return nil
}

func TestExecuteWithStructuredOutput_SemanticRetry(t *testing.T) {
	t.Parallel()

	callNum := 0
	var seen []*PromptExecution
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			seen = append(seen, clonePromptExecution(exec))
			if callNum == 1 {
				return NewResponse([]ContentPart{TextPart{Text: `{"answer":""}`}}), nil
			}
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"42"}`}}), nil
		},
	}

	exec := SimpleChat("Return JSON", "What is 6*7?")
	initialLen := len(exec.Messages)

	result, err := ExecuteWithStructuredOutput[semanticRetryResult](context.Background(), invoker, exec, 2)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "42", result.Answer)
	require.Len(t, seen, 2)
	require.Len(t, seen[1].Messages, initialLen+2)
	assert.Equal(t, RoleAssistant, seen[1].Messages[initialLen].Role)
	assert.Equal(
		t,
		"The JSON format is valid, but data violates business rules: assert.AnError general error for testing. Fix it.",
		seen[1].Messages[initialLen+1].Content[0].(TextPart).Text,
	)
}

func TestExecuteWithStructuredOutput_AutoSchemaValueReceiver(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			assert.Equal(t, "object", exec.ResponseFormat.Schema["type"])
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"ok"}`}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"), 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
}

func TestExecuteWithStructuredOutput_AutoSchemaPointerReceiver(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			assert.Equal(t, "object", exec.ResponseFormat.Schema["type"])
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"ok"}`}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[pointerSchemaResult](context.Background(), invoker, SimplePrompt("hi"), 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
}

func TestExecuteWithStructuredOutput_PreservesExplicitResponseFormat(t *testing.T) {
	t.Parallel()

	exec := SimplePrompt("hi")
	exec.ResponseFormat = &SchemaDefinition{
		Name: "explicit",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
		},
	}

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, got *PromptExecution) (*Response, error) {
			require.NotNil(t, got.ResponseFormat)
			assert.Equal(t, "explicit", got.ResponseFormat.Name)
			assert.Equal(t, "string", got.ResponseFormat.Schema["properties"].(map[string]any)["answer"].(map[string]any)["type"])
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"ok"}`}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, exec, 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
}

func TestExecuteWithStructuredOutput_AutoSchemaDoesNotBlockToolsInCore(t *testing.T) {
	t.Parallel()

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			require.Len(t, exec.Tools, 1)
			require.NotNil(t, exec.ResponseFormat)
			assert.Equal(t, "lookup", exec.Tools[0].Name)
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"ok"}`}}), nil
		},
	}

	exec := SimplePrompt("hi")
	exec.Tools = []ToolDefinition{{Name: "lookup"}}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, exec, 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
	assert.Equal(t, 1, callNum)
	assert.Nil(t, exec.ResponseFormat)
	require.Len(t, exec.Tools, 1)
}

func TestExecuteWithStructuredOutput_DoesNotMutateOriginalExecution(t *testing.T) {
	t.Parallel()

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			if callNum == 1 {
				require.NotNil(t, exec.ResponseFormat)
				return NewResponse([]ContentPart{TextPart{Text: `{invalid`}}), nil
			}
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"ok"}`}}), nil
		},
	}

	exec := SimplePrompt("hi")
	origLen := len(exec.Messages)

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, exec, 1)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Len(t, exec.Messages, origLen)
	assert.Nil(t, exec.ResponseFormat)
}

func TestExecuteWithStructuredOutput_StripsJSONFenceWithPrefixAndSuffix(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			return NewResponse([]ContentPart{TextPart{Text: "Here is the requested schema:\n```json\n{\"answer\":\"ok\"}\n```\nThanks."}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"), 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
}

func TestExecuteWithStructuredOutput_StripsGenericFence(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			return NewResponse([]ContentPart{TextPart{Text: "prefix\n```\n{\"answer\":\"ok\"}\n```\nsuffix"}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"), 0)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
}
