package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCacheOmissionWireCloneAndNormalization(t *testing.T) {
	t.Parallel()
	// Arrange: omission is not nil/inheritance and is not a remote cache control.
	exec := SimplePrompt("fixture")
	exec.Messages[0].Content[0] = TextPart{Text: "fixture", CachePolicy: &CachePolicy{
		Mechanism: CacheOmit, Mode: "", Provider: "", Controls: nil,
	}}
	// Act.
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	payload, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(payload)
	require.NoError(t, err)
	cloned := restored.Clone().Normalize()
	// Assert.
	policy := cloned.Messages[0].Content[0].(TextPart).CachePolicy
	require.Equal(t, &CachePolicy{Mechanism: CacheOmit, Mode: "", Provider: "", Controls: nil}, policy)
	require.False(t, EqualCachePolicy(policy, nil))
	policy.Mechanism = CacheBlockMarker
	require.Equal(t, CacheOmit, restored.Messages[0].Content[0].(TextPart).CachePolicy.Mechanism)
}

func TestCacheOmissionRejectsRemoteGuarantees(t *testing.T) {
	t.Parallel()
	for _, policy := range []*CachePolicy{
		{Mechanism: CacheOmit, Mode: CacheRequired, Provider: "", Controls: nil},
		{Mechanism: CacheOmit, Mode: "", Provider: "fixture", Controls: nil},
		{Mechanism: CacheOmit, Mode: "", Provider: "", Controls: JSONDocument(`{}`)},
	} {
		// Arrange/Act.
		err := ValidateCachePolicy(policy)
		// Assert.
		require.ErrorIs(t, err, ErrCachePolicy)
	}
}

func TestCacheOmissionExecutableWireRejectsExtraControls(t *testing.T) {
	t.Parallel()
	for _, field := range []string{`"mode":"advisory"`, `"provider":"fixture"`, `"controls":{}`} {
		// Arrange: otherwise valid execution wire; omission has no provider controls.
		exec := SimplePrompt("fixture")
		wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
		require.NoError(t, err)
		payload, err := json.Marshal(wire)
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(payload, &value))
		var policy map[string]any
		require.NoError(t, json.Unmarshal([]byte(`{"mechanism":"omit",`+field+`}`), &policy))
		value["messages"].([]any)[0].(map[string]any)["cache_policy"] = policy
		invalid, err := json.Marshal(value)
		require.NoError(t, err)
		// Act.
		_, decodeErr := DecodeExecutionJSON(invalid)
		// Assert: executable JSON contract rejects before semantic DTO restoration.
		require.ErrorIs(t, decodeErr, ErrSchemaMismatch)
	}
}
