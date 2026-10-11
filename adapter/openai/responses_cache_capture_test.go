package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesCapturePreservesOriginalAdvisoryCacheIntent(t *testing.T) {
	t.Parallel()
	for _, requestPolicy := range []bool{false, true} {
		t.Run(map[bool]string{false: "block", true: "request"}[requestPolicy], func(t *testing.T) {
			t.Parallel()
			// Arrange: original intent belongs to canonical capture, not the SDK request.
			var calls atomic.Int32
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				data, _ := io.ReadAll(request.Body)
				requests <- data
				writeResponseFixture(w, "completed", "answer")
			}))
			defer server.Close()
			manager := responseFixtureAdapter(server)
			exec := prompty.SimplePrompt("document")
			policy := &prompty.CachePolicy{
				Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheAdvisory,
				Provider: "openai.responses", Controls: prompty.JSONDocument(`{"unmapped":"original intent"}`),
			}
			if requestPolicy {
				policy.Mechanism = prompty.CacheImplicitPrefix
				exec.RequestCache = policy
			} else {
				exec.Messages[0].CachePolicy = policy
			}
			var canonical, translated []byte
			exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{}, Sink: captureFixtureSink{
				write: func(record prompty.CaptureRecord) error {
					switch record.Stage {
					case prompty.CaptureExecution:
						canonical = append([]byte(nil), record.Projection.Payload...)
						require.True(t, record.Projection.Complete)
					case prompty.CaptureTranslatedRequest:
						translated = append([]byte(nil), record.Projection.Payload...)
					case prompty.CaptureTransportRequest,
						prompty.CaptureTransportResponse,
						prompty.CaptureProviderResponse,
						prompty.CaptureProviderChunk,
						prompty.CaptureCanonicalParts,
						prompty.CaptureTerminal:
						fallthrough
					default:
						// Other stages belong to separate lifecycle fixtures.
					}
					return nil
				},
			}}
			exec.Requirements.Capture = prompty.CaptureRequirement{
				Accuracy: prompty.CaptureSemantic,
				Stages:   []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest},
			}
			// Act.
			result, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
			// Assert: both source and captured canonical operation retain exact policy.
			require.NoError(t, err)
			require.EqualValues(t, 1, calls.Load())
			var captured struct {
				Execution *struct {
					RequestCache *prompty.CachePolicy `json:"request_cache"`
					Messages     []struct {
						CachePolicy *prompty.CachePolicy `json:"cache_policy"`
					} `json:"Messages"`
				} `json:"Execution"`
				Store *bool `json:"Store"`
			}
			require.NoError(t, json.Unmarshal(canonical, &captured))
			require.NotNil(t, captured.Execution)
			require.NotNil(t, captured.Store)
			require.True(t, *captured.Store)
			if requestPolicy {
				require.Equal(t, policy, captured.Execution.RequestCache)
				require.Equal(t, policy, exec.RequestCache)
			} else {
				require.Equal(t, policy, captured.Execution.Messages[0].CachePolicy)
				require.Equal(t, policy, exec.Messages[0].CachePolicy)
			}
			require.Len(t, result.Diagnostics.Cache, 1)
			require.Equal(t, prompty.CacheUnsupported, result.Diagnostics.Cache[0].Disposition)
			require.True(t, result.Diagnostics.Capture.Complete)
			for _, data := range [][]byte{translated, <-requests} {
				require.NotContains(t, string(data), "original intent")
				require.NotContains(t, string(data), "unmapped")
				require.NotContains(t, string(data), "cache_control")
			}
		})
	}
}

func TestResponsesToolResultAdvisoryCacheOmission(t *testing.T) {
	t.Parallel()
	for _, location := range []string{"message", "result", "text", "media"} {
		t.Run(location, func(t *testing.T) {
			t.Parallel()
			// Arrange: host-owned result of a known call, not signed history.
			var calls atomic.Int32
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				data, _ := io.ReadAll(request.Body)
				requests <- data
				writeResponseFixture(w, "completed", "answer")
			}))
			defer server.Close()
			manager := localToolsFixtureAdapter(server)
			manager.base.profiles[0].InputMIME = []string{"image/png"}
			previous := remoteFixtureHandle(manager)
			previous.Status = prompty.RemoteResponseCompleted
			previous.ToolSchemaDigests = map[string]string{"lookup": strings.Repeat("a", 64)}
			previous.ToolCalls = map[string]string{"call_fixture": "lookup"}
			policy := &prompty.CachePolicy{
				Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheAdvisory,
				Provider: "openai.responses", Controls: prompty.JSONDocument(`{"unmapped":"original intent"}`),
			}
			text := prompty.TextPart{Text: "result"}
			media := prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1, 2, 3}}
			result := prompty.ToolResultPart{ToolCallID: "call_fixture", Name: "lookup"}
			message := prompty.ChatMessage{Role: prompty.RoleTool}
			switch location {
			case "message":
				message.CachePolicy = policy
			case "result":
				result.CachePolicy = policy
			case "text":
				text.CachePolicy = policy
			case "media":
				media.CachePolicy = policy
			}
			result.Content = []prompty.ContentPart{text, &media}
			message.Content = []prompty.ContentPart{&result}
			exec := prompty.NewExecution([]prompty.ChatMessage{message})
			var canonical []byte
			exec.Capture = &prompty.CaptureBinding{Projector: captureFixtureProjector{}, Sink: captureFixtureSink{
				write: func(record prompty.CaptureRecord) error {
					if record.Stage == prompty.CaptureExecution {
						canonical = append([]byte(nil), record.Projection.Payload...)
					}
					return nil
				},
			}}
			exec.Requirements.Capture = prompty.CaptureRequirement{
				Accuracy: prompty.CaptureSemantic, Stages: []prompty.CaptureStage{prompty.CaptureExecution},
			}
			// Act.
			observed, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
				Execution: exec, Previous: &previous, Store: new(true),
			})
			// Assert: omission keeps original intent, correlation and nested order.
			require.NoError(t, err)
			require.EqualValues(t, 1, calls.Load())
			require.Contains(t, string(canonical), "original intent")
			require.Equal(t, prompty.CacheBlockMarker, policy.Mechanism)
			require.Contains(t, string(policy.Controls), "original intent")
			require.Len(t, observed.Diagnostics.Cache, 1)
			require.Equal(t, prompty.CacheUnsupported, observed.Diagnostics.Cache[0].Disposition)
			request := string(<-requests)
			require.Contains(t, request, `"call_id":"call_fixture"`)
			require.Contains(t, request, `"text":"result"`)
			require.Contains(t, request, "data:image/png;base64,AQID")
			require.NotContains(t, request, "unmapped")
			require.NotContains(t, request, "cache_control")
			require.NotContains(t, request, "original intent")
		})
	}
}
