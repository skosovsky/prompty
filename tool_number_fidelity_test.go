package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolAssemblerAndDynamicDecodePreserveNumbers(t *testing.T) {
	t.Parallel()
	// Arrange: JSON numbers remain valid beyond the float64 range and precision.
	arguments := `{"id":9007199254740993,"values":[1.234567890123456789,1e1000]}`
	for name, parts := range map[string][]ContentPart{
		"snapshot": {ToolCallPart{ID: "a", Name: "lookup", Args: arguments}},
		"deltas": {
			ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: `{"id":9007199254740993,`},
			ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: `"values":[1.234567890123456789,1e1000]}`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Act.
			assembled, err := AssembleToolCallParts(parts)
			require.NoError(t, err)
			args, err := DecodeToolArgs[map[string]any](assembled[0].(ToolCallPart).Args)
			// Assert: dynamic DTOs see exact numbers, not rounded interface floats.
			require.NoError(t, err)
			require.Equal(t, json.Number("9007199254740993"), args["id"])
			require.Equal(t, []any{json.Number("1.234567890123456789"), json.Number("1e1000")}, args["values"])
		})
	}
}
