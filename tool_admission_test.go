package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAdmitToolCallsWholeBatchAndSnapshots(t *testing.T) {
	t.Parallel()
	// Arrange: use the real typed decoder; admission must never execute it.
	invocations := 0
	tool, err := NewTypedTool("lookup", func(_ context.Context, a struct {
		ID int64 `json:"id"`
	}) (int64, error) {
		invocations++
		return a.ID, nil
	})
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, tool))
	execution := SimplePrompt("lookup")
	execution.Tools = registry.Definitions()
	response := NewResponse([]ContentPart{
		ToolCallPart{ID: "one", Name: "lookup", Args: `{"id":9007199254740993}`},
		ToolCallPart{ID: "two", Name: "lookup", Args: `{"id":"invalid"}`},
	})
	// Act.
	calls, err := AdmitToolCalls(t.Context(), execution, response, registry)
	// Assert.
	require.Nil(t, calls)
	var batchErr *ToolCallError
	require.ErrorAs(t, err, &batchErr)
	require.Zero(t, invocations)
	response.Content[1] = ToolCallPart{ID: "two", Name: "lookup", Args: `{"id":9007199254740993}`}
	calls, err = AdmitToolCalls(t.Context(), execution, response, registry)
	require.NoError(t, err)
	require.Equal(t, "one", calls[0].ID)
	require.Equal(t, "two", calls[1].ID)
	require.Equal(t, `{"id":9007199254740993}`, calls[0].Args)
	calls[0].ID = "mutated"
	require.Equal(t, "one", response.Content[0].(ToolCallPart).ID)
	require.JSONEq(t, `{"id":"invalid"}`, batchErr.Response.Content[1].(ToolCallPart).Args)
	require.Zero(t, invocations)
}

func TestAdmitToolCallsFailClosed(t *testing.T) {
	t.Parallel()
	// Arrange.
	registry := NewTypedToolRegistry()
	tool, err := NewTypedTool("lookup", func(_ context.Context, _ struct{}) (string, error) { return "ok", nil })
	require.NoError(t, err)
	require.NoError(t, RegisterTool(registry, tool))
	execution := SimplePrompt("lookup")
	execution.Tools = registry.Definitions()
	for _, tc := range []struct {
		name     string
		response *Response
		want     error
	}{
		{"nil", nil, ErrResponseOutcome},
		{"refusal", &Response{Outcome: OutcomeRefusal}, ErrResponseOutcome},
		{"incomplete", &Response{Outcome: OutcomeIncomplete}, ErrResponseOutcome},
		{"paused", &Response{Outcome: OutcomePaused}, ErrResponseOutcome},
		{"unavailable", &Response{Outcome: OutcomeCompleted, ContinuationUnavailable: true}, ErrStateUnavailable},
		{"duplicate", NewResponse([]ContentPart{ToolCallPart{ID: "same", Name: "lookup", Args: `{}`}, ToolCallPart{ID: "same", Name: "lookup", Args: `{}`}}), ErrToolScope},
		{"idless", NewResponse([]ContentPart{ToolCallPart{Name: "lookup", Args: `{}`}}), ErrToolScope},
		{"unknown", NewResponse([]ContentPart{ToolCallPart{ID: "one", Name: "admin", Args: `{}`}}), ErrToolScope},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Act.
			calls, admissionErr := AdmitToolCalls(t.Context(), execution, tc.response, registry)
			// Assert.
			require.ErrorIs(t, admissionErr, tc.want)
			require.Nil(t, calls)
		})
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	calls, err := AdmitToolCalls(ctx, execution, NewResponse(nil), registry)
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, calls)
}
