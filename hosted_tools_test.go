package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostedToolsExecutionOwnershipAndWire(t *testing.T) {
	t.Parallel()
	// Arrange: adapter-owned settings include an integer outside float64 precision.
	settings := JSONDocument(`{"limit":9007199254740993,"nested":{"enabled":true}}`)
	exec := SimplePrompt("fixture")
	exec.HostedTools = []HostedToolRequest{{Kind: "fixture.search", Settings: settings}}
	// Act: all exported ownership boundaries clone the declaration and its bytes.
	cloned := exec.Clone()
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	var decoded PromptExecutionWire
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	restored, err := UnmarshalExecution(decoded)
	require.NoError(t, err)
	restored.HostedTools[0].Settings[2] = 'X'
	cloned.HostedTools[0].Kind = "changed"
	wire.HostedTools[0].Settings[2] = 'Y'
	// Assert: no aliases escape and exact JSON numbers survive serialization.
	require.Equal(t, "fixture.search", exec.HostedTools[0].Kind)
	require.Equal(t, settings, exec.HostedTools[0].Settings)
	require.Contains(t, string(encoded), `9007199254740993`)
	require.Equal(t, settings, cloned.HostedTools[0].Settings)
}

func TestHostedToolsPortableShapeAndBatchIdentity(t *testing.T) {
	t.Parallel()
	// Arrange.
	for _, declaration := range [][]HostedToolRequest{
		{{Kind: " "}}, {{Kind: "fixture"}, {Kind: "fixture"}},
		{{Kind: "fixture", Settings: JSONDocument(`null`)}},
		{{Kind: "fixture", Settings: JSONDocument(`[]`)}},
		{{Kind: "fixture", Settings: JSONDocument(`{} {}`)}},
	} {
		// Act / Assert: neither wire direction accepts a malformed declaration.
		exec := SimplePrompt("fixture")
		exec.HostedTools = declaration
		_, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
		require.ErrorIs(t, err, ErrHostedToolRequest)
		wire, err := MarshalExecution(SimplePrompt("fixture"), WirePolicy{State: StatePreserve})
		require.NoError(t, err)
		wire.HostedTools = declaration
		_, err = UnmarshalExecution(wire)
		require.Error(t, err)
	}
	exec := SimplePrompt("fixture")
	before, err := BatchExecutionDigest(exec)
	require.NoError(t, err)
	exec.HostedTools = []HostedToolRequest{{Kind: "fixture", Settings: JSONDocument(`{"enabled":false}`)}}
	after, err := BatchExecutionDigest(exec)
	require.NoError(t, err)
	exec.HostedTools[0].Settings = JSONDocument(`{"enabled":true}`)
	changed, err := BatchExecutionDigest(exec)
	require.NoError(t, err)
	require.NotEqual(t, before, after)
	require.NotEqual(t, after, changed)
}
