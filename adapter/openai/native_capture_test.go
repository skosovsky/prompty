package openai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const nativeCaptureDelta = `{"id":"fixture","choices":[{"index":0,"delta":{"content":"{}"}}],` +
	`"provider_extension":{"retained":true}}`

func nativeCaptureFixture(
	t *testing.T, calls *atomic.Int32, requestBody chan<- []byte,
) prompty.Invoker {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failed", http.StatusBadRequest)
			return
		}
		if requestBody != nil {
			requestBody <- body
		}
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			nativeCaptureDelta,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"total_tokens":9}}`,
			"[DONE]",
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	t.Cleanup(server.Close)
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("private-credential"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK/SSE fixture, not live capability evidence",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
		},
	}
	return adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
}

func TestNativeCaptureReadsProviderChunksOnceBeforeNormalization(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	requests := make(chan []byte, 1)
	client := nativeCaptureFixture(t, &calls, requests)
	var records []prompty.CaptureRecord
	exec := prompty.SimplePrompt("fixture")
	exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{},
		Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
			records = append(records, record)
			return nil
		}}}
	exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic,
		Stages: []prompty.CaptureStage{
			prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderChunk,
			prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
		}}
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
	require.Zero(t, calls.Load())
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 1, terminals)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.True(t, status.Result.Diagnostics.Capture.Complete)
	require.True(t, status.Result.Diagnostics.Capture.Closed)
	require.Len(t, records, 9) // two pre-dispatch + three raw/canonical pairs + terminal
	require.JSONEq(t, nativeCaptureDelta, string(records[2].Projection.Payload))
	require.Equal(t, prompty.CaptureProviderChunk, records[2].Stage)
	require.Equal(t, prompty.CaptureCanonicalParts, records[3].Stage)
	require.Equal(t, "fixture", records[2].Projection.Metadata.ProviderResponseID)
	require.Empty(t, records[2].Projection.Metadata.RequestID)
	require.Nil(t, records[2].Projection.Metadata.HTTPStatus)
	require.Empty(t, records[2].Projection.Metadata.Usage.Known)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, records[6].Projection.Metadata.Usage.Known)
	require.Equal(t, 9, records[6].Projection.Metadata.Usage.TotalTokens)
	require.Equal(t, status.Result.Usage, *records[8].Projection.Metadata.Usage)
	require.Equal(t, status.Result.Outcome, records[8].Projection.Metadata.Outcome)
	var request, translated, terminal map[string]any
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.NoError(t, json.Unmarshal(records[1].Projection.Payload, &translated))
	require.Equal(t, map[string]any{"include_usage": true}, translated["stream_options"])
	require.Equal(t, request["stream_options"], translated["stream_options"])
	require.NoError(t, json.Unmarshal(records[8].Projection.Payload, &terminal))
	require.InDelta(t, 9, terminal["response"].(map[string]any)["usage"].(map[string]any)["total_tokens"], 0)
	for _, index := range []int{3, 5, 7, 8} {
		var envelope struct {
			Identity prompty.StreamIdentity `json:"stream_identity"`
		}
		require.NoError(t, json.Unmarshal(records[index].Projection.Payload, &envelope))
		require.Equal(t, status.Identity.MessageID, envelope.Identity.MessageID)
		if index == 3 || index == 8 {
			require.Equal(t, []string{"content:0"}, envelope.Identity.PartIDs)
		} else {
			require.Empty(t, envelope.Identity.PartIDs)
		}
	}
	for index, record := range records {
		require.EqualValues(t, index+1, record.Sequence)
		require.Equal(t, status.Result.Diagnostics.Capture.InvocationID, record.InvocationID)
		require.NotContains(t, string(record.Projection.Payload), "private-credential")
	}
}

func TestNativeToolCaptureReferencesSurvivePrefixAndTerminalAssembly(t *testing.T) {
	t.Parallel()
	// Arrange: canonical prefixes contain incomplete ArgsChunk, while the terminal contains the assembled object.
	sse := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"fn","arguments":"{"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"}"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n" +
		"data: " + `{"choices":[],"usage":{"total_tokens":5}}` + "\n\n"
	client, transport := cleanupFixtureClient(strings.NewReader(sse))
	var records []prompty.CaptureRecord
	exec := prompty.SimplePrompt("fixture")
	exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{},
		Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
			records = append(records, record)
			return nil
		}}}
	exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic,
		Stages: []prompty.CaptureStage{prompty.CaptureCanonicalParts, prompty.CaptureTerminal}}
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	// Assert: canonical frame capture is prefix-safe, and final refs describe materialized calls.
	status := stream.Status()
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, []string{"tool:0"}, status.Identity.PartIDs)
	require.Len(t, status.Result.Content, 1)
	require.Equal(t, "{}", status.Result.Content[0].(prompty.ToolCallPart).Args)
	require.True(t, status.Result.Diagnostics.Capture.Complete)
	require.Equal(t, int32(1), transport.calls.Load())
	contentFrames, terminals := 0, 0
	for _, record := range records {
		if record.Stage != prompty.CaptureCanonicalParts && record.Stage != prompty.CaptureTerminal {
			continue
		}
		var envelope struct {
			Identity prompty.StreamIdentity `json:"stream_identity"`
		}
		require.NoError(t, json.Unmarshal(record.Projection.Payload, &envelope))
		require.Equal(t, status.Identity.MessageID, envelope.Identity.MessageID)
		if record.Stage == prompty.CaptureTerminal {
			terminals++
			require.Equal(t, status.Identity, envelope.Identity)
		} else if len(envelope.Identity.PartIDs) != 0 {
			contentFrames++
			require.Equal(t, []string{"tool:0"}, envelope.Identity.PartIDs)
		}
	}
	require.Equal(t, 2, contentFrames)
	require.Equal(t, 1, terminals)
}

func TestNativeCaptureSinkFailureRetainsParsedChunkWithoutReplay(t *testing.T) {
	t.Parallel()
	for _, stage := range []prompty.CaptureStage{
		prompty.CaptureExecution, prompty.CaptureProviderChunk, prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
	} {
		t.Run(string(stage), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			client := nativeCaptureFixture(t, &calls, nil)
			sentinel := errors.New("sink unavailable")
			exec := prompty.SimplePrompt("fixture")
			exec.Requirements.Capture.Stages = []prompty.CaptureStage{stage}
			sink := captureFixtureSink{write: func(record prompty.CaptureRecord) error {
				if record.Stage == stage {
					return sentinel
				}
				return nil
			}}
			exec.Capture = &prompty.CaptureBinding{Sink: sink}
			stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
			// Act.
			terminals := 0
			for event := range stream.Events() {
				if event.Kind == prompty.StreamTerminal {
					terminals++
				}
			}
			// Assert.
			status := stream.Status()
			require.Equal(t, prompty.StreamFailed, status.State)
			require.ErrorIs(t, status.Err, sentinel)
			require.Equal(t, 1, terminals)
			var failure *prompty.CaptureError
			require.ErrorAs(t, status.Err, &failure)
			if stage == prompty.CaptureExecution {
				require.False(t, failure.AfterDispatch)
				require.Zero(t, calls.Load())
			} else {
				require.True(t, failure.AfterDispatch)
				require.EqualValues(t, 1, calls.Load())
				require.NotEmpty(t, status.Result.Content)
			}
			if stage == prompty.CaptureTerminal {
				require.Equal(t, 9, status.Result.Usage.TotalTokens)
			}
			require.False(t, status.Result.Diagnostics.Capture.Complete)
		})
	}
}

func TestNativeTerminalCaptureSeesOriginalValidationFailure(t *testing.T) {
	t.Parallel()
	// Arrange: source output {} violates original required x despite valid JSON.
	var calls atomic.Int32
	client := nativeCaptureFixture(t, &calls, nil)
	var terminal prompty.CaptureRecord
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"x":{"type":"integer"}},"required":["x"],"additionalProperties":false}`,
	)}
	exec.Requirements.Capture.Stages = []prompty.CaptureStage{prompty.CaptureTerminal}
	exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{},
		Sink: captureFixtureSink{write: func(record prompty.CaptureRecord) error {
			if record.Stage == prompty.CaptureTerminal {
				terminal = record
			}
			return nil
		}}}
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
	// Act.
	for event, streamErr := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			require.ErrorIs(t, streamErr, prompty.ErrSchemaMismatch)
		}
	}
	// Assert.
	status := stream.Status()
	require.ErrorIs(t, status.Err, prompty.ErrSchemaMismatch)
	require.EqualValues(t, 1, calls.Load())
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	var value map[string]any
	require.NoError(t, json.Unmarshal(terminal.Projection.Payload, &value))
	require.Equal(t, "schema_validation", value["error_kind"])
	require.Equal(t, "completed", value["response"].(map[string]any)["outcome"])
	require.True(t, status.Result.Diagnostics.Capture.Complete)
}
