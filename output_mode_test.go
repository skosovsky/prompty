package prompty

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOutputModeWireAndClone(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := SimplePrompt("extract")
	exec.Requirements.OutputMode = OutputSyntheticTool
	exec.ResponseFormat = &SchemaDefinition{Name: "extract", Schema: JSONDocument(`{"type":"object"}`)}
	// Act.
	cloned := exec.Clone()
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	restored, err := UnmarshalExecution(wire)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, OutputSyntheticTool, cloned.Requirements.OutputMode)
	require.Equal(t, OutputSyntheticTool, restored.Requirements.OutputMode)
	cloned.Requirements.OutputMode = OutputNative
	require.Equal(t, OutputSyntheticTool, exec.Requirements.OutputMode)
}

func TestInvalidOutputModeWire(t *testing.T) {
	t.Parallel()
	for _, mode := range []StructuredOutputMode{OutputSyntheticTool, OutputNative, "future"} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exec := SimplePrompt("extract")
			exec.Requirements.OutputMode = mode
			// Act.
			_, err := MarshalExecution(exec, WirePolicy{})
			// Assert.
			require.ErrorIs(t, err, ErrOutputMode)
		})
	}
}
