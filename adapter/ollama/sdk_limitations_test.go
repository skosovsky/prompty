package ollama

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/adapter"
)

// SDK-OLLAMA-FRAME: official framing policy, without a custom parser/limit fallback.
func TestSDKFrameLimitAndPrivateDecodeErrorCharacterization(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct{ name, body, sdkError string }{
		{"oversized", `{"message":{"content":"` + strings.Repeat("x", 8<<20) + `"},"done":true}` + "\n", "token too long"},
		{"malformed", "private-provider-body\n", "private-provider-body"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: genuine SDK acquisition with an SDK-owned scanner/decoder.
			var calls atomic.Int32
			server := httptest.NewServer(
				http.HandlerFunc(
					func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); _, _ = w.Write([]byte(fixture.body)) },
				),
			)
			defer server.Close()
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			sdk := api.NewClient(endpoint, server.Client())
			streaming := false
			request := &api.ChatRequest{Model: "fixture", Stream: &streaming}
			// Act: reproduce dependency behavior, then verify the adapter's safe projection.
			sdkErr := sdk.Chat(t.Context(), request, func(api.ChatResponse) error { return nil })
			response, adapterErr := New(WithClient(sdk)).Execute(t.Context(), request)
			// Assert: characterization is no upstream fix; adapter neither retries nor leaks it.
			require.ErrorContains(t, sdkErr, fixture.sdkError)
			require.ErrorIs(t, adapterErr, adapter.ErrInvalidResponse)
			require.NotContains(t, adapterErr.Error(), "private-provider-body")
			require.Nil(t, response)
			require.Equal(t, int32(2), calls.Load())
		})
	}
}
