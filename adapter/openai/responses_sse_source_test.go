package openai

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type guardedSSEBody struct {
	reader io.Reader
	reads  int
	closed bool
}

func (b *guardedSSEBody) Read(p []byte) (int, error) { b.reads++; return b.reader.Read(p) }
func (b *guardedSSEBody) Close() error               { b.closed = true; return nil }

func TestResponseSSESourceDoesNotReadOrCloseBody(t *testing.T) {
	t.Parallel()
	// Arrange: an SSE body remains live until the decoder consumes it.
	body := &guardedSSEBody{reader: strings.NewReader("data: fixture\n\n")}
	response := &http.Response{StatusCode: http.StatusOK, Body: body}
	request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, "https://example.test/v1/responses", nil)
	require.NoError(t, err)
	source := new(responseSSESource)
	// Act.
	observed, err := source.observe(request, func(*http.Request) (*http.Response, error) { return response, nil })
	// Assert: capture middleware must preserve streaming ownership and latency.
	require.NoError(t, err)
	require.Same(t, response, observed)
	require.Same(t, body, observed.Body)
	require.Zero(t, body.reads)
	require.False(t, body.closed)
	require.Equal(t, http.StatusOK, source.status)
	data, err := io.ReadAll(observed.Body)
	require.NoError(t, err)
	require.Equal(t, "data: fixture\n\n", string(data))
	require.NoError(t, observed.Body.Close())
	require.True(t, body.closed)
}

func TestResponseSSESourcePreservesTransportError(t *testing.T) {
	t.Parallel()
	// Arrange.
	source := new(responseSSESource)
	transportErr := errors.New("transport")
	// Act.
	response, err := source.observe(nil, func(*http.Request) (*http.Response, error) { return nil, transportErr })
	// Assert.
	require.Nil(t, response)
	require.ErrorIs(t, err, transportErr)
	require.Zero(t, source.status)
}
