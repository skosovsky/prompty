package adapter_test

import (
	"context"
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

func TestRuntimeClient_DescriptorAndEstimator(t *testing.T) {
	t.Parallel()
	client := adapter.NewRuntimeClient(openaiadapter.New())

	desc, err := client.RuntimeDescriptor()
	require.NoError(t, err)
	assert.Equal(t, "openai", desc.Name)
	assert.NotContains(t, desc.Capabilities, "token_estimation")

	estimator, err := client.TokenEstimator()
	require.NoError(t, err)
	n, err := estimator.EstimateTokens(sampleExecution(t))
	require.NoError(t, err)
	assert.Positive(t, n)
}

func TestRuntimeClient_DescriptorsForAdapters(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		descriptor func() (adapter.RuntimeDescriptor, error)
		wantName   string
	}{
		{
			name: "openai",
			descriptor: func() (adapter.RuntimeDescriptor, error) {
				return adapter.NewRuntimeClient(openaiadapter.New()).RuntimeDescriptor()
			},
			wantName: "openai",
		},
		{
			name: "anthropic",
			descriptor: func() (adapter.RuntimeDescriptor, error) {
				return adapter.NewRuntimeClient(anthropicadapter.New()).RuntimeDescriptor()
			},
			wantName: "anthropic",
		},
		{
			name: "gemini",
			descriptor: func() (adapter.RuntimeDescriptor, error) {
				return adapter.NewRuntimeClient(geminiadapter.New()).RuntimeDescriptor()
			},
			wantName: "gemini",
		},
		{
			name: "ollama",
			descriptor: func() (adapter.RuntimeDescriptor, error) {
				return adapter.NewRuntimeClient(ollamaadapter.New()).RuntimeDescriptor()
			},
			wantName: "ollama",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			desc, err := tt.descriptor()

			require.NoError(t, err)
			assert.Equal(t, tt.wantName, desc.Name)
			assert.Empty(t, desc.Capabilities)
		})
	}
}

func TestRuntimeClient_MissingDescriptorAndEstimatorReturnExplicitErrors(t *testing.T) {
	t.Parallel()
	client := adapter.NewRuntimeClient(runtimeMockAdapter{})

	_, descErr := client.RuntimeDescriptor()
	_, estimatorErr := client.TokenEstimator()

	require.ErrorIs(t, descErr, adapter.ErrNoRuntimeDescriptor)
	require.ErrorIs(t, estimatorErr, adapter.ErrNoTokenEstimator)
}

type runtimeMockAdapter struct{}

func (runtimeMockAdapter) Translate(*prompty.PromptExecution) (struct{}, error) {
	return struct{}{}, nil
}

func (runtimeMockAdapter) Execute(context.Context, struct{}) (struct{}, error) {
	return struct{}{}, nil
}

func (runtimeMockAdapter) ParseResponse(struct{}) (*prompty.Response, error) {
	return &prompty.Response{}, nil
}
