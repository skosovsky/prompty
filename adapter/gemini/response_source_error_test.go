package gemini

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestObservedMalformedResponseNeverEchoesProviderBody(t *testing.T) {
	t.Parallel()
	for index, body := range []string{
		`{"candidates":[{"content":{"parts":[{"text":"private-response","thoughtSignature":"cHJpdmF0ZS1zaWduYXR1cmU="},]},"finishReason":"STOP"}]}`,
		`{"private-response":true,}`,
		`{"private-response":1e1000}`,
	} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered} {
			for _, limit := range []int{defaultResponsePayloadLimit, 8} {
				t.Run(strconv.Itoa(index)+"/"+string(mode)+"/"+strconv.Itoa(limit), func(t *testing.T) {
					t.Parallel()
					// Arrange: full HTTP 200 body, with malformed or unconvertible private data.
					var calls atomic.Int64
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						calls.Add(1)
						w.Header().Set("Content-Type", "application/json")
						_, _ = io.WriteString(w, body)
					}))
					t.Cleanup(server.Close)
					client := boundedResponseFixtureClient(t, server, limit)
					// Act.
					_, err := checkedIngressInvoke(context.Background(), client, prompty.SimplePrompt("fixture"), mode)
					// Assert: the public error chain has no raw SDK cause to unwrap/log.
					require.ErrorIs(t, err, prompty.ErrProviderState)
					require.NotContains(t, err.Error(), "private-response")
					require.NotContains(t, err.Error(), "cHJpdmF0ZS1zaWduYXR1cmU=")
					require.Equal(t, prompty.ErrProviderState.Error(), err.Error())
					require.Equal(t, int64(1), calls.Load())
				})
			}
		}
	}
}
