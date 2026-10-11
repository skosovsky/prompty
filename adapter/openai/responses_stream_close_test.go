package openai

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type sseObservedBody struct {
	io.ReadCloser

	closed chan struct{}
	once   sync.Once
}

func (b *sseObservedBody) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() { close(b.closed) })
	return err
}

type sseBodyTransport struct {
	base        http.RoundTripper
	calls       atomic.Int32
	firstClosed chan struct{}
}

func (t *sseBodyTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	call := t.calls.Add(1)
	response, err := t.base.RoundTrip(request)
	if err == nil && response != nil && call == 1 {
		response.Body = &sseObservedBody{ReadCloser: response.Body, closed: t.firstClosed}
	}
	return response, err
}

func awaitSSESignal(ctx context.Context, t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
}

func consumeUntilSSEFinalization(stream prompty.RemoteResponseStream, ready, done chan struct{}) {
	defer close(done)
	var once sync.Once
	for frame := range stream.Events() {
		if frame != nil && len(frame.Finalizations) != 0 {
			once.Do(func() { close(ready) })
		}
	}
}

func TestResponsesCreateStreamCloseReleasesBodyAndIsolatesConcurrentStream(t *testing.T) {
	t.Parallel()
	// Arrange: both actual SDK calls remain blocked after exposing encrypted state.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	releaseSecond := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		events := mixedResponseSSEFixture(true)
		const prefix = 7
		writeResponseSSEFixture(w, events[:prefix])
		w.(http.Flusher).Flush()
		if !strings.Contains(string(body), "second") {
			<-request.Context().Done()
			return
		}
		select {
		case <-releaseSecond:
			writeResponseSSEFixtureFrom(w, events[prefix:], prefix)
		case <-request.Context().Done():
		}
	}))
	defer server.Close()
	transport := &sseBodyTransport{base: server.Client().Transport, firstClosed: make(chan struct{})}
	server.Client().Transport = transport
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
	create := func(prompt string) prompty.RemoteResponseStream {
		exec := prompty.SimplePrompt(prompt)
		exec.Tools = responseToolDefinitions()
		return manager.CreateStream(ctx, prompty.RemoteResponseRequest{Execution: withResponseHostedTools(exec,
			[]prompty.HostedToolRequest{{Kind: HostedWebSearch}}), Store: new(false)})
	}
	first := create("first")
	firstReady, firstDone := make(chan struct{}), make(chan struct{})
	go consumeUntilSSEFinalization(first, firstReady, firstDone)
	awaitSSESignal(ctx, t, firstReady)
	second := create("second")
	secondReady, secondDone := make(chan struct{}), make(chan struct{})
	go consumeUntilSSEFinalization(second, secondReady, secondDone)
	awaitSSESignal(ctx, t, secondReady)
	// Act: close one active transport while the second remains blocked.
	require.NoError(t, first.Close())
	awaitSSESignal(ctx, t, firstDone)
	awaitSSESignal(ctx, t, transport.firstClosed)
	close(releaseSecond)
	awaitSSESignal(ctx, t, secondDone)
	// Assert: local cancellation retains the first resource/state and leaves the second successful.
	require.NoError(t, ctx.Err())
	require.Equal(t, prompty.StreamCanceled, first.Status().State)
	require.Equal(t, prompty.RemoteResponseRunning, first.RemoteResult().Handle.Status)
	require.Equal(t, "resp_fixture", first.RemoteResult().Handle.ProviderID)
	require.Len(t, first.RemoteResult().Response.ProviderState, 1)
	require.Equal(t, prompty.StreamCompleted, second.Status().State)
	require.Equal(t, prompty.RemoteResponseCompleted, second.RemoteResult().Handle.Status)
	require.EqualValues(t, 2, transport.calls.Load())
}
