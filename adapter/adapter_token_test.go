package adapter_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	anthropicadapter "github.com/skosovsky/prompty/adapter/anthropic"
	geminiadapter "github.com/skosovsky/prompty/adapter/gemini"
	ollamaadapter "github.com/skosovsky/prompty/adapter/ollama"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

func sampleExecution(t *testing.T) *prompty.PromptExecution {
	t.Helper()
	return prompty.NewExecution([]prompty.ChatMessage{
		prompty.NewUserMessage("hello world"),
	})
}

type sentinelEstimator struct{}

func (sentinelEstimator) EstimateTokens(exec *prompty.PromptExecution) (int, error) {
	if exec == nil {
		return 0, adapter.ErrNilExecution
	}
	return 777, nil
}

func TestEstimateTokens_DispatchUsesAdapterEstimator(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(sentinelEstimator{}, sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 777, n)
}

func TestEstimateTokens_OpenAIAdapter(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(openaiadapter.New(), sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 3, n) // "hello world" => 11 chars / 4
}

func TestEstimateTokens_AnthropicAdapter(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(anthropicadapter.New(), sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 4, n)
}

func TestEstimateTokens_GeminiAdapter(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(geminiadapter.New(), sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestEstimateTokens_OllamaAdapter(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(ollamaadapter.New(), sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestEstimateTokens_RequiresEstimatorByDefault(t *testing.T) {
	t.Parallel()
	_, err := adapter.EstimateTokens(nil, sampleExecution(t))
	require.ErrorIs(t, err, adapter.ErrNoTokenEstimator)
}

func TestEstimateTokens_NilExecution(t *testing.T) {
	t.Parallel()
	_, err := adapter.EstimateTokens(openaiadapter.New(), nil)
	require.ErrorIs(t, err, adapter.ErrNilExecution)
}

func TestEstimateTokensStrict_RequiresEstimator(t *testing.T) {
	t.Parallel()
	_, err := adapter.EstimateTokensStrict(nil, sampleExecution(t))
	require.ErrorIs(t, err, adapter.ErrNoTokenEstimator)
}

func TestEstimateTokensStrict_UsesAdapterEstimator(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokensStrict(openaiadapter.New(), sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestEstimateTokens_ProviderRatiosDiffer(t *testing.T) {
	t.Parallel()
	exec := prompty.NewExecution([]prompty.ChatMessage{
		prompty.NewUserMessage("hello world test"),
	})
	openaiN, err := adapter.EstimateTokens(openaiadapter.New(), exec)
	require.NoError(t, err)
	anthropicN, err := adapter.EstimateTokens(anthropicadapter.New(), exec)
	require.NoError(t, err)
	assert.Greater(t, anthropicN, openaiN)
}

func TestRuntimeClient_EstimateTokensDispatchesToAdapter(t *testing.T) {
	t.Parallel()
	client := adapter.NewRuntimeClient(openaiadapter.New())
	n, err := client.EstimateTokens(sampleExecution(t))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestRuntimeClient_EstimateTokensNilExecution(t *testing.T) {
	t.Parallel()
	client := adapter.NewRuntimeClient(openaiadapter.New())
	_, err := client.EstimateTokens(nil)
	require.ErrorIs(t, err, adapter.ErrNilExecution)
}
