package openai

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestResponsesOperationsDoNotRequireGenerationOptions(t *testing.T) {
	t.Parallel()
	// Arrange: profile generation requires max_tokens, but Get/Cancel send no options.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseFixture(w, "completed", "accepted output")
	}))
	defer server.Close()
	manager := responseFixtureAdapter(server)
	manager.base.profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	handle := remoteFixtureHandle(manager)
	// Act.
	got, getErr := manager.Get(t.Context(), handle)
	canceled, cancelErr := manager.Cancel(t.Context(), handle)
	_, createErr := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("new completion"), Store: new(false),
	})
	// Assert: retrieval does not replay settings; generation still fails preflight.
	require.NoError(t, getErr)
	require.NoError(t, cancelErr)
	require.Equal(t, prompty.RemoteResponseCompleted, got.Handle.Status)
	require.Equal(t, prompty.RemoteResponseCompleted, canceled.Handle.Status)
	require.ErrorIs(t, createErr, adapter.ErrUnsupportedCapability)
	require.EqualValues(t, 2, calls.Load())
	for _, result := range []*prompty.RemoteResponseResult{got, canceled} {
		for _, check := range result.Diagnostics.Checks {
			require.NotEqual(t, "model_options", check.Requirement)
		}
	}
}
