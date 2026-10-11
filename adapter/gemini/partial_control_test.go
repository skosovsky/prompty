package gemini

import (
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

func partialExecution() *prompty.PromptExecution {
	execution := prompty.SimplePrompt("fixture")
	execution.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
	execution.Requirements.Capabilities = []string{CapabilityIncrementalToolArguments}
	return execution
}

func TestNativePartialControlActualVertexSDKRequest(t *testing.T) {
	t.Parallel()
	// Arrange: real Vertex request converter/HTTP, fake key and local SSE only.
	requests := make(chan map[string]any, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "fixture", http.StatusBadRequest)
			return
		}
		requests <- request
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = io.WriteString(
			w,
			"data: "+functionEvent(`{"name":"lookup","partialArgs":[{"jsonPath":"$.n","stringValue":"one"}]}`)+"\n\n",
		)
		_, _ = io.WriteString(w, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n")
	}))
	t.Cleanup(server.Close)
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendVertexAI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta1"},
	})
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider: "gemini",
			Endpoint: server.URL,
			Model:    "fixture",
		},
		Provenance: "local Vertex converter fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
			prompty.CapabilityTools:            prompty.CapabilitySupported,
			CapabilityIncrementalToolArguments: prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	// Act.
	stream := client.ExecuteStream(t.Context(), partialExecution(), prompty.StreamNative)
	for _, streamErr := range stream.Events() {
		require.NoError(t, streamErr)
	}
	// Assert: opt-in reaches actual SDK JSON and returns a complete canonical call.
	require.Equal(t, prompty.StreamCompleted, stream.Status().State)
	require.JSONEq(t, `{"n":"one"}`, stream.Status().Result.Content[0].(prompty.ToolCallPart).Args)
	actual := <-requests
	config := actual["toolConfig"].(map[string]any)["functionCallingConfig"].(map[string]any)
	require.Equal(t, true, config["streamFunctionCallArguments"])
}

func TestPartialControlCannotInventMLDevMapperOrUseSync(t *testing.T) {
	t.Parallel()
	// Arrange: the installed MLDev SDK mapper rejects this field, no HTTP allowed.
	var calls atomic.Int32
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
			calls.Add(1)
			return nil, io.ErrUnexpectedEOF
		})}, HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid"},
	})
	require.NoError(t, err)
	a := New(WithClient(sdk))
	// Act.
	_, translateErr := a.Translate(partialExecution())
	_, syncErr := a.Execute(t.Context(), &Request{Config: &genai.GenerateContentConfig{
		ToolConfig: &genai.ToolConfig{
			FunctionCallingConfig: &genai.FunctionCallingConfig{StreamFunctionCallArguments: new(true)},
		},
	}})
	_, nilErr := a.Execute(t.Context(), nil)
	// Assert: knowledge cannot implement a mapper; raw sync cannot stream args.
	require.ErrorIs(t, translateErr, adapter.ErrUnsupportedCapability)
	require.ErrorIs(t, syncErr, adapter.ErrUnsupportedCapability)
	require.ErrorIs(t, nilErr, adapter.ErrNilExecution)
	require.Zero(t, calls.Load())
}

func TestPartialControlPreservesForcedSelectionAndRejectsConflicts(t *testing.T) {
	t.Parallel()
	// Arrange: pure translation retains an existing explicit native forced choice.
	a := New()
	execution := partialExecution()
	execution.ForcedTool = "lookup"
	// Act.
	request, err := a.Translate(execution)
	// Assert.
	require.NoError(t, err)
	config := request.Config.ToolConfig.FunctionCallingConfig
	require.Equal(t, genai.FunctionCallingConfigModeAny, config.Mode)
	require.Equal(t, []string{"lookup"}, config.AllowedFunctionNames)
	require.True(t, *config.StreamFunctionCallArguments)
	for _, setting := range []any{false, "true", nil} {
		// Arrange/Act: required capability conflicts or nonboolean control fail.
		data, encodeErr := json.Marshal(map[string]any{"stream_function_call_arguments": setting})
		require.NoError(t, encodeErr)
		execution.ModelOptions = &prompty.ModelOptions{
			ProviderSettings: prompty.JSONDocument(data),
		}
		_, err = a.Translate(execution)
		// Assert.
		require.Error(t, err)
	}
}

func TestCheckedPartialKnowledgeRejectsBeforeProviderIO(t *testing.T) {
	t.Parallel()
	for _, support := range []prompty.CapabilitySupport{prompty.CapabilityUnknown, prompty.CapabilityUnsupported} {
		t.Run(string(support), func(t *testing.T) {
			t.Parallel()
			// Arrange: a real Vertex mapper exists, but caller knowledge denies guarantee.
			var calls atomic.Int32
			sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
				APIKey: "fixture", Backend: genai.BackendVertexAI,
				HTTPClient: &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
					calls.Add(1)
					return nil, io.ErrUnexpectedEOF
				})}, HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid"},
			})
			require.NoError(t, err)
			profile := prompty.CapabilityProfile{
				Identity: prompty.ProfileIdentity{
					Provider: "gemini",
					Endpoint: "https://fixture.invalid",
					Model:    "fixture",
				},
				Provenance: "fixture caller knowledge",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
					prompty.CapabilityTools:            prompty.CapabilitySupported,
					CapabilityIncrementalToolArguments: support,
				},
			}
			client := adapter.NewClient(
				New(WithClient(sdk), WithModel("fixture"), WithProfiles("https://fixture.invalid", profile)),
			)
			// Act.
			stream := client.ExecuteStream(t.Context(), partialExecution(), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert: no request, retry or downgrade to buffered/snapshot behavior.
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.ErrorIs(t, stream.Status().Err, adapter.ErrCapabilityRequirement)
			require.Zero(t, calls.Load())
		})
	}
}
