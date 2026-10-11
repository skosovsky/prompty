package anthropic

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestConsumedSourceRejectsSDKCoercions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, content, usage string
		sentinel             error
	}{
		{"numeric text", `[{"type":"text","text":123}]`, `{"input_tokens":1,"output_tokens":2}`, adapter.ErrInvalidResponse},
		{"boolean text", `[{"type":"text","text":true}]`, `{}`, adapter.ErrInvalidResponse},
		{"null text", `[{"type":"text","text":null}]`, `{}`, adapter.ErrInvalidResponse},
		{"numeric tool id", `[{"type":"tool_use","id":1,"name":"lookup","input":{}}]`, `{}`, adapter.ErrInvalidResponse},
		{"boolean tool name", `[{"type":"tool_use","id":"call","name":true,"input":{}}]`, `{}`, adapter.ErrInvalidResponse},
		{"string usage", `[{"type":"text","text":"known"}]`, `{"input_tokens":"10","output_tokens":2}`, prompty.ErrUsage},
		{"boolean usage", `[{"type":"text","text":"known"}]`, `{"input_tokens":true,"output_tokens":2}`, prompty.ErrUsage},
		{"fraction usage", `[{"type":"text","text":"known"}]`, `{"input_tokens":1.5,"output_tokens":2}`, prompty.ErrUsage},
		{"null usage", `[{"type":"text","text":"known"}]`, `{"input_tokens":null,"output_tokens":2}`, prompty.ErrUsage},
		{"future optional", `[{"type":"text","text":"known","future_optional":42}]`, `{"input_tokens":1,"output_tokens":2,"future_optional":true}`, nil},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: the real SDK receives malformed consumed fields from a local endpoint.
			body := `{"id":"msg_fixture","type":"message","model":"fixture","role":"assistant","content":` + test.content + `,"stop_reason":"end_turn","usage":` + test.usage + `}`
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(server.Close)
			sdkClient := sdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local SDK fixture",
			}
			client := adapter.NewClient(
				New(WithModel("fixture"), WithClient(&sdkClient), WithProfiles(server.URL, profile)),
			)
			for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered} {
				// Act.
				var result *prompty.Response
				var err error
				if mode == "" {
					result, err = client.Execute(t.Context(), prompty.SimplePrompt("fixture"))
				} else {
					stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), mode)
					for range stream.Events() {
					}
					result, err = stream.Status().Result, stream.Status().Err
				}
				// Assert: source coercion cannot become successful replayable canonical data.
				if test.sentinel == nil {
					require.NoError(t, err)
					require.NotNil(t, result)
					require.False(t, result.ContinuationUnavailable)
					continue
				}
				require.ErrorIs(t, err, test.sentinel)
				if result != nil && len(result.Content) > 0 {
					require.True(t, result.ContinuationUnavailable)
				}
				if errors.Is(test.sentinel, prompty.ErrUsage) {
					require.NotNil(t, result)
					require.Equal(t, "known", result.Content[0].(prompty.TextPart).Text)
					require.Zero(t, result.Usage.PromptTokens)
					require.NotContains(t, result.Usage.Known, prompty.UsagePrompt)
					require.Equal(t, 2, result.Usage.CompletionTokens)
				}
			}
		})
	}
}

func TestBatchConsumedSourceRejectsSDKCoercions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, line string
		valid      bool
	}{
		{"text", strings.Replace(batchSuccessLine, `"text":"answer"`, `"text":123`, 1), false},
		{"usage", strings.Replace(batchSuccessLine, `"input_tokens":5`, `"input_tokens":"5"`, 1), false},
		{"valid", batchSuccessLine, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: actual SDK batch JSONL result uses the same source-backed parser.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/results") {
					_, _ = fmt.Fprintln(w, test.line)
					_, _ = fmt.Fprintln(w, batchErrorLine)
					_, _ = fmt.Fprintln(w, batchExpiredLine)
					return
				}
				writeBatchFixture(w, "ended")
			}))
			t.Cleanup(server.Close)
			manager, input := batchFixtureManager(server), batchFixtureInput()
			submitted, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			// Act.
			result, _ := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
			// Assert: complete transport/correlation does not imply a malformed successful item.
			require.NotNil(t, result)
			require.Len(t, result.Items, 3)
			item := result.Items[0]
			require.Equal(t, "one", item.ID)
			if test.valid {
				require.Equal(t, prompty.BatchItemSuccess, item.Outcome)
				require.Empty(t, item.ErrorKind)
			} else {
				require.NotEqual(t, prompty.BatchItemSuccess, item.Outcome)
				require.NotEmpty(t, item.ErrorKind)
				if item.Response != nil && len(item.Response.Content) > 0 {
					require.True(t, item.Response.ContinuationUnavailable)
				}
			}
		})
	}
}

func TestNativeConsumedSourceRejectsSDKCoercions(t *testing.T) {
	t.Parallel()
	for _, test := range []struct{ name, block, usage string }{
		{"numeric text", `{"type":"text","text":123}`, `{"output_tokens":2}`},
		{"boolean text", `{"type":"text","text":true}`, `{"output_tokens":2}`},
		{"null text", `{"type":"text","text":null}`, `{"output_tokens":2}`},
		{"numeric tool id", `{"type":"tool_use","id":1,"name":"lookup","input":{}}`, `{"output_tokens":2}`},
		{"boolean tool name", `{"type":"tool_use","id":"call","name":true,"input":{}}`, `{"output_tokens":2}`},
		{"string usage", `{"type":"text","text":"known"}`, `{"output_tokens":"2"}`},
		{"boolean usage", `{"type":"text","text":"known"}`, `{"output_tokens":true}`},
		{"fraction usage", `{"type":"text","text":"known"}`, `{"output_tokens":2.5}`},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: otherwise legal provider SSE; source checks run before SDK values are trusted.
			client, calls := nativeFixture(t, []string{messageStartFixture,
				`{"type":"content_block_start","index":0,"content_block":` + test.block + `}`,
				`{"type":"content_block_stop","index":0}`,
				`{"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":` + test.usage + `}`,
				`{"type":"message_stop"}`,
			})
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert: no successful completed/replayable result and no retries.
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.Error(t, stream.Status().Err)
			require.NotEqual(t, prompty.OutcomeCompleted, stream.Status().Result.Outcome)
			if len(stream.Status().Result.Content) > 0 {
				require.True(t, stream.Status().Result.ContinuationUnavailable)
			}
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
