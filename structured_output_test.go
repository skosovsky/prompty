package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecuteWithStructuredOutput_ValidationRetry(t *testing.T) {
	t.Parallel()
	type result struct {
		Answer string `json:"answer"`
	}

	callNum := 0
	mockClient := &mockLLMClient{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			callNum++
			if callNum == 1 {
				return &Response{Content: []ContentPart{TextPart{Text: `{invalid`}}}, nil
			}
			return &Response{Content: []ContentPart{TextPart{Text: `{"answer":"42"}`}}}, nil
		},
	}

	exec := SimpleChat("You respond with JSON", "What is 6*7?")
	initialLen := len(exec.Messages)

	resultPtr, err := ExecuteWithStructuredOutput[result](context.Background(), mockClient, exec, 3)
	require.NoError(t, err)
	require.NotNil(t, resultPtr)
	assert.Equal(t, "42", resultPtr.Answer)
	assert.Equal(t, 2, callNum)

	// After retry, exec was passed with appended messages. Use a fresh mock that fails once
	// so the spy captures exec with +2 validation retry messages.
	callNum2 := 0
	mock2 := &mockLLMClient{
		generate: func(_ context.Context, _ *PromptExecution) (*Response, error) {
			callNum2++
			if callNum2 == 1 {
				return &Response{Content: []ContentPart{TextPart{Text: `{invalid`}}}, nil
			}
			return &Response{Content: []ContentPart{TextPart{Text: `{"answer":"4"}`}}}, nil
		},
	}
	spy := &spyLLMClient{LLMClient: mock2}
	exec2 := SimpleChat("You respond with JSON", "What is 2+2?")
	_, _ = ExecuteWithStructuredOutput[result](context.Background(), spy, exec2, 3)
	require.Len(t, spy.lastExec.Messages, initialLen+2)
	assert.Equal(t, RoleAssistant, spy.lastExec.Messages[initialLen].Role)
	assert.Equal(t, RoleUser, spy.lastExec.Messages[initialLen+1].Role)
	assert.Equal(t, `{invalid`, spy.lastExec.Messages[initialLen].Content[0].(TextPart).Text)
	assert.Contains(t, spy.lastExec.Messages[initialLen+1].Content[0].(TextPart).Text, "JSON validation failed:")
	assert.Contains(t, spy.lastExec.Messages[initialLen+1].Content[0].(TextPart).Text, "Please fix your output.")
}

type mockLLMClient struct {
	generate func(context.Context, *PromptExecution) (*Response, error)
}

func (m *mockLLMClient) Generate(ctx context.Context, exec *PromptExecution) (*Response, error) {
	return m.generate(ctx, exec)
}

func (m *mockLLMClient) GenerateStream(ctx context.Context, exec *PromptExecution) iter.Seq2[*ResponseChunk, error] {
	resp, err := m.Generate(ctx, exec)
	if err != nil {
		return func(yield func(*ResponseChunk, error) bool) { yield(nil, err) }
	}
	return func(yield func(*ResponseChunk, error) bool) {
		yield(&ResponseChunk{Content: resp.Content, Usage: resp.Usage, IsFinished: true}, nil)
	}
}

type spyLLMClient struct {
	LLMClient
	lastExec *PromptExecution
}

func (s *spyLLMClient) Generate(ctx context.Context, exec *PromptExecution) (*Response, error) {
	s.lastExec = exec
	return s.LLMClient.Generate(ctx, exec)
}
