// Package streamtest provides local protocol harnesses for adapter conformance
// tests. It does not implement a provider, runtime, retry loop or live probe.
package streamtest

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
	"github.com/skosovsky/prompty/adapter"
)

type isolationResult struct {
	label     string
	status    prompty.StreamStatus
	terminals int
}

const (
	leftLabel        = "left"
	rightLabel       = "right"
	isolationTimeout = 5 * time.Second
)

// CancellationIsolation overlaps two streams from one client with equal native
// call IDs/indices and distinct arguments. Canceling one must not stop the other.
// emit publishes early text/tools, then terminal protocol/late usage when final.
func CancellationIsolation(t *testing.T, build func(*httptest.Server) prompty.Invoker,
	emit func(http.ResponseWriter, string, bool), total int,
) {
	t.Helper()
	// Arrange: both sources stay live after publishing their distinct calls.
	ctx, cancelAll := context.WithTimeout(t.Context(), isolationTimeout)
	defer cancelAll()
	release := make(chan struct{})
	releaseRight := sync.OnceFunc(func() { close(release) })
	closed := make(chan string, 2)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		label := rightLabel
		if strings.Contains(string(body), `"left"`) {
			label = leftLabel
		}
		defer func() { closed <- label }()
		emit(w, label, false)
		if err := http.NewResponseController(w).Flush(); err != nil {
			return
		}
		select {
		case <-r.Context().Done():
			return
		case <-release:
			emit(w, label, true)
		}
	}))
	defer func() { cancelAll(); releaseRight(); server.Close() }()
	client := build(server)
	assertNativeIntentGates(ctx, t, client, &calls)
	ready := make(chan string, 2)
	finished := make(chan isolationResult, 2)
	leftCtx, cancelLeft := context.WithCancel(ctx)
	defer cancelLeft()
	for _, label := range []string{leftLabel, rightLabel} {
		callCtx := ctx
		if label == leftLabel {
			callCtx = leftCtx
		}
		go consumeIsolationStream(callCtx, client, label, ready, finished)
	}
	// Act: prove both have published before canceling just one source.
	seen := make(map[string]bool)
	for range 2 {
		select {
		case label := <-ready:
			seen[label] = true
		case <-ctx.Done():
			t.Fatal("both native calls were not published before completion")
		}
	}
	require.Len(t, seen, 2)
	cancelLeft()
	var left, right isolationResult
	select {
	case left = <-finished:
	case <-ctx.Done():
		t.Fatal("cancel did not release the left source")
	}
	require.Equal(t, leftLabel, left.label)
	select {
	case label := <-closed:
		require.Equal(t, leftLabel, label)
	case <-ctx.Done():
		t.Fatal("canceled native HTTP request did not close")
	}
	releaseRight()
	select {
	case right = <-finished:
	case <-ctx.Done():
		t.Fatal("unrelated right source did not finish")
	}
	// Assert: one request/terminal per stream; no shared identities or replay.
	require.Equal(t, rightLabel, right.label)
	require.EqualValues(t, 2, calls.Load())
	assertIsolationResults(t, left, right, total)
	require.NoError(t, ctx.Err())
}

func consumeIsolationStream(ctx context.Context, client prompty.Invoker, label string,
	ready chan<- string, finished chan<- isolationResult,
) {
	stream := client.ExecuteStream(ctx, prompty.SimplePrompt(label), prompty.StreamNative)
	published, terminals := false, 0
	for frame := range stream.Events() {
		if frame == nil {
			continue
		}
		if frame.Kind == prompty.StreamTerminal {
			terminals++
		}
		for _, part := range frame.Content {
			call, ok := part.(prompty.ToolCallPart)
			if ok && !published && strings.Contains(call.Args+call.ArgsChunk, label) {
				published = true
				ready <- label
			}
		}
	}
	finished <- isolationResult{label: label, status: stream.Status(), terminals: terminals}
}

func assertIsolationResults(t *testing.T, left, right isolationResult, total int) {
	t.Helper()
	require.Equal(t, 1, left.terminals)
	require.Equal(t, 1, right.terminals)
	require.Equal(t, prompty.StreamCanceled, left.status.State)
	require.ErrorIs(t, left.status.Err, context.Canceled)
	// No provider finish was observed; cancellation must not fabricate one.
	require.Empty(t, left.status.Result.Outcome)
	require.NoError(t, right.status.Err)
	require.Equal(t, prompty.StreamCompleted, right.status.State)
	require.Equal(t, prompty.OutcomeToolCalls, right.status.Result.Outcome)
	require.Equal(t, total, right.status.Result.Usage.TotalTokens)
	require.Contains(t, right.status.Result.Usage.Known, prompty.UsageTotal)
	require.NotEqual(t, left.status.Identity.MessageID, right.status.Identity.MessageID)
	for _, result := range []isolationResult{left, right} {
		var text strings.Builder
		tools := 0
		for _, part := range result.status.Result.Content {
			switch value := part.(type) {
			case prompty.TextPart:
				text.WriteString(value.Text)
			case prompty.ToolCallPart:
				tools++
				require.Equal(t, "shared-call", value.ID)
				if value.Args+value.ArgsChunk != "" {
					opposite := map[string]string{leftLabel: rightLabel, rightLabel: leftLabel}[result.label]
					require.NotContains(t, value.Args+value.ArgsChunk, opposite)
				}
			}
		}
		require.Equal(t, result.label, text.String())
		require.Positive(t, tools)
		if result.label == rightLabel {
			require.Equal(t, 1, tools)
			for _, part := range result.status.Result.Content {
				if call, ok := part.(prompty.ToolCallPart); ok {
					require.JSONEq(t, `{"tag":"right"}`, call.Args)
				}
			}
		}
	}
}

func assertNativeIntentGates(ctx context.Context, t *testing.T, client prompty.Invoker, calls *atomic.Int32) {
	t.Helper()
	// Native transport availability does not invent model/tool capability knowledge.
	unknownModel := prompty.SimplePrompt("unsupported")
	unknownModel.ModelOptions = &prompty.ModelOptions{Model: "unknown-fixture"}
	unknownTools := prompty.SimplePrompt("unsupported")
	unknownTools.Tools = []prompty.ToolDefinition{{
		Name:         "lookup",
		Description:  "",
		Parameters:   []byte(`{"type":"object"}`),
		Dialect:      "",
		Capabilities: nil,
	}}
	for _, exec := range []*prompty.PromptExecution{unknownModel, unknownTools} {
		stream := client.ExecuteStream(ctx, exec, prompty.StreamNative)
		for range stream.Events() {
		}
		require.Equal(t, prompty.StreamFailed, stream.Status().State)
		require.ErrorIs(t, stream.Status().Err, adapter.ErrUnknownProfile)
		require.Zero(t, calls.Load())
	}
}
