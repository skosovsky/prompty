package prompty

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithStructuredOutput_InvalidJSONReturnsValidationError(t *testing.T) {
	t.Parallel()

	type result struct {
		Answer string `json:"answer"`
	}

	callNum := 0
	var seen []*PromptExecution
	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			callNum++
			seen = append(seen, clonePromptExecution(exec))
			return NewResponse([]ContentPart{TextPart{Text: `{invalid`}}), nil
		},
	}

	exec := SimpleChat("Return JSON", "What is 6*7?")
	initialLen := len(exec.Messages)

	resultPtr, err := ExecuteWithStructuredOutput[result](context.Background(), invoker, exec)
	require.Nil(t, resultPtr)
	require.Error(t, err)
	assert.Equal(t, 1, callNum)
	assert.Len(t, seen, 1)
	assert.Len(t, exec.Messages, initialLen)
	assert.Nil(t, exec.ResponseFormat)
	require.NotNil(t, seen[0].ResponseFormat)

	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	require.NotNil(t, valErr.RawAssistantMessage)
	assert.Equal(t, RoleAssistant, valErr.RawAssistantMessage.Role)
	assert.Equal(t, `{invalid`, TextFromParts(valErr.RawAssistantMessage.Content))
	assert.Contains(t, valErr.FeedbackPrompt, "JSON validation failed:")
	assert.Contains(t, valErr.FeedbackPrompt, "Please fix your output.")
}

func TestExecuteWithStructuredOutput_SemanticValidationReturnsValidationError(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":""}`}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[semanticFeedbackResult](
		context.Background(),
		invoker,
		SimplePrompt("hi"),
	)
	require.Nil(t, result)
	require.Error(t, err)

	var valErr *ValidationError
	require.ErrorAs(t, err, &valErr)
	require.NotNil(t, valErr.RawAssistantMessage)
	assert.JSONEq(t, `{"answer":""}`, TextFromParts(valErr.RawAssistantMessage.Content))
	assert.Equal(
		t,
		"The JSON format is valid, but data violates business rules: assert.AnError general error for testing. Fix it.",
		valErr.FeedbackPrompt,
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

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"))
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

	result, err := ExecuteWithStructuredOutput[pointerSchemaResult](context.Background(), invoker, SimplePrompt("hi"))
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
			assert.Equal(
				t,
				"string",
				got.ResponseFormat.Schema["properties"].(map[string]any)["answer"].(map[string]any)["type"],
			)
			return NewResponse([]ContentPart{TextPart{Text: `{"answer":"ok"}`}}), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, exec)
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

	type result struct {
		Answer string `json:"answer"`
	}

	exec := SimplePrompt("hi")
	exec.Tools = []ToolDefinition{{Name: "lookup"}}

	got, err := ExecuteWithStructuredOutput[result](context.Background(), invoker, exec)
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "ok", got.Answer)
	assert.Equal(t, 1, callNum)
	assert.Nil(t, exec.ResponseFormat)
	require.Len(t, exec.Tools, 1)
}

func TestExecuteWithStructuredOutput_StripsJSONFenceWithPrefixAndSuffix(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			return NewResponse(
				[]ContentPart{
					TextPart{Text: "Here is the requested schema:\n```json\n{\"answer\":\"ok\"}\n```\nThanks."},
				},
			), nil
		},
	}

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"))
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

	result, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("hi"))
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "ok", result.Answer)
}

func TestExecuteWithStructuredOutput_FenceParserIgnoresBackticksInsideJSONString(t *testing.T) {
	t.Parallel()

	invoker := &scriptedInvoker{
		generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
			require.NotNil(t, exec.ResponseFormat)
			return NewResponse(
				[]ContentPart{
					TextPart{
						Text: "prefix\n```json\n{\"answer\":\"ok\",\"code\":\"```go\\nfmt.Println()\\n```\"}\n```\nsuffix\n```note\nignored\n```",
					},
				},
			), nil
		},
	}

	type result struct {
		Answer string `json:"answer"`
		Code   string `json:"code"`
	}

	got, err := ExecuteWithStructuredOutput[result](context.Background(), invoker, SimplePrompt("hi"))
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "ok", got.Answer)
	assert.Equal(t, "```go\nfmt.Println()\n```", got.Code)
}

func TestExecuteWithStructuredOutput_UnsupportedTypeFailsPreflight(t *testing.T) {
	t.Parallel()

	type unsupported struct {
		Values map[string]string `json:"values"`
	}

	callNum := 0
	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) {
			callNum++
			return nil, errors.New("should not be called")
		},
	}

	result, err := ExecuteWithStructuredOutput[unsupported](context.Background(), invoker, SimplePrompt("hi"))
	require.Nil(t, result)
	require.Error(t, err)
	assert.Equal(t, 0, callNum)
	assert.Contains(t, err.Error(), "unsupported type")
}

type valueSchemaResult struct {
	Answer string `json:"answer"`
}

func (valueSchemaResult) JSONSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"answer": map[string]any{"type": "string"},
		},
		"required":             []string{"answer"},
		"additionalProperties": false,
	}
}

type pointerSchemaResult struct {
	Answer string `json:"answer"`
}

func (*pointerSchemaResult) JSONSchema() map[string]any {
	return valueSchemaResult{}.JSONSchema()
}

type semanticFeedbackResult struct {
	Answer string `json:"answer"`
}

func (r semanticFeedbackResult) Validate() error {
	if r.Answer != "" {
		return nil
	}
	return assert.AnError
}

func (r semanticFeedbackResult) JSONSchema() map[string]any {
	return valueSchemaResult{}.JSONSchema()
}
