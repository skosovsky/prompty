package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func geminiNativeFixture(t *testing.T, events ...string) (prompty.Invoker, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range events {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", event)
		}
	}))
	t.Cleanup(server.Close)
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider:    "gemini",
			Endpoint:    server.URL,
			Model:       "fixture",
			ModelFamily: "fixture-family",
		},
		Provenance:   "local SSE fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityNativeStream: prompty.CapabilitySupported},
	}
	return adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile))), &calls
}

func TestNativePartsExactStateAndLateUsage(t *testing.T) {
	t.Parallel()
	// Arrange: complete tool snapshots, signed reasoning and usage after finish.
	client, calls := geminiNativeFixture(
		t,
		`{"candidates":[{"content":{"role":"model","parts":[{"text":"early"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"id":"a","name":"lookup","args":{"n":"exact-string"}}},{"functionCall":{"id":"b","name":"lookup","args":{}}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"thought":true,"text":"signed","thoughtSignature":"c2lnbmF0dXJl"}]},"finishReason":"STOP"}]}`,
		`{"usageMetadata":{"promptTokenCount":4,"candidatesTokenCount":2,"totalTokenCount":6,"cachedContentTokenCount":0}}`,
	)
	// Act.
	stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	terminals := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if frame.Result != nil {
			terminals++
		}
	}
	status := stream.Status()
	// Assert: no SDK rounding, name merging, state loss or premature finish.
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.OutcomeToolCalls, status.Result.Outcome)
	require.Len(t, status.Result.Content, 4)
	require.Equal(t, "early", status.Result.Content[0].(prompty.TextPart).Text)
	require.JSONEq(t, `{"n":"exact-string"}`, status.Result.Content[1].(prompty.ToolCallPart).Args)
	require.Equal(t, "{}", status.Result.Content[2].(prompty.ToolCallPart).Args)
	require.Equal(t, 6, status.Result.Usage.TotalTokens)
	require.NotContains(t, status.Result.Usage.Known, prompty.UsageCacheRead)
	require.Len(t, status.Result.ProviderState, 1)
	require.Equal(t, status.Result.Diagnostics.Profile, status.Result.ProviderState[0].Envelope.Scope)
	data, err := prompty.MarshalResponse(status.Result, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(data)
	require.NoError(t, err)
	require.Equal(t, status.Result.ProviderState, restored.ProviderState)
}

//nolint:gocognit // Keep SDK malformed-shape characterizations next to their retained-prefix assertions.
func TestNativeFaultsRetainAcceptedPartial(t *testing.T) {
	t.Parallel()
	for _, last := range []string{
		"", `{"candidates":[null]}`, `{"candidates":"private-malformed"}`,
		`{"candidates":[{"content":{"parts":[null]}}]}`,
		`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"private-tool","args":{"huge":1e1000}}}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"text":"lost","text":"kept"}]}}]}`,
		`{"candidates":[{"content":{"parts":[{"Text":"private-signed","thoughtSignature":"c2lnbmF0dXJl"}]}}]}`,
	} {
		t.Run(last, func(t *testing.T) {
			t.Parallel()
			// Arrange: failure after an already accepted early text part.
			events := []string{`{"candidates":[{"content":{"parts":[{"text":"partial"}]}}]}`}
			if last != "" {
				events = append(events, last)
			}
			client, calls := geminiNativeFixture(t, events...)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			if last == `{"candidates":[null]}` || last == `{"candidates":"private-malformed"}` {
				require.Panics(t, func() {
					for range stream.Events() {
					}
				})
				require.EqualValues(t, 1, calls.Load())
				return
			}
			for range stream.Events() {
			}
			status := stream.Status()
			// Assert: safe failed terminal, no panic/replay or loss of early content.
			require.Equal(t, prompty.StreamFailed, status.State)
			require.Error(t, status.Err)
			require.NotContains(t, status.Err.Error(), "private")
			if strings.Contains(last, `"text":"lost","text":"kept"`) ||
				strings.Contains(last, `"Text":"private-signed"`) {
				require.Len(t, status.Result.Content, 2)
			} else {
				require.Len(t, status.Result.Content, 1)
			}
			require.Equal(t, "partial", status.Result.Content[0].(prompty.TextPart).Text)
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestNativeEarlyTextBreakAndCancellation(t *testing.T) {
	t.Parallel()
	for _, cancelConsumer := range []bool{false, true} {
		t.Run(strconv.FormatBool(cancelConsumer), func(t *testing.T) {
			t.Parallel()
			// Arrange: server never finishes until consumer terminates the request.
			closed := make(chan struct{})
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "text/event-stream")
				_, _ = io.WriteString(
					w,
					"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"early\"}]}}]}\n\n",
				)
				w.(http.Flusher).Flush()
				<-r.Context().Done()
				close(closed)
			}))
			t.Cleanup(server.Close)
			sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
				APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
				HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
			})
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			a := New(WithClient(sdk))
			request := &Request{
				Model:    "fixture",
				Contents: genai.Text("fixture"),
				Config: &genai.GenerateContentConfig{
					SystemInstruction: &genai.Content{Parts: []*genai.Part{{Text: "system"}}},
					HTTPOptions:       &genai.HTTPOptions{Headers: http.Header{"X-Host": {"policy"}}},
				},
			}
			// Act: this cannot wait for server EOF and must release the HTTP body.
			parts := 0
			var terminal error
			for frame, callErr := range a.StreamEvents(ctx, request) {
				if callErr != nil {
					terminal = callErr
					break
				}
				parts += len(frame.Content)
				if cancelConsumer {
					cancel()
				} else {
					break
				}
			}
			// Assert: early publication, cancellation cause and immutable caller config.
			require.Equal(t, 1, parts)
			require.Empty(t, request.Config.SystemInstruction.Role)
			require.Equal(t, http.Header{"X-Host": {"policy"}}, request.Config.HTTPOptions.Headers)
			if cancelConsumer {
				require.ErrorIs(t, terminal, context.Canceled)
			}
			select {
			case <-closed:
			case <-time.After(5 * time.Second):
				t.Fatal("native transport did not close")
			}
		})
	}
}

func TestNativePostFinishFaultAndRefusal(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, first, last string
		want              prompty.StreamState
	}{
		{"content-after-finish", `{"candidates":[{"content":{"parts":[{"text":"accepted"}]},"finishReason":"STOP"}]}`, `{"candidates":[{"content":{"parts":[{"text":"bad"}]}}]}`, prompty.StreamFailed},
		{"duplicate-finish", `{"candidates":[{"finishReason":"STOP"}]}`, `{"candidates":[{"finishReason":"STOP"}]}`, prompty.StreamFailed},
		{"refusal", `{"promptFeedback":{"blockReason":"SAFETY"}}`, `{"usageMetadata":{"totalTokenCount":0}}`, prompty.StreamCompleted},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			client, calls := geminiNativeFixture(t, tc.first, tc.last)
			// Act.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert.
			require.Equal(t, tc.want, stream.Status().State)
			require.Equal(t, int32(1), calls.Load())
			if tc.name == "refusal" {
				require.Equal(t, prompty.OutcomeRefusal, stream.Status().Result.Outcome)
				require.NotContains(t, stream.Status().Result.Usage.Known, prompty.UsageTotal)
			}
		})
	}
}

func TestNativeSDKBodyReadFailureAfterFinish(t *testing.T) {
	t.Parallel()
	// Arrange: pinned SDK reports and logs this read error, even after candidate finish.
	body := &sourceFailureBody{reader: strings.NewReader("data: {\"candidates\":[{\"finishReason\":\"STOP\"}]}\n\n")}
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		})}, HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid", APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	a := New(WithClient(sdk))
	// Act.
	var terminal error
	for _, callErr := range a.StreamEvents(context.Background(), &Request{Model: "fixture", Contents: genai.Text("fixture")}) {
		if callErr != nil {
			terminal = callErr
		}
	}
	// Assert: a prior finish cannot turn a transport failure into success.
	require.ErrorIs(t, terminal, prompty.ErrProviderState)
	require.True(t, body.closed.Load())
}
