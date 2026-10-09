//go:build integration

package anthropic_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	provider "github.com/skosovsky/prompty/adapter/anthropic"
)

func TestIntegrationRuntimeClientDescriptor(t *testing.T) {
	t.Parallel()
	// Arrange.
	client := adapter.NewRuntimeClient(provider.New())
	// Act.
	descriptor, err := client.RuntimeDescriptor()
	// Assert.
	require.NoError(t, err)
	assert.Equal(t, "anthropic", descriptor.Name)
	assert.Empty(t, descriptor.Capabilities)
}

func TestIntegrationEstimateTokens_AnthropicAdapter(t *testing.T) {
	t.Parallel()
	n, err := adapter.EstimateTokens(
		provider.New(),
		prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hello world")}),
	)
	require.NoError(t, err)
	assert.Equal(t, 4, n)
}

func TestIntegrationProviderTokenRatio(t *testing.T) {
	t.Parallel()
	// Arrange.
	execution := prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hello world test")})
	// Act.
	count, err := adapter.EstimateTokens(provider.New(), execution)
	// Assert.
	require.NoError(t, err)
	assert.Greater(t, count, 4) // OpenAI estimates four tokens for the same input.
}
