package gemini

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestArgumentBindingAllowsOnlyLocalFloatProjection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		sdk      any
		accepted bool
	}{
		{"mixed exact mismatch", map[string]any{"n": int64(9007199254740992), "f": float64(1)}, false},
		{"mixed exact match", map[string]any{"n": int64(9007199254740993), "f": float64(1)}, true},
		{"SDK rounded leaf", map[string]any{"n": float64(9007199254740993), "f": float64(1)}, true},
		{"typed unsigned mismatch", map[string]uint64{"n": 9007199254740992, "f": 1}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			source := map[string]any{"n": json.Number("9007199254740993"), "f": json.Number("1")}
			// Act.
			result := matchesGeminiSDKArguments(source, test.sdk)
			// Assert.
			require.Equal(t, test.accepted, result)
		})
	}
}
