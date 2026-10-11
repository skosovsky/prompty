package openai

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func task43ResponsesControlsProfile(manager *Responses) {
	profile := &manager.base.profiles[0]
	profile.Capabilities[prompty.CapabilityReasoning] = prompty.CapabilitySupported
	profile.Capabilities[prompty.CapabilityStrictTools] = prompty.CapabilitySupported
	profile.ModelOptionsSchema = prompty.JSONDocument(
		`{"type":"object","properties":{"provider_settings":{"type":"object","properties":{"reasoning_effort":{"enum":["low"]},"reasoning_summary":{"enum":["concise"]}},"additionalProperties":false}}}`,
	)
}

func TestResponsesControlsActualSDKAndStrictOriginal(t *testing.T) {
	t.Parallel()
	for _, invalidArgs := range []bool{false, true} {
		t.Run(
			map[bool]string{false: "valid", true: "invalid original args"}[invalidArgs],
			func(t *testing.T) {
				t.Parallel()
				// Arrange: native strict intent with exact original numeric enum.
				requests := make(chan []byte, 1)
				server := httptest.NewServer(
					http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
						body, _ := io.ReadAll(request.Body)
						requests <- body
						if invalidArgs {
							toolResponseFixture(
								w,
								`{"id":"fc_fixture","type":"function_call","call_id":"call_fixture","name":"lookup","arguments":"{\"id\":1}","status":"completed"}`,
							)
						} else {
							toolResponseFixture(w, responsesFunctionFixture)
						}
					}),
				)
				defer server.Close()
				manager := localToolsFixtureAdapter(server)
				task43ResponsesControlsProfile(manager)
				exec := prompty.SimplePrompt("fixture")
				exec.Tools = responseToolDefinitions()
				exec.ModelOptions = &prompty.ModelOptions{
					ProviderSettings: prompty.JSONDocument(
						`{"reasoning_effort":"low","reasoning_summary":"concise"}`,
					),
				}
				// Act.
				result, err := manager.Create(
					t.Context(),
					prompty.RemoteResponseRequest{
						Execution:   exec,
						Store:       new(false),
						StrictTools: true,
					},
				)
				// Assert: summary does not implicitly request encrypted state; arguments stay checked.
				if invalidArgs {
					require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
				} else {
					require.NoError(t, err)
				}
				require.NotNil(t, result)
				require.NotNil(t, result.Handle)
				var request map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(<-requests, &request))
				require.JSONEq(
					t,
					`{"effort":"low","summary":"concise"}`,
					string(request["reasoning"]),
				)
				require.Empty(t, request["include"])
				var tools []struct {
					Strict     bool            `json:"strict"`
					Parameters json.RawMessage `json:"parameters"`
				}
				require.NoError(t, json.Unmarshal(request["tools"], &tools))
				require.Len(t, tools, 1)
				require.True(t, tools[0].Strict)
				require.JSONEq(t, string(exec.Tools[0].Parameters), string(tools[0].Parameters))
				decoder := json.NewDecoder(bytes.NewReader(tools[0].Parameters))
				decoder.UseNumber()
				var parameters map[string]any
				require.NoError(t, decoder.Decode(&parameters))
				properties := parameters["properties"].(map[string]any)
				numbers := properties["id"].(map[string]any)["enum"].([]any)
				require.Equal(t, json.Number("9007199254740993"), numbers[0])
			},
		)
	}
}

func TestResponsesControlsAndStrictRejectBeforeIO(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unknown reasoning", "no options schema", "unsupported strict", "unknown settings",
		"invalid effort", "invalid summary", "open object", "optional", "nested open", "unsupported keyword", "missing type", "no tools", "forbidden combination"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: lowering permission must not authorize hidden strict tool edits.
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }),
			)
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			task43ResponsesControlsProfile(manager)
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = responseToolDefinitions()
			exec.ModelOptions = &prompty.ModelOptions{
				ProviderSettings: prompty.JSONDocument(`{"reasoning_effort":"low"}`),
			}
			exec.Requirements.Schema = prompty.SchemaPolicy{
				Mode:              prompty.SchemaLowering,
				AllowedTransforms: []string{LoweringCloseObject, LoweringRequiredNullable},
			}
			profile := &manager.base.profiles[0]
			switch scenario {
			case "unknown reasoning":
				delete(profile.Capabilities, prompty.CapabilityReasoning)
			case "no options schema":
				profile.ModelOptionsSchema = nil
			case "unsupported strict":
				profile.Capabilities[prompty.CapabilityStrictTools] = prompty.CapabilityUnsupported
			case "unknown settings":
				exec.ModelOptions.ProviderSettings = prompty.JSONDocument(`{"seed":1}`)
			case "invalid effort":
				exec.ModelOptions.ProviderSettings = prompty.JSONDocument(
					`{"reasoning_effort":"high"}`,
				)
			case "invalid summary":
				exec.ModelOptions.ProviderSettings = prompty.JSONDocument(
					`{"reasoning_summary":"detailed"}`,
				)
			case "open object":
				exec.Tools[0].Parameters = prompty.JSONDocument(`{"type":"object"}`)
			case "optional":
				exec.Tools[0].Parameters = prompty.JSONDocument(
					`{"type":"object","additionalProperties":false,"properties":{"n":{"type":"integer"}}}`,
				)
			case "nested open":
				exec.Tools[0].Parameters = prompty.JSONDocument(
					`{"type":"object","additionalProperties":false,"properties":{"n":{"type":"object"}},"required":["n"]}`,
				)
			case "unsupported keyword":
				exec.Tools[0].Parameters = prompty.JSONDocument(
					`{"type":"object","additionalProperties":false,"not":{"required":["n"]}}`,
				)
			case "missing type":
				exec.Tools[0].Parameters = prompty.JSONDocument(
					`{"type":"object","additionalProperties":false,"properties":{"n":{}},"required":["n"]}`,
				)
			case "no tools":
				exec.Tools = nil
			case "forbidden combination":
				profile.ForbiddenCombinations = [][]string{
					{prompty.CapabilityReasoning, prompty.CapabilityStrictTools},
				}
			}
			// Act.
			_, err := manager.Create(
				t.Context(),
				prompty.RemoteResponseRequest{
					Execution:   exec,
					Store:       new(false),
					StrictTools: true,
				},
			)
			// Assert.
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}

func TestResponsesAdapterWhitelistIndependentFromChat(t *testing.T) {
	t.Parallel()
	// Arrange: even an overly permissive host options schema cannot authorize passthrough.
	var calls atomic.Int32
	server := httptest.NewServer(
		http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }),
	)
	defer server.Close()
	manager := responseFixtureAdapter(server)
	task43ResponsesControlsProfile(manager)
	manager.base.profiles[0].ModelOptionsSchema = prompty.JSONDocument(`true`)
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{ProviderSettings: prompty.JSONDocument(`{"seed":1}`)}
	// Act.
	_, err := manager.Create(
		t.Context(),
		prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
	)
	// Assert.
	require.ErrorIs(t, err, adapter.ErrInvalidProviderSettings)
	require.Zero(t, calls.Load())
}
