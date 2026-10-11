package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type task45StructuredResult struct {
	Answer string `json:"answer"`
}
type task45ObservedInvoker struct {
	prompty.Invoker

	stream    *prompty.Stream
	closeouts int
}

func (i *task45ObservedInvoker) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	i.stream = i.Invoker.ExecuteStream(ctx, exec, mode)
	_ = i.stream.ObserveLifecycle(func(ctx context.Context) (context.Context, func(prompty.StreamStatus)) {
		return ctx, func(prompty.StreamStatus) { i.closeouts++ }
	})
	return i.stream
}

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestActualStructuredEmptyAndCitationFrames(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{prompty.StreamNative, prompty.StreamBuffered} {
		for _, empty := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/empty=%t", mode, empty), func(t *testing.T) {
				t.Parallel()
				// Arrange: real SDK transport with close accounting and semantic citation metadata.
				var calls, closed atomic.Int32
				citation := `{"type":"web_search_result_location","cited_text":"source","url":"https://example.com","title":null,"encrypted_index":"index=="}`
				text := ""
				if !empty {
					text = `{"answer":"ok"}`
				}
				encoded, _ := json.Marshal(text)
				transport := nativeTransport(func(req *http.Request) (*http.Response, error) {
					calls.Add(1)
					body := `{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":` + string(
						encoded,
					) + `,"citations":[` + citation + `]}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`
					contentType := "application/json"
					if mode == prompty.StreamNative {
						contentType = "text/event-stream"
						events := []string{
							messageStartFixture,
							`{"type":"content_block_start","index":0,"content_block":{"type":"text","text":""}}`,
							`{"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":` + string(
								encoded,
							) + `}}`,
							`{"type":"content_block_delta","index":0,"delta":{"type":"citations_delta","citation":` + citation + `}}`,
							`{"type":"content_block_stop","index":0}`,
							`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":2}}`,
							`{"type":"message_stop"}`,
						}
						var output strings.Builder
						for _, event := range events {
							var header struct {
								Type string `json:"type"`
							}
							_ = json.Unmarshal([]byte(event), &header)
							_, _ = fmt.Fprintf(&output, "event: %s\ndata: %s\n\n", header.Type, event)
						}
						body = output.String()
					}
					return &http.Response{
						StatusCode: http.StatusOK,
						Header:     http.Header{"Content-Type": {contentType}},
						Body:       nativeBody{ReadCloser: io.NopCloser(strings.NewReader(body)), closed: &closed},
						Request:    req,
					}, nil
				})
				sdkClient := sdk.NewClient(
					option.WithBaseURL("https://fixture.invalid"),
					option.WithAPIKey("fixture"),
					option.WithHTTPClient(&http.Client{Transport: transport}),
				)
				profile := prompty.CapabilityProfile{
					Identity: prompty.ProfileIdentity{
						Provider: "anthropic",
						Endpoint: "https://fixture.invalid",
						Model:    "fixture",
					},
					Provenance: "offline SDK/SSE fixture",
					Capabilities: map[string]prompty.CapabilitySupport{
						prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
						prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
					},
				}
				invoker := &task45ObservedInvoker{
					Invoker: adapter.NewClient(
						New(
							WithModel("fixture"),
							WithClient(&sdkClient),
							WithProfiles("https://fixture.invalid", profile),
						),
					),
				}
				execution := prompty.SimplePrompt("fixture")
				terminalCaptures := 0
				execution.Requirements.Capture.Stages = []prompty.CaptureStage{prompty.CaptureTerminal}
				execution.Capture = &prompty.CaptureBinding{
					Sink: batchFixtureSink{write: func(record prompty.CaptureRecord) error {
						if record.Stage == prompty.CaptureTerminal {
							terminalCaptures++
						}
						return nil
					}},
				}
				// Act: provisional structured objects are emitted once despite empty citation frame.
				var items []task45StructuredResult
				var observedErr error
				for item, err := range prompty.StreamStructuredOutput[task45StructuredResult](t.Context(), invoker, execution, mode) {
					items = append(items, item)
					if err != nil {
						observedErr = err
					}
				}
				// Assert: whole-empty failure is deferred to terminal; cleanup/capture occur once.
				if empty {
					require.ErrorIs(t, observedErr, prompty.ErrEmptyTextResponse)
					require.Equal(t, prompty.StreamFailed, invoker.stream.Status().State)
				} else {
					require.NoError(t, observedErr)
					require.Equal(t, []task45StructuredResult{{Answer: "ok"}}, items)
					require.Equal(t, prompty.StreamCompleted, invoker.stream.Status().State)
				}
				require.Len(t, invoker.stream.Status().Result.Annotations, 1)
				require.EqualValues(t, 1, calls.Load())
				require.EqualValues(t, 1, closed.Load())
				require.Equal(t, 1, terminalCaptures)
				require.Equal(t, 1, invoker.closeouts)
			})
		}
	}
}
