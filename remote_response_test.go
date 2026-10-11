package prompty

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestRemoteResponseHandleWire(t *testing.T) {
	t.Parallel()
	// Arrange: a scoped identity without runtime transport or credentials.
	handle := RemoteResponseHandle{Format: RemoteResponseWireFormat,
		Identity: ProfileIdentity{
			Provider: "fixture.responses",
			Endpoint: "https://fixture.invalid/v1",
			Model:    "fixture",
		},
		AccountBinding: "host-account", ProviderID: "remote-identity", Status: RemoteResponseQueued,
		Background: true, Store: false, ProviderStateRequired: true, OutputSchemaDigest: strings.Repeat("a", 64),
		HostedTools:       []string{"web_search"},
		ToolSchemaDigests: map[string]string{"lookup": strings.Repeat("b", 64)}, ForcedTool: "lookup",
		ToolCalls: map[string]string{"call_fixture": "lookup"},
	}
	// Act.
	document, err := EncodeRemoteResponseHandle(handle)
	require.NoError(t, err)
	restored, err := DecodeRemoteResponseHandle(document)
	// Assert: false storage intent remains explicit and schema binding is immutable.
	require.NoError(t, err)
	require.Equal(t, handle, restored)
	require.Contains(t, string(document), `"store":false`)
	require.Contains(t, string(document), `"provider_state_required":true`)
	cloned := CloneRemoteResponseHandle(handle)
	cloned.HostedTools[0] = "different"
	cloned.ToolSchemaDigests["lookup"] = strings.Repeat("c", 64)
	cloned.ToolCalls["call_fixture"] = "changed"
	require.Equal(t, "lookup", handle.ToolCalls["call_fixture"])
	require.Equal(t, strings.Repeat("b", 64), handle.ToolSchemaDigests["lookup"])
	require.Equal(t, "web_search", handle.HostedTools[0])
	for _, corrupt := range []string{
		strings.Replace(string(document), `"call_fixture":"lookup"`, `"call_fixture":"unknown"`, 1),
		strings.Replace(string(document), `"forced_tool":"lookup"`, `"forced_tool":"unknown"`, 1),
		strings.Replace(string(document), strings.Repeat("b", 64), "invalid-digest", 1),
		strings.Replace(string(document), `,"provider_state_required":true`, "", 1),
		strings.Replace(string(document), `"provider_state_required":true`, `"provider_state_required":null`, 1),
		strings.Replace(string(document), `"provider_state_required":true`, `"provider_state_required":"true"`, 1),
		strings.Replace(string(document), `"queued"`, `"accepted"`, 1),
		strings.Replace(string(document), `"host-account"`, `" "`, 1),
		strings.Replace(string(document), `"store":false`, `"store":null`, 1),
		strings.Replace(string(document), `["web_search"]`, `["web_search","web_search"]`, 1),
		strings.Replace(string(document), `"store":false`, `"credentials":"secret","store":false`, 1),
		strings.Replace(string(document), `https://fixture.invalid/v1`, `https://user:secret@fixture.invalid/v1`, 1),
	} {
		_, decodeErr := DecodeRemoteResponseHandle([]byte(corrupt))
		require.Error(t, decodeErr)
	}
}
