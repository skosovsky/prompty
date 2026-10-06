package openai

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestResponsesRequiredStateHandleSDKGetAndFreshCreate(t *testing.T) {
	t.Parallel()
	// Arrange: queued acceptance, persisted handle, then a rich terminal result.
	type observed struct{ method, query, body string }
	requests := make(chan observed, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- observed{request.Method, request.URL.RawQuery, string(body)}
		if request.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(
				w,
				strings.Replace(hostedOutputFixture, `"output":[`, `"output":[`+responseReasoningFixture+`,`, 1),
			)
			return
		}
		writeResponseFixture(w, "queued", "")
	}))
	defer server.Close()
	manager := hostedFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	exec := prompty.SimplePrompt("search")
	exec.Requirements.Capabilities = []string{prompty.CapabilityProviderState}
	// Act.
	accepted, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: exec, Store: new(true), Background: true,
		HostedTools: []prompty.HostedToolRequest{{Kind: HostedWebSearch}, {
			Kind: HostedFileSearch, Settings: []byte(`{"vector_store_ids":["vs_fixture"]}`),
		}},
	})
	require.NoError(t, err)
	document, err := prompty.EncodeRemoteResponseHandle(*accepted.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeRemoteResponseHandle(document)
	require.NoError(t, err)
	completed, err := manager.Get(t.Context(), handle)
	require.NoError(t, err)
	fresh, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("fresh"), Store: new(false), Previous: completed.Handle,
	})
	// Assert.
	require.NoError(t, err)
	require.True(t, handle.ProviderStateRequired)
	require.True(t, completed.Handle.ProviderStateRequired)
	require.False(t, fresh.Handle.ProviderStateRequired)
	require.Len(t, completed.Response.ProviderState, 1)
	require.Contains(t, string(document), `"provider_state_required":true`)
	created := <-requests
	require.Contains(t, created.body, `"reasoning.encrypted_content"`)
	get := <-requests
	require.Equal(t, http.MethodGet, get.method)
	require.Contains(t, get.query, "reasoning.encrypted_content")
	require.Contains(t, get.query, "web_search_call.action.sources")
	require.Contains(t, get.query, "file_search_call.results")
	require.Empty(t, get.body)
	newCreate := <-requests
	require.NotContains(t, newCreate.body, "reasoning.encrypted_content")
	require.NotContains(t, newCreate.body, "web_search")
	require.Contains(t, newCreate.body, `"instructions":""`)
	require.Contains(t, newCreate.body, `"previous_response_id":"resp_fixture"`)
	require.True(t, accepted.Handle.ProviderStateRequired)
	require.Equal(t, prompty.RemoteResponseQueued, handle.Status)
}

func TestResponsesRequiredStateGetCapabilityRejectsBeforeIO(t *testing.T) {
	t.Parallel()
	for _, support := range []prompty.CapabilitySupport{prompty.CapabilityUnknown, prompty.CapabilityUnsupported} {
		t.Run(string(support), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeReasoningResponse(w, responseReasoningFixture)
			}))
			defer server.Close()
			manager := responseFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = support
			handle := remoteFixtureHandle(manager)
			handle.ProviderStateRequired = true
			// Act.
			_, err := manager.Get(t.Context(), handle)
			// Assert.
			if support == prompty.CapabilityUnknown {
				require.ErrorIs(t, err, adapter.ErrUnknownProfile)
			} else {
				require.ErrorIs(t, err, adapter.ErrUnsupportedCapability)
			}
			require.Zero(t, calls.Load())
			require.True(t, handle.ProviderStateRequired)
		})
	}
}

func TestResponsesRequiredStateMissingAndUnavailableGet(t *testing.T) {
	t.Parallel()
	for _, status := range []int{http.StatusOK, http.StatusNotFound, http.StatusInternalServerError} {
		t.Run(http.StatusText(status), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			queries := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				queries <- request.URL.RawQuery
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(status)
				if status == http.StatusOK {
					writeReasoningResponse(w, `{"id":"rs_fixture","type":"reasoning","summary":[]}`)
				}
			}))
			defer server.Close()
			manager := reasoningFixtureAdapter(server)
			handle := remoteFixtureHandle(manager)
			handle.ProviderStateRequired = true
			// Act.
			known, err := manager.Get(t.Context(), handle)
			// Assert.
			require.EqualValues(t, 1, calls.Load())
			require.Contains(t, <-queries, "reasoning.encrypted_content")
			require.True(t, known.Handle.ProviderStateRequired)
			require.Equal(t, "resp_fixture", known.Handle.ProviderID)
			if status == http.StatusOK {
				require.ErrorIs(t, err, prompty.ErrStateUnavailable)
				require.Equal(t, prompty.RemoteResponseCompleted, known.Handle.Status)
				require.True(t, known.Response.ContinuationUnavailable)
				require.Equal(t, prompty.TextPart{Text: "answer"}, known.Response.Content[1])
			} else {
				require.ErrorIs(t, err, prompty.ErrRemoteResponseUnavailable)
				require.Equal(t, handle, *known.Handle)
			}
		})
	}
}

func TestResponsesCancelRetainsRequiredStateWithoutUnsupportedInclude(t *testing.T) {
	t.Parallel()
	// Arrange: cancel has its own capability and no native include parameter.
	var calls atomic.Int32
	requests := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		requests <- request.Method + " " + request.URL.String()
		writeReasoningResponse(w, `{"id":"rs_fixture","type":"reasoning","summary":[]}`)
	}))
	defer server.Close()
	manager := responseFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilityUnsupported
	manager.base.profiles[0].Capabilities[prompty.CapabilityResponseRetrieve] = prompty.CapabilityUnsupported
	handle := remoteFixtureHandle(manager)
	handle.ProviderStateRequired = true
	// Act.
	known, err := manager.Cancel(t.Context(), handle)
	// Assert: completed race remains known; missing state does not authorize replay.
	require.ErrorIs(t, err, prompty.ErrStateUnavailable)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, "POST /v1/responses/resp_fixture/cancel", <-requests)
	require.True(t, known.Handle.ProviderStateRequired)
	require.Equal(t, prompty.RemoteResponseCompleted, known.Handle.Status)
	require.True(t, known.Response.ContinuationUnavailable)
	require.Equal(t, prompty.RemoteResponseQueued, handle.Status)
}
