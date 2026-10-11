package openai

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"sync"
	"testing"

	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func streamResponseFixture(t *testing.T, source string) *responses.Response {
	t.Helper()
	var raw responses.Response
	require.NoError(t, json.Unmarshal([]byte(source), &raw))
	return &raw
}

func streamHandleFixture() *responseStreamHandle {
	return &responseStreamHandle{handle: prompty.RemoteResponseHandle{
		Format: prompty.RemoteResponseWireFormat,
		Identity: prompty.ProfileIdentity{
			Provider: "openai.responses",
			Model:    "fixture",
			Endpoint: "https://example.test/v1",
		},
		AccountBinding: "account", Status: prompty.RemoteResponseUnknown,
		HostedTools: []string{HostedWebSearch}, ToolSchemaDigests: map[string]string{"lookup": "digest"},
	}}
}

func TestResponseStreamHandleRetainsRemoteIdentityAndPartial(t *testing.T) {
	t.Parallel()
	// Arrange: the resource is observed before a partial delta and a transport failure.
	handle := streamHandleFixture()
	raw := streamResponseFixture(t, `{"id":"resp_fixture","model":"fixture","status":"in_progress"}`)
	transportErr := errors.New("transport failed")
	handle.stream = prompty.NewStream(t.Context(), prompty.StreamNative,
		func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				if err := handle.observeResponse(raw); err != nil {
					yield(nil, err)
					return
				}
				if !yield(&prompty.ResponseChunk{Kind: prompty.StreamStart}, nil) {
					return
				}
				if !yield(&prompty.ResponseChunk{Kind: prompty.StreamDelta,
					Identity: prompty.StreamIdentity{PartIDs: []string{"text"}},
					Content:  []prompty.ContentPart{prompty.TextPart{Text: "partial"}}}, nil) {
					return
				}
				yield(nil, transportErr)
			}
		}, prompty.WithStreamCaptureObserver(func(context.Context, prompty.StreamStatus, error) (*prompty.CaptureReport, error) {
			return &prompty.CaptureReport{
				InvocationID: "capture",
				Coverage: []prompty.CaptureStageCoverage{
					{Stage: prompty.CaptureTerminal, Accuracy: prompty.CaptureSemantic},
				},
				Observed: []prompty.CaptureStage{prompty.CaptureTerminal},
				Closed:   true,
			}, nil
		}))
	// Act.
	terminals := 0
	for chunk, err := range handle.Events() {
		if chunk != nil && chunk.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, transportErr)
		}
	}
	result := handle.RemoteResult()
	// Assert: local failure does not invent a remote failed/canceled status.
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.StreamFailed, handle.Status().State)
	require.Equal(t, "resp_fixture", result.Handle.ProviderID)
	require.Equal(t, prompty.RemoteResponseRunning, result.Handle.Status)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "partial"}}, result.Response.Content)
	require.NotNil(t, result.Diagnostics.Capture)
	require.NotNil(t, result.Response.Diagnostics.Capture)
	require.Equal(t, "capture", result.Diagnostics.Capture.InvocationID)
	require.Equal(t, "capture", result.Response.Diagnostics.Capture.InvocationID)
	result.Diagnostics.Capture.Observed[0] = prompty.CaptureExecution
	result.Response.Diagnostics.Capture.InvocationID = "mutated"
	result.Handle.HostedTools[0] = "mutated"
	result.Handle.ToolSchemaDigests["lookup"] = "mutated"
	result.Response.Content[0] = prompty.TextPart{Text: "mutated"}
	fresh := handle.RemoteResult()
	require.Equal(t, HostedWebSearch, fresh.Handle.HostedTools[0])
	require.Equal(t, "digest", fresh.Handle.ToolSchemaDigests["lookup"])
	require.Equal(t, prompty.TextPart{Text: "partial"}, fresh.Response.Content[0])
	require.Equal(t, []prompty.CaptureStage{prompty.CaptureTerminal}, fresh.Diagnostics.Capture.Observed)
	require.Equal(t, "capture", fresh.Response.Diagnostics.Capture.InvocationID)
}

func TestResponseStreamHandleRejectsContradictions(t *testing.T) {
	t.Parallel()
	for _, source := range []string{
		`{"id":"resp_other","model":"fixture","status":"completed"}`,
		`{"id":"resp_fixture","model":"other","status":"completed"}`,
		`{"id":"resp_fixture","model":"fixture","status":"future"}`,
		`{"id":"resp_fixture","model":"fixture","status":"queued"}`,
		`{"id":"resp_fixture","model":"fixture","status":"completed","status":"failed"}`,
	} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			handle := streamHandleFixture()
			require.NoError(t, handle.observeResponse(streamResponseFixture(t,
				`{"id":"resp_fixture","model":"fixture","status":"in_progress"}`)))
			// Act.
			err := handle.observeResponse(streamResponseFixture(t, source))
			// Assert: a contradictory event cannot replace the accepted binding/status.
			require.Error(t, err)
			require.Equal(t, "resp_fixture", handle.handle.ProviderID)
			require.Equal(t, prompty.RemoteResponseRunning, handle.handle.Status)
		})
	}
}

func TestResponseStreamHandleRetainsOnlySafeIdentityFromMalformedResponse(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := streamHandleFixture()
	handle.stream = prompty.ErrorStream(t.Context(), prompty.StreamNative, prompty.ErrStreamProtocol)
	// Act: a resource ID remains useful for explicit reconciliation despite invalid metadata.
	err := handle.observeResponse(streamResponseFixture(t, `{"id":"resp_fixture","status":"completed"}`))
	result := handle.RemoteResult()
	// Assert.
	require.ErrorIs(t, err, prompty.ErrRemoteResponseUnknown)
	require.Equal(t, "resp_fixture", result.Handle.ProviderID)
	require.Equal(t, prompty.RemoteResponseUnknown, result.Handle.Status)
}

func TestResponseStreamHandleConcurrentObservationAndSnapshots(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := streamHandleFixture()
	handle.stream = prompty.ErrorStream(t.Context(), prompty.StreamNative, prompty.ErrStreamProtocol)
	raw := streamResponseFixture(t, `{"id":"resp_fixture","model":"fixture","status":"in_progress"}`)
	var workers sync.WaitGroup
	// Act: independent readers may mutate their snapshots while metadata is updated.
	for range 8 {
		workers.Go(func() {
			for range 100 {
				if err := handle.observeResponse(raw); err != nil {
					t.Error(err)
					return
				}
				result := handle.RemoteResult()
				result.Handle.ToolSchemaDigests["lookup"] = "owned"
				result.Handle.HostedTools[0] = "owned"
			}
		})
	}
	workers.Wait()
	// Assert (also exercised under -race).
	require.Equal(t, "digest", handle.RemoteResult().Handle.ToolSchemaDigests["lookup"])
	require.Equal(t, HostedWebSearch, handle.RemoteResult().Handle.HostedTools[0])
}

func TestResponseStreamHandleCloseIsLocalAndLazy(t *testing.T) {
	t.Parallel()
	// Arrange: an unused stream must not dispatch, and a running one retains its resource.
	unused := streamHandleFixture()
	unused.stream = prompty.NewStream(t.Context(), prompty.StreamNative,
		func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			t.Error("unused stream dispatched")
			return func(func(*prompty.ResponseChunk, error) bool) {}
		})
	running := streamHandleFixture()
	raw := streamResponseFixture(t, `{"id":"resp_fixture","model":"fixture","status":"in_progress"}`)
	started := make(chan struct{})
	running.stream = prompty.NewStream(t.Context(), prompty.StreamNative,
		func(ctx context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				if err := running.observeResponse(raw); err != nil {
					yield(nil, err)
					return
				}
				if !yield(&prompty.ResponseChunk{Kind: prompty.StreamStart}, nil) {
					return
				}
				close(started)
				<-ctx.Done()
				yield(nil, ctx.Err())
			}
		})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for range running.Events() {
		}
	}()
	<-started
	// Act.
	require.NoError(t, unused.Close())
	require.NoError(t, running.Close())
	<-finished
	// Assert: no remote cancellation is implied by local cancellation.
	require.Nil(t, unused.RemoteResult().Handle)
	require.Equal(t, prompty.StreamCanceled, running.Status().State)
	require.Equal(t, prompty.RemoteResponseRunning, running.RemoteResult().Handle.Status)
	require.Equal(t, "resp_fixture", running.RemoteResult().Handle.ProviderID)
}

func TestResponseStreamHandleDoesNotGuessAmbiguousIdentity(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := streamHandleFixture()
	// Act: duplicate resource IDs cannot authorize persistence of either value.
	err := handle.observeResponse(streamResponseFixture(t,
		`{"id":"resp_first","id":"resp_second","model":"fixture","status":"in_progress"}`))
	// Assert.
	require.ErrorIs(t, err, prompty.ErrRemoteResponseUnknown)
	require.False(t, handle.known)
	require.Empty(t, handle.handle.ProviderID)
}
