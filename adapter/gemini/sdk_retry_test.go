package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestSDKOperationRetryOverrideDoesNotMutateHost(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "native"}[native], func(t *testing.T) {
			t.Parallel()
			// Arrange: the host explicitly requests retries; the adapter uses public per-call override.
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				attempts.Add(1)
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error":{"code":503,"message":"private"}}`))
			}))
			defer server.Close()
			hostAttempts := int32(3)
			sdk, err := genai.NewClient(
				t.Context(),
				&genai.ClientConfig{
					APIKey:     "fixture",
					Backend:    genai.BackendGeminiAPI,
					HTTPClient: server.Client(),
					HTTPOptions: genai.HTTPOptions{
						BaseURL:      server.URL,
						APIVersion:   "v1beta",
						RetryOptions: &genai.HTTPRetryOptions{Attempts: &hostAttempts},
					},
				},
			)
			require.NoError(t, err)
			originalOptions := &genai.HTTPOptions{
				RetryOptions: &genai.HTTPRetryOptions{Attempts: &hostAttempts},
				Headers:      http.Header{"X-Host": []string{"kept"}},
			}
			request := &Request{
				Model:    "fixture",
				Contents: genai.Text("fixture"),
				Config:   &genai.GenerateContentConfig{HTTPOptions: originalOptions},
			}
			a := New(WithClient(sdk), WithModel("fixture"))
			// Act.
			if native {
				for _, streamErr := range a.StreamEvents(context.Background(), request) {
					require.Error(t, streamErr)
				}
			} else {
				_, err = a.Execute(t.Context(), request)
				require.Error(t, err)
			}
			// Assert: actual network evidence and host configuration ownership, not inference from options.
			require.Equal(t, int32(1), attempts.Load())
			require.Equal(t, int32(3), *request.Config.HTTPOptions.RetryOptions.Attempts)
			require.Equal(t, "kept", originalOptions.Headers.Get("X-Host"))
			require.Equal(t, int32(3), *sdk.ClientConfig().HTTPOptions.RetryOptions.Attempts)
		})
	}
}
