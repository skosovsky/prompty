package adapter_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

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
