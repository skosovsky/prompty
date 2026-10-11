package ollama

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const formatFixtureSchema = `{"type":"object","properties":{"n":{"type":"integer","minimum":10}},"required":["n"],"additionalProperties":false}`

func task43Profile(endpoint string) prompty.CapabilityProfile {
	return prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "ollama", Endpoint: endpoint, Model: "fixture"},
		Provenance: "offline SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
			prompty.CapabilityReasoning:        prompty.CapabilitySupported,
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
		},
		ModelOptionsSchema: prompty.JSONDocument(
			`{"type":"object","properties":{"provider_settings":{"type":"object","properties":{"think":{"enum":[true,false,null,"low","medium","high"]}},"additionalProperties":false}}}`,
		),
	}
}

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestTask43NativeFormatAndThinkSDKWire(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		for _, output := range []string{`{"n":12}`, `{"n":1}`, `not JSON`} {
			t.Run(fmt.Sprintf("native=%v/output=%s", native, output), func(t *testing.T) {
				t.Parallel()
				// Arrange: actual api.Client serialization, caller profile and original schema.
				requests := make(chan map[string]json.RawMessage, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request map[string]json.RawMessage
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					requests <- request
					w.Header().Set("Content-Type", "application/x-ndjson")
					if native {
						_ = json.NewEncoder(w).
							Encode(api.ChatResponse{Message: api.Message{Role: "assistant", Thinking: "separate"}})
					}
					message := api.Message{Role: "assistant", Content: output}
					if !native {
						message.Thinking = "separate"
					}
					_ = json.NewEncoder(w).Encode(api.ChatResponse{Message: message, Done: true, DoneReason: "stop"})
				}))
				defer server.Close()
				endpoint, err := url.Parse(server.URL)
				require.NoError(t, err)
				client := adapter.NewClient(
					New(WithModel("fixture"), WithClient(api.NewClient(endpoint, server.Client())),
						WithProfiles(server.URL, task43Profile(server.URL))),
				)
				exec := prompty.SimplePrompt("fixture")
				exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(formatFixtureSchema)}
				exec.ModelOptions = &prompty.ModelOptions{ProviderSettings: prompty.JSONDocument(`{"think":"low"}`)}
				// Act.
				var response *prompty.Response
				if native {
					stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
					for _, streamErr := range stream.Events() {
						if streamErr != nil {
							err = streamErr
						}
					}
					response = stream.Status().Result
				} else {
					response, err = client.Execute(context.Background(), exec)
				}
				// Assert: reasoning is not output JSON; invalid original data never passes.
				if output == `{"n":12}` {
					require.NoError(t, err)
				} else {
					require.Error(t, err)
				}
				require.NotNil(t, response)
				request := <-requests
				require.JSONEq(t, formatFixtureSchema, string(request["format"]))
				require.JSONEq(t, `"low"`, string(request["think"]))
				require.JSONEq(t, strconv.FormatBool(native), string(request["stream"]))
				require.NotContains(t, string(request["options"]), "think")
				require.NotContains(t, string(request["messages"]), "additionalProperties")
				require.JSONEq(t, formatFixtureSchema, string(exec.ResponseFormat.Schema))
			})
		}
	}
}

func TestTask43ThinkAndFormatFailBeforeIO(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unknown", "no options schema", "unsupported", "invalid type", "invalid level", "forbidden combination"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer server.Close()
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			profile := task43Profile(server.URL)
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(formatFixtureSchema)}
			exec.ModelOptions = &prompty.ModelOptions{ProviderSettings: prompty.JSONDocument(`{"think":false}`)}
			switch scenario {
			case "unknown":
				profile.Capabilities = nil
			case "no options schema":
				profile.ModelOptionsSchema = nil
			case "unsupported":
				profile.Capabilities[prompty.CapabilityStructuredOutput] = prompty.CapabilityUnsupported
			case "invalid type":
				exec.ModelOptions.ProviderSettings = prompty.JSONDocument(`{"think":1}`)
			case "invalid level":
				exec.ModelOptions.ProviderSettings = prompty.JSONDocument(`{"think":"unknown"}`)
			case "forbidden combination":
				profile.ForbiddenCombinations = [][]string{
					{prompty.CapabilityStructuredOutput, prompty.CapabilityReasoning},
				}
			}
			a := New(
				WithModel("fixture"),
				WithClient(api.NewClient(endpoint, server.Client())),
				WithProfiles(server.URL, profile),
			)
			// Act/Assert: direct translation and both checked ports reject before transport.
			_, err = a.Translate(exec)
			require.Error(t, err)
			client := adapter.NewClient(a)
			_, err = client.Execute(context.Background(), exec)
			require.Error(t, err)
			stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
			err = nil
			for _, streamErr := range stream.Events() {
				if streamErr != nil {
					err = streamErr
				}
			}
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}
