package gemini

import (
	"context"
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

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestSDKUnsignedArgumentsPrecisionAdmissionAndIDs(t *testing.T) {
	t.Parallel()
	for _, args := range []string{
		`{"integer":9007199254740993}`, `{"nested":[{"negative":-9007199254740993,"fraction":0.10000000000000001,"exponent":1.234567890123456789e2}]}`, `{}`,
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			t.Run(args+string(mode), func(t *testing.T) {
				t.Parallel()
				// Arrange: actual SDK transport supplies unsigned parallel calls of one function.
				var count atomic.Int64
				requests := make(chan []byte, 2)
				first := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"first","name":"lookup","args":` + args + `}},{"functionCall":{"id":"second","name":"lookup","args":` + args + `}}]},"finishReason":"STOP"}]}`
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, err := io.ReadAll(r.Body)
					if err != nil {
						http.Error(w, "read", http.StatusInternalServerError)
						return
					}
					requests <- body
					response := first
					if count.Add(1) > 1 {
						response = `{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`
					}
					if strings.Contains(r.URL.Path, "streamGenerateContent") {
						w.Header().Set("Content-Type", "text/event-stream")
						_, _ = io.WriteString(w, "data: "+response+"\n\n")
					} else {
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, response)
					}
				}))
				t.Cleanup(server.Close)
				sdk, err := genai.NewClient(
					t.Context(),
					&genai.ClientConfig{
						APIKey:      "fixture",
						Backend:     genai.BackendGeminiAPI,
						HTTPClient:  server.Client(),
						HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
					},
				)
				require.NoError(t, err)
				profile := prompty.CapabilityProfile{
					Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
					Provenance: "local SDK conformance fixture",
					Capabilities: map[string]prompty.CapabilitySupport{
						prompty.CapabilityNativeStream: prompty.CapabilitySupported,
					},
				}
				client := adapter.NewClient(
					New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)),
				)
				// Act: real ingress -> response wire -> history -> next actual SDK request.
				response, err := checkedIngressInvoke(
					context.Background(),
					client,
					prompty.SimplePrompt("fixture"),
					mode,
				)
				if args != "{}" {
					// SDK-GENAI-NUM: no checked original-number evidence survives SDK decoding.
					require.Error(t, err)
					require.EqualValues(t, 1, count.Load())
					return
				}
				require.NoError(t, err)
				wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
				require.NoError(t, err)
				restored, err := prompty.UnmarshalResponse(wire)
				require.NoError(t, err)
				message, err := restored.AssistantMessage()
				require.NoError(t, err)
				execution := prompty.SimplePrompt("fixture").
					AddMessage(message).
					AddMessage(prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{
						prompty.ToolResultPart{
							ToolCallID: "second",
							Name:       "lookup",
							Content:    []prompty.ContentPart{prompty.TextPart{Text: "bad"}},
							IsError:    true,
						},
						prompty.ToolResultPart{
							ToolCallID: "first",
							Name:       "lookup",
							Content:    []prompty.ContentPart{prompty.TextPart{Text: "good"}},
						},
					}})
				_, err = checkedIngressInvoke(t.Context(), client, execution, mode)
				// Assert: repeated names retain their IDs and exact args through both exchanges.
				require.NoError(t, err)
				require.Equal(t, int64(2), count.Load())
				for _, part := range restored.Content {
					call := part.(prompty.ToolCallPart)
					require.Equal(t, geminiJSONValue([]byte(args)), geminiJSONValue([]byte(call.Args)))
				}
				if args == `{"integer":9007199254740993}` {
					tool, bindErr := prompty.NewTypedTool("lookup", func(_ context.Context, value struct {
						Integer int64 `json:"integer"`
					}) (int64, error) {
						return value.Integer, nil
					})
					require.NoError(t, bindErr)
					for _, part := range restored.Content {
						value, invokeErr := tool.Invoke(t.Context(), part.(prompty.ToolCallPart).Args)
						require.NoError(t, invokeErr)
						require.Equal(t, int64(9007199254740993), value)
					}
				}
				<-requests
				var request struct {
					Contents []struct {
						Parts []json.RawMessage `json:"parts"`
					} `json:"contents"`
				}
				require.NoError(t, json.Unmarshal(<-requests, &request))
				require.Len(t, request.Contents, 3)
				for index, id := range []string{"first", "second"} {
					native := geminiJSONValue(request.Contents[1].Parts[index]).(map[string]any)["functionCall"].(map[string]any)
					require.Equal(t, id, native["id"])
					actual, ok := native["args"]
					if !ok {
						actual = map[string]any{}
					}
					require.Equal(t, geminiJSONValue([]byte(args)), actual)
				}
				for index, id := range []string{"second", "first"} {
					native := geminiJSONValue(request.Contents[2].Parts[index]).(map[string]any)["functionResponse"].(map[string]any)
					require.Equal(t, id, native["id"])
					key := "output"
					if index == 0 {
						key = "error"
					}
					require.Contains(t, native["response"], key)
				}
			})
		}
	}
}

func TestIDlessHistoryRequiresConfirmedNameCorrelation(t *testing.T) {
	t.Parallel()
	for _, support := range []prompty.CapabilitySupport{prompty.CapabilityUnknown, prompty.CapabilityUnsupported, prompty.CapabilitySupported} {
		t.Run(string(support), func(t *testing.T) {
			t.Parallel()
			// Arrange: the host supplies ID-less history without an explicit requirement.
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(
					w,
					`{"candidates":[{"content":{"parts":[{"text":"done"}]},"finishReason":"STOP"}]}`,
				)
			}))
			t.Cleanup(server.Close)
			sdk, err := genai.NewClient(
				t.Context(),
				&genai.ClientConfig{
					APIKey:      "fixture",
					Backend:     genai.BackendGeminiAPI,
					HTTPClient:  server.Client(),
					HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
				},
			)
			require.NoError(t, err)
			profile := prompty.CapabilityProfile{
				Identity:     prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
				Provenance:   "local name correlation conformance",
				Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityToolNameCorrelation: support},
			}
			client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
			execution := prompty.SimplePrompt("fixture").
				AddMessage(prompty.ChatMessage{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.ToolCallPart{Name: "lookup", Args: "{}"}}}).
				AddMessage(prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{prompty.ToolResultPart{Name: "lookup", Content: []prompty.ContentPart{prompty.TextPart{Text: "result"}}}}})
			// Act.
			_, err = client.Execute(t.Context(), execution)
			// Assert: fail before transport unless the profile confirms native name correlation.
			if support == prompty.CapabilitySupported {
				require.NoError(t, err)
				require.Equal(t, int64(1), calls.Load())
			} else {
				require.ErrorIs(t, err, adapter.ErrCapabilityRequirement)
				require.Zero(t, calls.Load())
			}
		})
	}
}
