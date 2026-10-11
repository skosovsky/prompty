package gemini

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func mediaResultExecution(image prompty.MediaPart) *prompty.PromptExecution {
	exec := prompty.SimplePrompt("fixture")
	exec.Messages = append(exec.Messages, prompty.ChatMessage{Role: prompty.RoleAssistant,
		Content: []prompty.ContentPart{prompty.ToolCallPart{ID: "call_a", Name: "lookup", Args: "{}"}}},
		prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{
			prompty.ToolResultPart{ToolCallID: "call_a", Name: "lookup", Content: []prompty.ContentPart{
				prompty.TextPart{Text: "before"}, image, prompty.TextPart{Text: "after"},
				prompty.MediaPart{MediaType: "image", MIMEType: "image/jpeg", Data: []byte("second")},
			}},
		}})
	return exec
}

func mediaResultFixtureAdapter(t *testing.T, server *httptest.Server, backend genai.Backend) *Adapter {
	t.Helper()
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: backend, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "offline SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			CapabilityFunctionResultMedia:  prompty.CapabilitySupported,
			prompty.CapabilityNativeStream: prompty.CapabilitySupported,
		},
		InputMIME: []string{"image/png", "image/jpeg", "image/webp"},
	}
	return New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile))
}

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestFunctionResultMediaActualSDKWireOrder(t *testing.T) {
	t.Parallel()
	for _, backend := range []genai.Backend{genai.BackendGeminiAPI, genai.BackendVertexAI} {
		for _, native := range []bool{false, true} {
			t.Run(backend.String()+map[bool]string{false: "/sync", true: "/stream"}[native], func(t *testing.T) {
				t.Parallel()
				// Arrange: actual SDK transport receives a canonical execution after wire restoration.
				requests := make(chan []byte, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					requests <- body
					response := `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`
					if strings.Contains(r.URL.Path, "streamGenerateContent") {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+response+"\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, response)
					}
				}))
				defer server.Close()
				a := mediaResultFixtureAdapter(t, server, backend)
				image := prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte("first")}
				if backend == genai.BackendVertexAI {
					image.Data = nil
					image.URL = "gs://fixture-bucket/image.png"
				}
				exec := mediaResultExecution(image)
				document, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
				require.NoError(t, err)
				restored, err := prompty.UnmarshalExecution(document)
				require.NoError(t, err)
				client := adapter.NewClient(a)
				// Act.
				if native {
					stream := client.ExecuteStream(t.Context(), restored, prompty.StreamNative)
					for _, err := range stream.Events() {
						require.NoError(t, err)
					}
					require.Equal(t, prompty.StreamCompleted, stream.Status().State)
				} else {
					_, err = client.Execute(t.Context(), restored)
					require.NoError(t, err)
				}
				// Assert: native result ID, exact references and ordered media survive SDK mapping.
				var request struct {
					Contents []genai.Content `json:"contents"`
				}
				require.NoError(t, json.Unmarshal(<-requests, &request))
				result := request.Contents[2].Parts[0].FunctionResponse
				require.Equal(t, "call_a", result.ID)
				require.Equal(t, "lookup", result.Name)
				ordered, err := json.Marshal(result.Response)
				require.NoError(t, err)
				require.JSONEq(
					t,
					`{"output":["before",{"$ref":"prompty_part_1"},"after",{"$ref":"prompty_part_3"}]}`,
					string(ordered),
				)
				require.Len(t, result.Parts, 2)
				if backend == genai.BackendVertexAI {
					require.Equal(t, image.URL, result.Parts[0].FileData.FileURI)
					require.Equal(t, "image/png", result.Parts[0].FileData.MIMEType)
					require.Equal(t, "prompty_part_1", result.Parts[0].FileData.DisplayName)
				} else {
					require.Equal(t, image.Data, result.Parts[0].InlineData.Data)
					require.Equal(t, "image/png", result.Parts[0].InlineData.MIMEType)
					require.Equal(t, "prompty_part_1", result.Parts[0].InlineData.DisplayName)
				}
				require.Equal(t, []byte("second"), result.Parts[1].InlineData.Data)
				require.Equal(t, "prompty_part_3", result.Parts[1].InlineData.DisplayName)
			})
		}
	}
}

func TestFunctionResultMediaUnsupportedZeroIO(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unknown", "unsupported", "MIME knowledge", "invalid MIME", "unsupported media", "both forms", "no data", "URI on Gemini API", "invalid Vertex URI"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
			defer server.Close()
			backend := genai.BackendGeminiAPI
			if scenario == "invalid Vertex URI" {
				backend = genai.BackendVertexAI
			}
			a := mediaResultFixtureAdapter(t, server, backend)
			image := prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte("first")}
			switch scenario {
			case "unknown":
				delete(a.profiles[0].Capabilities, CapabilityFunctionResultMedia)
			case "unsupported":
				a.profiles[0].Capabilities[CapabilityFunctionResultMedia] = prompty.CapabilityUnsupported
			case "MIME knowledge":
				a.profiles[0].InputMIME = nil
			case "invalid MIME":
				image.MIMEType = "image/gif"
				a.profiles[0].InputMIME = append(a.profiles[0].InputMIME, "image/gif")
			case "unsupported media":
				image.MediaType = "document"
			case "both forms":
				image.URL = "gs://fixture-bucket/image.png"
			case "no data":
				image.Data = nil
			case "URI on Gemini API":
				image.Data = nil
				image.URL = "https://fixture.invalid/image.png"
			case "invalid Vertex URI":
				image.Data = nil
				image.URL = "gs://fixture-bucket/image.png?secret=true"
			}
			exec := mediaResultExecution(image)
			// Act/Assert.
			_, err := a.Translate(exec)
			require.Error(t, err)
			_, err = adapter.NewClient(a).Execute(t.Context(), exec)
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}
