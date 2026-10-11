package openai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestRequestCacheMapsDistinctSDKRequests(t *testing.T) {
	t.Parallel()
	// Arrange.
	policy := &prompty.CachePolicy{
		Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: "openai",
		Controls: prompty.JSONDocument(`{"prompt_cache_key":" document ","prompt_cache_retention":"24h"}`),
	}
	chat := sdk.ChatCompletionNewParams{}
	remote := responses.ResponseNewParams{}
	// Act.
	require.NoError(t, applyChatRequestCache(&chat, policy))
	responsePolicy := *policy
	responsePolicy.Provider = "openai.responses"
	require.NoError(t, applyResponsesRequestCache(&remote, &responsePolicy))
	chatJSON, err := json.Marshal(chat)
	require.NoError(t, err)
	responseJSON, err := json.Marshal(remote)
	require.NoError(t, err)
	// Assert.
	for _, payload := range [][]byte{chatJSON, responseJSON} {
		var object map[string]any
		require.NoError(t, json.Unmarshal(payload, &object))
		require.Equal(t, " document ", object["prompt_cache_key"])
		require.Equal(t, "24h", object["prompt_cache_retention"])
		require.NotContains(t, object, "cache_control")
	}
	require.Equal(t, "openai", policy.Provider)
	require.ErrorIs(t, applyResponsesRequestCache(&remote, policy), prompty.ErrCacheRequirement)
	require.ErrorIs(t, applyChatRequestCache(&chat, &responsePolicy), prompty.ErrCacheRequirement)
}

func TestRequestCacheStreamModesPreserveIntentAndUsage(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{prompty.StreamNative, prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			bodies := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(request.Body)
				bodies <- body
				if mode == prompty.StreamBuffered {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(
						w,
						`{"id":"chat_fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":3}}}`,
					)
					return
				}
				w.Header().Set("Content-Type", "text/event-stream")
				for _, frame := range []string{
					`{"choices":[{"index":0,"delta":{"role":"assistant","content":"answer"}}]}`,
					`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
					`{"choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":3}}}`,
					"[DONE]",
				} {
					_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
				}
			}))
			defer server.Close()
			client := sdk.NewClient(option.WithAPIKey("fixture"), option.WithBaseURL(server.URL))
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
				Provenance: "actual SDK local stream fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream:         prompty.CapabilitySupported,
					prompty.CapabilityCacheImplicit:        prompty.CapabilitySupported,
					"cache_control:prompt_cache_key":       prompty.CapabilitySupported,
					"cache_control:prompt_cache_retention": prompty.CapabilitySupported,
				},
			}
			exec := prompty.SimplePrompt("question")
			exec.RequestCache = &prompty.CachePolicy{
				Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: "openai",
				Controls: prompty.JSONDocument(`{"prompt_cache_key":"document","prompt_cache_retention":"24h"}`),
			}
			transport := New(WithClient(&client), WithModel("fixture"), WithProfiles(server.URL, profile))
			stream := adapter.NewClient(transport).ExecuteStream(context.Background(), exec, mode)
			// Act.
			for _, err := range stream.Events() {
				require.NoError(t, err)
			}
			status := stream.Status()
			// Assert.
			require.Equal(t, prompty.StreamCompleted, status.State)
			require.Equal(t, prompty.OutcomeCompleted, status.Result.Outcome)
			require.Equal(t, 12, status.Result.Usage.TotalTokens)
			require.Equal(t, 3, status.Result.Usage.PromptTokensCached)
			require.EqualValues(t, 1, calls.Load())
			var body map[string]any
			require.NoError(t, json.Unmarshal(<-bodies, &body))
			require.Equal(t, "document", body["prompt_cache_key"])
			require.Equal(t, "24h", body["prompt_cache_retention"])
			require.Equal(t, prompty.CacheRequired, exec.RequestCache.Mode)
		})
	}
}

func TestRequestCacheActualSDKDispatchAndPreflight(t *testing.T) {
	t.Parallel()
	for _, provider := range []string{"openai", "openai.responses"} {
		t.Run(provider, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			bodies := make(chan []byte, 3)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				body, _ := io.ReadAll(request.Body)
				bodies <- body
				if provider == "openai.responses" {
					writeResponseFixture(w, "completed", "answer")
					return
				}
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(
					w,
					`{"id":"chat_fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":2,"total_tokens":12,"prompt_tokens_details":{"cached_tokens":3}}}`,
				)
			}))
			defer server.Close()
			remote := responseFixtureAdapter(server)
			profile := remote.base.profiles[0]
			profile.Identity.Provider = provider
			profile.Capabilities[prompty.CapabilityCacheImplicit] = prompty.CapabilitySupported
			profile.Capabilities["cache_control:prompt_cache_key"] = prompty.CapabilitySupported
			profile.Capabilities["cache_control:prompt_cache_retention"] = prompty.CapabilitySupported
			client := sdk.NewClient(option.WithAPIKey("fixture"), option.WithBaseURL(server.URL+"/v1"))
			chat := New(WithModel("fixture"), WithClient(&client), WithProfiles(server.URL+"/v1", profile))
			remote.base.profiles[0].Capabilities = profile.Capabilities
			exec := prompty.SimplePrompt("question")
			exec.RequestCache = &prompty.CachePolicy{
				Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: provider,
				Controls: prompty.JSONDocument(`{"prompt_cache_key":"document","prompt_cache_retention":"in-memory"}`),
			}
			dispatch := func() error {
				if provider == "openai.responses" {
					_, err := remote.Create(context.Background(), prompty.RemoteResponseRequest{
						Execution: exec, Store: new(bool),
					})
					return err
				}
				_, err := adapter.NewClient(chat).Execute(context.Background(), exec)
				return err
			}
			// Act.
			err := dispatch()
			// Assert: actual SDK HTTP payload, not only parameter construction.
			require.NoError(t, err)
			require.EqualValues(t, 1, calls.Load())
			var body map[string]any
			require.NoError(t, json.Unmarshal(<-bodies, &body))
			require.Equal(t, "document", body["prompt_cache_key"])
			require.Equal(t, "in-memory", body["prompt_cache_retention"])
			require.NotContains(t, body, "cached_content")
			require.Equal(t, prompty.CacheRequired, exec.RequestCache.Mode)
			// Arrange: a mapper cannot invent a bypass knob.
			exec.RequestCache.Controls = prompty.JSONDocument(`{"bypass":true}`)
			// Act / Assert: required is zero I/O; advisory removes only the working policy.
			require.ErrorIs(t, dispatch(), prompty.ErrCacheRequirement)
			require.EqualValues(t, 1, calls.Load())
			exec.RequestCache.Mode = prompty.CacheAdvisory
			require.NoError(t, dispatch())
			require.EqualValues(t, 2, calls.Load())
			body = nil
			require.NoError(t, json.Unmarshal(<-bodies, &body))
			require.NotContains(t, body, "prompt_cache_key")
			require.NotContains(t, body, "prompt_cache_retention")
			require.Equal(t, prompty.JSONDocument(`{"bypass":true}`), exec.RequestCache.Controls)
			// Arrange: installed SDK fields do not imply endpoint/model support.
			exec.RequestCache.Mode = prompty.CacheRequired
			exec.RequestCache.Controls = prompty.JSONDocument(`{"prompt_cache_key":"document"}`)
			chat.profiles[0].Capabilities["cache_control:prompt_cache_key"] = prompty.CapabilityUnknown
			remote.base.profiles[0].Capabilities["cache_control:prompt_cache_key"] = prompty.CapabilityUnknown
			// Act / Assert.
			require.ErrorIs(t, dispatch(), prompty.ErrCacheRequirement)
			require.EqualValues(t, 2, calls.Load())
		})
	}
}

func TestRequestCacheRejectsUnsupportedKnobs(t *testing.T) {
	t.Parallel()
	for _, controls := range []string{
		`{"prompt_cache_key":""}`, `{"prompt_cache_key":null}`, `{"prompt_cache_key":1}`,
		`{"prompt_cache_retention":"forever"}`, `{"bypass":true}`, `{"ttl":"1h"}`,
		`{"prompt_cache_key":"a","prompt_cache_key":"b"}`,
	} {
		t.Run(controls, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			policy := &prompty.CachePolicy{
				Mechanism: prompty.CacheImplicitPrefix, Provider: "openai", Controls: prompty.JSONDocument(controls),
			}
			// Act.
			_, err := openAIRequestCache(policy, "openai")
			// Assert.
			require.ErrorIs(t, err, prompty.ErrCacheRequirement)
			require.NotContains(t, err.Error(), controls)
		})
	}
}
