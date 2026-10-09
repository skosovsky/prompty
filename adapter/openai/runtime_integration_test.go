//go:build integration

package openai_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	provider "github.com/skosovsky/prompty/adapter/openai"
)

func TestIntegrationRuntimeClientDescriptor(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := adapter.NewRuntimeClient(provider.New())
	// Act.
	descriptor, err := client.RuntimeDescriptor()
	// Assert.
	require.NoError(t, err)
	assert.Equal(t, "openai", descriptor.Name)
	assert.Empty(t, descriptor.Capabilities)
}

func TestIntegrationRuntimeClientTokenEstimator(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := adapter.NewRuntimeClient(provider.New())
	estimator, err := client.TokenEstimator()
	require.NoError(t, err)
	execution := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "hello"}}},
		},
	}
	// Act.
	count, err := estimator.EstimateTokens(execution)
	// Assert.
	require.NoError(t, err)
	assert.Positive(t, count)
}

func TestIntegrationEstimateTokens_OpenAIAdapter(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(
		provider.New(),
		prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hello world")}),
	)
	require.NoError(t, err)
	assert.Equal(t, 3, n) // "hello world" => 11 chars / 4
}

func TestIntegrationEstimateTokensStrict_UsesAdapterEstimator(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokensStrict(
		provider.New(),
		prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hello world")}),
	)
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestIntegrationRuntimeClient_EstimateTokensDispatchesToAdapter(t *testing.T) {
	t.Parallel()
	client := adapter.NewRuntimeClient(provider.New())
	n, err := client.EstimateTokens(prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hello world")}))
	require.NoError(t, err)
	assert.Equal(t, 3, n)
}

func TestIntegrationRuntimeClient_EstimateTokensNilExecution(t *testing.T) {
	t.Parallel()
	client := adapter.NewRuntimeClient(provider.New())
	_, err := client.EstimateTokens(nil)
	require.ErrorIs(t, err, adapter.ErrNilExecution)
}

func TestIntegrationProviderTokenRatio(t *testing.T) {
	t.Parallel()
	// Arrange.
	execution := prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hello world test")})
	// Act.
	count, err := adapter.EstimateTokens(provider.New(), execution)
	// Assert.
	require.NoError(t, err)
	assert.Equal(t, 4, count)
}
