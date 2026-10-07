package anthropic

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestHostedBatchActualSDKPauseAndBinding(t *testing.T) {
	t.Parallel()
	// Arrange: batch uses the same hosted source contract as ordinary Messages.
	var calls atomic.Int32
	requests := make(chan []byte, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		requests <- body
		if strings.HasSuffix(r.URL.Path, "/results") {
			_, _ = fmt.Fprintf(
				w,
				`{"custom_id":"one","result":{"type":"succeeded","message":%s}}`+"\n",
				hostedHistoryFixture,
			)
			return
		}
		writeBatchFixture(w, "ended")
	}))
	defer server.Close()
	manager := batchFixtureManager(server)
	for _, capability := range []string{prompty.CapabilityHostedWebSearch, prompty.CapabilityHostedWebFetch,
		prompty.CapabilityProviderState, prompty.CapabilityTools} {
		manager.base.profiles[0].Capabilities[capability] = prompty.CapabilitySupported
	}
	exec := prompty.SimplePrompt("fixture")
	exec.HostedTools = []prompty.HostedToolRequest{{Kind: HostedWebSearch}, {Kind: HostedWebFetch}}
	exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object"}`)}}
	input := prompty.BatchSubmitRequest{Items: []prompty.BatchItem{{ID: "one", AttemptID: "first", Execution: exec}}}
	// Act.
	submitted, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	var request struct {
		Requests []struct {
			Params struct {
				Tools []json.RawMessage `json:"tools"`
			} `json:"params"`
		} `json:"requests"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	result, err := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
	// Assert: remote success means a received message, not completed generation.
	require.NoError(t, err)
	<-requests
	require.Len(t, request.Requests[0].Params.Tools, 3)
	require.Contains(t, string(request.Requests[0].Params.Tools[0]), "web_search_20250305")
	require.Contains(t, string(request.Requests[0].Params.Tools[1]), "web_fetch_20250910")
	require.Equal(t, prompty.BatchItemSuccess, result.Items[0].Outcome)
	require.Equal(t, prompty.OutcomePaused, result.Items[0].Response.Outcome)
	items, err := HostedItems(result.Items[0].Response)
	require.NoError(t, err)
	require.Len(t, items, 6)
	wire, err := prompty.MarshalResponse(result.Items[0].Response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	document, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(document)
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomePaused, restored.Outcome)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	continuation := exec.Clone()
	continuation.Messages = append(continuation.Messages, message)
	params, err := manager.base.Translate(continuation)
	require.NoError(t, err)
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	require.Contains(t, string(encoded), "encrypted_content")
	require.Contains(t, string(encoded), "signed==")
	// Act / Assert: changed hosted intent cannot rebind accepted batch work.
	exec.HostedTools[0].Settings = prompty.JSONDocument(`{"max_uses":2}`)
	_, err = manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
	require.ErrorIs(t, err, prompty.ErrBatchScope)
	require.EqualValues(t, 2, calls.Load())
}

func TestHostedBatchForbiddenCombinationZeroIO(t *testing.T) {
	t.Parallel()
	// Arrange: knowledge permits features individually but forbids their batch use.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeBatchFixture(w, "ended")
	}))
	defer server.Close()
	manager := batchFixtureManager(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityHostedWebSearch] = prompty.CapabilitySupported
	manager.base.profiles[0].ForbiddenCombinations = [][]string{
		{prompty.CapabilityBatchSubmit, prompty.CapabilityHostedWebSearch},
	}
	exec := prompty.SimplePrompt("fixture")
	exec.HostedTools = []prompty.HostedToolRequest{{Kind: HostedWebSearch}}
	// Act.
	_, err := manager.Submit(
		t.Context(),
		prompty.BatchSubmitRequest{Items: []prompty.BatchItem{{ID: "one", AttemptID: "first", Execution: exec}}},
	)
	// Assert.
	require.Error(t, err)
	require.Zero(t, calls.Load())
}
