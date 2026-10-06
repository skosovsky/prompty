package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureIntentPersistsButRuntimeBindingDoesNot(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := SimplePrompt("fixture")
	exec.Requirements.Capture = CaptureRequirement{
		Stages: []CaptureStage{CaptureExecution, CaptureTerminal}, Accuracy: CaptureSemantic, Durable: true,
	}
	exec.Capture = &CaptureBinding{CorrelationID: "runtime-only", MaxBytes: 1024}
	cloned := exec.Clone().Normalize().WithHistory(nil)
	// Act.
	wire, err := MarshalExecution(cloned, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(data)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, exec.Requirements.Capture, restored.Requirements.Capture)
	require.Nil(t, restored.Capture)
	require.NotContains(t, string(data), "runtime-only")
	cloned.Requirements.Capture.Stages[0] = CaptureProviderResponse
	cloned.Capture.MaxBytes = 1
	require.Equal(t, CaptureExecution, exec.Requirements.Capture.Stages[0])
	require.Equal(t, 1024, exec.Capture.MaxBytes)
}

func TestCaptureResponseDiagnosticsWireAndCloneIsolation(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := NewResponse([]ContentPart{TextPart{Text: "fixture"}})
	response.Diagnostics.Capture = &CaptureReport{
		InvocationID: "fixture-invocation", Observed: []CaptureStage{CaptureTerminal}, Closed: true,
		Coverage: []CaptureStageCoverage{{Stage: CaptureTerminal, Accuracy: CaptureMetadata}},
		Failures: []CaptureFailure{{Stage: CaptureProviderResponse, AfterDispatch: true, Kind: "sink"}},
	}
	// Act.
	wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeResponseJSON(data)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, response.Diagnostics.Capture, restored.Diagnostics.Capture)
	restored.Diagnostics.Capture.Failures[0].Kind = "mutated"
	restored.Diagnostics.Capture.Observed[0] = CaptureExecution
	require.Equal(t, "sink", response.Diagnostics.Capture.Failures[0].Kind)
	require.Equal(t, CaptureTerminal, response.Diagnostics.Capture.Observed[0])
}

func TestCaptureWireRejectsUnknownAndDuplicateIntent(t *testing.T) {
	t.Parallel()
	for _, policy := range []CaptureRequirement{
		{Accuracy: "invented"},
		{Stages: []CaptureStage{"invented"}},
		{Stages: []CaptureStage{CaptureExecution, CaptureExecution}},
	} {
		// Arrange.
		exec := SimplePrompt("fixture")
		exec.Requirements.Capture = policy
		// Act.
		_, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
		// Assert.
		require.ErrorIs(t, err, ErrCaptureUnsupported)
	}
}
