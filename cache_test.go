package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCacheWireCloneAndRequiredNormalization(t *testing.T) {
	t.Parallel()
	// Arrange.
	policy := &CachePolicy{
		Mechanism: CacheBlockMarker,
		Mode:      CacheRequired,
		Provider:  "fixture",
		Controls:  JSONDocument(`{"ttl":"1h"}`),
	}
	exec := NewExecution(
		[]ChatMessage{{Role: RoleSystem, CachePolicy: policy, Content: []ContentPart{TextPart{Text: "one"}}},
			{Role: RoleSystem, Content: []ContentPart{TextPart{Text: "two"}}}},
	)
	// Act.
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	payload, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(payload)
	require.NoError(t, err)
	cloned := restored.Clone()
	cloned.Messages[0].CachePolicy.Controls[0] = 'x'
	// Assert.
	require.Equal(t, policy, restored.Messages[0].CachePolicy)
	require.Equal(t, policy, exec.Messages[0].CachePolicy)
	require.Len(t, restored.Normalize().Messages, 2)
	require.NotContains(t, string(payload), `"type":"ephemeral"`)
}

func TestCacheRejectsLegacyAndInvalidControls(t *testing.T) {
	t.Parallel()
	for _, controls := range []JSONDocument{JSONDocument(`null`), JSONDocument(`[]`), JSONDocument(`{"x":`)} {
		// Arrange.
		exec := SimplePrompt("fixture")
		exec.Messages[0].CachePolicy = &CachePolicy{
			Mechanism: CacheBlockMarker,
			Provider:  "fixture",
			Controls:  controls,
		}
		// Act.
		_, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
		// Assert.
		require.ErrorIs(t, err, ErrCachePolicy)
	}
	// Arrange.
	legacy := JSONDocument(
		`{"format":"prompty.execution.v2","messages":[{"role":"user","content":[],"cache_policy":{"type":"ephemeral"}}]}`,
	)
	// Act.
	_, err := DecodeExecutionJSON(legacy)
	// Assert.
	require.Error(t, err)
}
