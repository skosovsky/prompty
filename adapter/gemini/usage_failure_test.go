package gemini

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

//nolint:gocognit // Compare sync/native retained usage through actual SDK acquisition in one fixture.
func TestSDKNumericFailureRetainsKnownUsageWithoutContent(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "native"}[native], func(t *testing.T) {
			t.Parallel()
			// Arrange: actual SDK receives a numeric call and nonzero usage.
			payload := `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"id":9007199254740993}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7}}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				body := payload
				if native {
					body = "data: " + payload + "\n\n"
				}
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
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
			a := New(WithClient(sdk))
			request := &Request{
				Model:    "fixture",
				Contents: genai.Text("fixture"),
				Config:   &genai.GenerateContentConfig{},
			}
			// Act.
			var usage prompty.Usage
			if native {
				var finalErr error
				for frame, streamErr := range a.StreamEvents(t.Context(), request) {
					if frame != nil {
						require.Empty(t, frame.Content)
						require.Empty(t, frame.Outcome)
						usage = frame.Usage
					}
					if streamErr != nil {
						finalErr = streamErr
					}
				}
				require.ErrorIs(t, finalErr, adapter.ErrMalformedArgs)
			} else {
				raw, callErr := a.Execute(t.Context(), request)
				require.NoError(t, callErr)
				response, parseErr := a.ParseResponse(raw)
				require.ErrorIs(t, parseErr, adapter.ErrMalformedArgs)
				require.NotNil(t, response)
				require.Empty(t, response.Content)
				require.Empty(t, response.Outcome)
				usage = response.Usage
			}
			// Assert: known counter survives failed materialization.
			require.Equal(t, 7, usage.PromptTokens)
			require.Contains(t, usage.Known, prompty.UsagePrompt)
		})
	}
}

func TestSDKPartialNumericFailureRetainsUsage(t *testing.T) {
	t.Parallel()
	// Arrange: partial numeric fields arrive already rounded in the SDK DTO.
	raw := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{
			{
				Content: &genai.Content{
					Parts: []*genai.Part{
						{
							FunctionCall: &genai.FunctionCall{
								PartialArgs: []*genai.PartialArg{{NumberValue: new(float64(1))}},
							},
						},
					},
				},
			},
		},
		UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 7},
	}
	payload, err := json.Marshal(raw)
	require.NoError(t, err)
	// Act.
	frame, err := (&generateStreamState{}).frameSDK(raw, payload)
	// Assert.
	require.ErrorIs(t, err, adapter.ErrMalformedArgs)
	require.NotNil(t, frame)
	require.Empty(t, frame.Content)
	require.Empty(t, frame.Outcome)
	require.Equal(t, 7, frame.Usage.PromptTokens)
	require.True(t, frame.ContinuationUnavailable)
}
