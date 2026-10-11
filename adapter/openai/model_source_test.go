package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestChatModelSourceActualSDK(t *testing.T) {
	t.Parallel()
	for _, field := range []string{`42`, `true`, `null`, `{}`, `[]`, `"observed","model":"observed"`, `"observed"`} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			// Arrange: otherwise valid completion with a coercible or ambiguous model.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(
					w,
					`{"model":%s,"choices":[{"index":0,"message":{"role":"assistant","content":"known"},"finish_reason":"stop"}],"usage":{"total_tokens":3}}`,
					field,
				)
			}))
			defer server.Close()
			client := sdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local adversarial fixture; allow state admission to test the unavailable flag",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityProviderState: prompty.CapabilitySupported,
				},
			}
			checked := adapter.NewClient(
				New(WithClient(&client), WithModel("fixture"), WithProfiles(server.URL, profile)),
			)
			// Act.
			response, err := checked.Execute(context.Background(), prompty.SimplePrompt("fixture"))
			// Assert: retain evidence without inventing an observed model.
			if field == `"observed"` {
				require.NoError(t, err)
				require.Equal(t, "observed", response.Model)
				require.False(t, response.ContinuationUnavailable)
				_, err = checked.Execute(t.Context(), modelSourceHistory(t, response))
				require.NoError(t, err)
				require.Equal(t, int32(2), calls.Load())
				return
			}
			require.ErrorIs(t, err, adapter.ErrInvalidResponse)
			require.NotNil(t, response)
			require.Empty(t, response.Model)
			require.Equal(t, prompty.OutcomeIncomplete, response.Outcome)
			require.Equal(t, 3, response.Usage.TotalTokens)
			require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "known"}}, response.Content)
			require.True(t, response.ContinuationUnavailable)
			_, err = checked.Execute(t.Context(), modelSourceHistory(t, response))
			require.ErrorIs(t, err, prompty.ErrStateUnavailable)
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestChatStreamModelSourceActualSDK(t *testing.T) {
	t.Parallel()
	for _, field := range []string{`42`, `true`, `null`, `{}`, `[]`, `"observed","model":"observed"`} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			// Arrange: verified prefix followed by a malformed model on a finish frame.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = fmt.Fprint(
					w,
					"data: {\"model\":\"observed\",\"choices\":[{\"index\":0,\"delta\":{\"content\":\"known\"}}]}\n\n",
				)
				_, _ = fmt.Fprintf(
					w,
					"data: {\"model\":%s,\"choices\":[{\"index\":0,\"delta\":{\"content\":\" tail\"},\"finish_reason\":\"stop\"}],\"usage\":{\"total_tokens\":3}}\n\ndata: [DONE]\n\n",
					field,
				)
			}))
			defer server.Close()
			client := sdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
				Provenance: "local adversarial SSE fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream: prompty.CapabilitySupported,
				},
			}
			checked := adapter.NewClient(
				New(WithClient(&client), WithModel("fixture"), WithProfiles(server.URL, profile)),
			)
			stream := checked.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			// Act.
			for range stream.Events() {
			}
			status := stream.Status()
			// Assert: no successful finish or coerced replacement of verified model.
			require.ErrorIs(t, status.Err, prompty.ErrStreamProtocol)
			require.NotEqual(t, prompty.StreamCompleted, status.State)
			require.NotNil(t, status.Result)
			require.True(t, status.Result.ContinuationUnavailable)
			require.Equal(t, "observed", status.Result.Model)
			require.Equal(t, 3, status.Result.Usage.TotalTokens)
			require.Equal(t, []prompty.ContentPart{
				prompty.TextPart{Text: "known"}, prompty.TextPart{Text: " tail"},
			}, status.Result.Content)
		})
	}
}

func TestResponsesModelSourceActualSDK(t *testing.T) {
	t.Parallel()
	for _, field := range []string{`42`, `true`, `null`, `{}`, `[]`, `"42"`, `"42","model":"42"`} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			// Arrange: requested model "42" cannot authenticate a numeric source 42.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(
					w,
					`{"id":"resp_fixture","model":%s,"status":"completed","output":[{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":"output_text","text":"known","annotations":[]}]}],"usage":{"total_tokens":3}}`,
					field,
				)
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			adp.base.defaultModel = "42"
			adp.base.profiles[0].Identity.Model = "42"
			adp.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
			store := false
			// Act.
			result, err := adp.Create(context.Background(), prompty.RemoteResponseRequest{
				Execution: prompty.SimplePrompt("fixture"), Store: &store,
			})
			// Assert.
			require.NotNil(t, result)
			require.NotNil(t, result.Handle)
			require.Equal(t, "resp_fixture", result.Handle.ProviderID)
			if field == `"42"` {
				require.NoError(t, err)
				require.Equal(t, "42", result.Response.Model)
				require.False(t, result.Response.ContinuationUnavailable)
				_, err = adp.Create(t.Context(), prompty.RemoteResponseRequest{
					Execution: modelSourceHistory(t, result.Response), Store: &store, Previous: result.Handle,
				})
				require.NoError(t, err)
				require.Equal(t, int32(2), calls.Load())
				return
			}
			require.ErrorIs(t, err, adapter.ErrInvalidResponse)
			if field == `"42","model":"42"` {
				require.ErrorIs(t, err, prompty.ErrRemoteResponseUnknown)
				return
			}
			require.NotNil(t, result.Response)
			require.Empty(t, result.Response.Model)
			require.Equal(t, 3, result.Response.Usage.TotalTokens)
			require.Equal(t, prompty.OutcomeIncomplete, result.Response.Outcome)
			require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "known"}}, result.Response.Content)
			require.True(t, result.Response.ContinuationUnavailable)
			require.Equal(t, prompty.RemoteResponseCompleted, result.Handle.Status)
			for _, previous := range []*prompty.RemoteResponseHandle{nil, result.Handle} {
				_, err = adp.Create(t.Context(), prompty.RemoteResponseRequest{
					Execution: modelSourceHistory(t, result.Response), Store: &store, Previous: previous,
				})
				require.ErrorIs(t, err, prompty.ErrStateUnavailable)
			}
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func modelSourceHistory(t *testing.T, response *prompty.Response) *prompty.PromptExecution {
	t.Helper()
	// Exercise the portable projection, not just an in-memory flag.
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	history, err := restored.AssistantMessage()
	require.NoError(t, err)
	return prompty.NewExecution([]prompty.ChatMessage{history, prompty.NewUserMessage("continue")})
}

func TestBatchMalformedModelHistoryUnavailable(t *testing.T) {
	t.Parallel()
	// Arrange: numeric model passes the SDK's string comparison with profile "42".
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		switch request.URL.Path {
		case "/v1/files":
			writeBatchUploadFixture(w)
		case "/v1/files/file-errors/content":
			return
		case "/v1/files/file-output/content":
			_, _ = fmt.Fprintln(w, strings.Replace(batchSuccessLine, `"model":"fixture"`, `"model":42`, 1))
		default:
			writeBatchFixture(w, "completed")
		}
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	manager.base.defaultModel = "42"
	manager.base.profiles[0].Identity.Model = "42"
	manager.base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	input.Items = input.Items[:1]
	submitted, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	// Act.
	result, err := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
	// Assert: completed transport retains the invalid item's evidence, not replay permission.
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	item := result.Items[0]
	require.Equal(t, prompty.BatchItemIncomplete, item.Outcome)
	require.Equal(t, "validation_failed", item.ErrorKind)
	require.NotNil(t, item.Response)
	require.True(t, item.Response.ContinuationUnavailable)
	require.Equal(t, 7, item.Response.Usage.TotalTokens)
	checked := adapter.NewClient(manager.base)
	before := calls.Load()
	_, err = checked.Execute(t.Context(), modelSourceHistory(t, item.Response))
	require.ErrorIs(t, err, prompty.ErrStateUnavailable)
	require.Equal(t, before, calls.Load())
}

func TestObservedModelTypedAndAbsent(t *testing.T) {
	t.Parallel()
	for _, source := range []string{"", `{"model":"observed"}`, `{}`} {
		t.Run(source, func(t *testing.T) {
			t.Parallel()
			// Arrange / Act.
			model, err := observedModel(source, "observed")
			// Assert.
			require.NoError(t, err)
			if source == `{}` {
				require.Empty(t, model)
			} else {
				require.Equal(t, "observed", model)
			}
		})
	}
}

func TestChatTypedModelWithoutSource(t *testing.T) {
	t.Parallel()
	// Arrange: a caller-owned SDK value, with no transport JSON to coerce.
	completion := &sdk.ChatCompletion{
		Model: "42",
		Choices: []sdk.ChatCompletionChoice{{
			Index: 0, FinishReason: "stop", Message: sdk.ChatCompletionMessage{Content: "known"},
		}},
	}
	// Act.
	response, err := New().ParseResponse(completion)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "42", response.Model)
}
