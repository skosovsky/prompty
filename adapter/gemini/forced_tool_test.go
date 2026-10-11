package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestForcedToolSelectionThroughActualSDK(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: actual SDK transport, not marshaling the adapter DTO directly.
			var calls atomic.Int32
			requests := make(chan []byte, 16)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "fixture read failure", http.StatusBadRequest)
					return
				}
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(
					w,
					`{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call_1","name":"lookup","args":{}}}]},"finishReason":"STOP"}]}`,
				)
			}))
			t.Cleanup(server.Close)
			sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
				APIKey: "fixture-key", Backend: genai.BackendGeminiAPI, Project: "", Location: "",
				Credentials: nil, HTTPClient: server.Client(),
				HTTPOptions: genai.HTTPOptions{
					BaseURL: server.URL, APIVersion: "v1beta", Headers: nil, Timeout: nil,
					ExtraBody: nil, ExtrasRequestProvider: nil,
				},
			})
			require.NoError(t, err)
			profile := geminiStateFixtureProfile()
			profile.Identity.Endpoint = server.URL
			profile.Capabilities = map[string]prompty.CapabilitySupport{
				prompty.CapabilityTools:               prompty.CapabilitySupported,
				prompty.CapabilityForcedToolSelection: prompty.CapabilitySupported,
			}
			client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
			exec := prompty.SimplePrompt("unchanged user instruction")
			exec.Tools = []prompty.ToolDefinition{
				{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object"}`)},
			}
			exec.ForcedTool = "lookup"
			before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			// Act.
			var response *prompty.Response
			if mode == "" {
				response, err = client.Execute(context.Background(), exec)
			} else {
				stream := client.ExecuteStream(context.Background(), exec, mode)
				for _, streamErr := range stream.Events() {
					require.NoError(t, streamErr)
				}
				response, err = stream.Status().Result, stream.Status().Err
			}
			// Assert.
			require.NoError(t, err)
			require.NotNil(t, response)
			require.Equal(t, prompty.OutcomeToolCalls, response.Outcome)
			require.Equal(t, int32(1), calls.Load())
			var wire map[string]any
			require.NoError(t, json.Unmarshal(<-requests, &wire))
			selection := wire["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)
			require.Equal(t, "ANY", selection["mode"])
			require.Equal(t, []any{"lookup"}, selection["allowedFunctionNames"])
			require.NotContains(t, wire, "systemInstruction")
			after, marshalErr := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, marshalErr)
			require.Equal(t, before, after)
		})
	}
}
