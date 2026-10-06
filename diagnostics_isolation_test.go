package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestExecutionDiagnosticsCollectionsAreIsolatedInBothDirections(t *testing.T) {
	t.Parallel()
	// Arrange: every nested mutable diagnostics field is populated.
	source := ExecutionDiagnostics{
		Checks: []RequirementCheck{{Requirement: "tools", Status: CapabilitySupported}},
		Cache:  []CacheDecision{{Path: "/messages/0", Mechanism: CacheBlockMarker, Disposition: CacheAccepted}},
		Schemas: []SchemaCompilationReport{
			{Transformations: []SchemaTransformation{{ID: "original", Pointer: "/properties/x"}}},
		},
		Capture: &CaptureReport{
			InvocationID: "fixture",
			Coverage:     []CaptureStageCoverage{{Stage: CaptureExecution, Accuracy: CaptureSemantic}},
			Observed: []CaptureStage{
				CaptureExecution,
			},
			Failures: []CaptureFailure{{Stage: CaptureExecution, Kind: "sink"}},
		},
	}
	baseline, err := json.Marshal(source)
	require.NoError(t, err)
	// Act.
	copyOne := CloneExecutionDiagnostics(source)
	copyTwo := CloneExecutionDiagnostics(source)
	mutateDiagnosticsCollections(&copyOne)
	// Assert: one returned view cannot change any source or sibling collection.
	for _, unchanged := range []ExecutionDiagnostics{source, copyTwo} {
		encoded, encodeErr := json.Marshal(unchanged)
		require.NoError(t, encodeErr)
		require.JSONEq(t, string(baseline), string(encoded))
	}
	// Act.
	mutateDiagnosticsCollections(&source)
	// Assert: source-side mutation cannot change the retained view either.
	retained, err := json.Marshal(copyTwo)
	require.NoError(t, err)
	require.JSONEq(t, string(baseline), string(retained))
}

func mutateDiagnosticsCollections(diagnostics *ExecutionDiagnostics) {
	diagnostics.Checks[0].Status = CapabilityUnsupported
	diagnostics.Cache[0].Reason = "changed"
	diagnostics.Schemas[0].Role = SchemaToolInput
	diagnostics.Schemas[0].Transformations[0].ID = "changed"
	diagnostics.Capture.InvocationID = "changed"
	diagnostics.Capture.Coverage[0].Accuracy = CaptureMetadata
	diagnostics.Capture.Observed[0] = CaptureTerminal
	diagnostics.Capture.Failures[0].Kind = "changed"
}
