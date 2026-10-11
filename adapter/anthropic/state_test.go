package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const orderedThinkingFixture = `{"content":[{"type":"text","text":"first"},` +
	`{"type":"thinking","thinking":"exposed summary","signature":"opaque-signature=="},` +
	`{"type":"text","text":"middle","citations":[{"type":"char_location","cited_text":"source",` +
	`"document_index":0,"document_title":"Doc","start_char_index":0,"end_char_index":6,"future":true}]},` +
	`{"type":"redacted_thinking","data":"opaque-redacted=="},{"type":"text","text":"last"}],` +
	`"stop_reason":"end_turn","usage":{"output_tokens":0}}`

func TestThinkingResponseWireAndEffectiveSDKContinuation(t *testing.T) {
	t.Parallel()
	// Arrange: the effective model differs from the adapter default.
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failed", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, orderedThinkingFixture)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	identity := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"}
	profile := prompty.CapabilityProfile{
		Identity: identity, Provenance: "local SDK fixture, not live capability evidence",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityProviderState: prompty.CapabilitySupported,
		},
	}
	a := New(WithModel("default"), WithClient(&sdk), WithProfiles(server.URL, profile))
	client := adapter.NewClient(a)
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{Model: "fixture"}
	// Act: real SDK parse → response wire → restore → normalized history → execution wire → SDK.
	response, err := client.Execute(context.Background(), exec)
	require.NoError(t, err)
	<-requests
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	var decoded prompty.ResponseWire
	require.NoError(t, json.Unmarshal(data, &decoded))
	restored, err := prompty.UnmarshalResponse(decoded)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	next := exec.AddMessage(message).Clone().Normalize()
	executionWire, err := prompty.MarshalExecution(next, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	next, err = prompty.UnmarshalExecution(executionWire)
	require.NoError(t, err)
	_, err = client.Execute(context.Background(), next)
	require.NoError(t, err)
	var request struct {
		Messages []struct {
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	// Assert: ordered native blocks and opaque signature/data survive exactly.
	require.Len(t, restored.Content, 5)
	require.Len(t, restored.ProviderState, 2)
	require.Len(t, request.Messages[1].Content, 5)
	require.Equal(t, identity, restored.ProviderState[0].Envelope.Scope)
	require.JSONEq(t, string(restored.ProviderState[0].Envelope.Payload), string(request.Messages[1].Content[1]))
	require.JSONEq(t, string(restored.ProviderState[1].Envelope.Payload), string(request.Messages[1].Content[3]))
	require.Equal(t, prompty.ReasoningPart{}, restored.Content[3])
	require.Len(t, restored.Annotations, 1)
	require.Equal(t, 2, restored.Annotations[0].PartIndex)
	require.Contains(t, string(restored.Annotations[0].Value), `"future":true`)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageCompletion}, restored.Usage.Known)
	require.Equal(t, prompty.UsagePartial, restored.Usage.Availability())
	restored.ProviderState[0].Envelope.Payload[0] = 'x'
	require.NotEqual(t, response.ProviderState[0].Envelope.Payload, restored.ProviderState[0].Envelope.Payload)
}

func TestThinkingInvalidContinuationNeverDispatches(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*prompty.PromptExecution)
		err    error
	}{
		{"edited", func(e *prompty.PromptExecution) {
			e.Messages[1].Content[1] = prompty.ReasoningPart{Text: "edited"}
		}, prompty.ErrProviderState},
		{"foreign model", func(e *prompty.PromptExecution) {
			e.ModelOptions = &prompty.ModelOptions{Model: "foreign"}
		}, adapter.ErrUnknownProfile},
		{"missing", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState = nil
		}, prompty.ErrStateUnavailable},
		{"corrupt", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Payload = []byte(`{"type":"thinking","thinking":"exposed summary"}`)
		}, prompty.ErrProviderState},
		{"cross variant null", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Payload = []byte(
				`{"type":"thinking","thinking":"exposed summary","signature":"opaque-signature==","data":null}`)
		}, prompty.ErrProviderState},
		{"case alias", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Payload = []byte(
				`{"type":"thinking","Thinking":"exposed summary","signature":"opaque-signature=="}`)
		}, prompty.ErrProviderState},
		{"duplicate signed field", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Payload = []byte(
				`{"type":"thinking","thinking":"exposed summary","signature":"discarded","signature":"opaque-signature=="}`)
		}, prompty.ErrProviderState},
		{"wrong codec", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Codec = "foreign.codec"
		}, prompty.ErrProviderState},
		{"foreign endpoint", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Scope.Endpoint = "https://foreign.invalid"
		}, prompty.ErrStateDestination},
		{"expired", func(e *prompty.PromptExecution) {
			expired := time.Unix(1, 0)
			e.Messages[1].ProviderState[0].Envelope.ExpiresAt = &expired
		}, prompty.ErrStateExpired},
		{"optional signed state", func(e *prompty.PromptExecution) {
			e.Messages[1].ProviderState[0].Envelope.Required = false
		}, prompty.ErrProviderState},
		{"cache on thinking", func(e *prompty.PromptExecution) {
			e.Messages[1].CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheBlockMarker}
		}, prompty.ErrProviderState},
		{"part cache on thinking", func(e *prompty.PromptExecution) {
			part := e.Messages[1].Content[1].(prompty.ReasoningPart)
			part.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheBlockMarker}
			e.Messages[1].Content[1] = part
		}, prompty.ErrProviderState},
		{"malformed message omission", func(e *prompty.PromptExecution) {
			e.Messages[1].CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit, Mode: prompty.CacheRequired}
		}, prompty.ErrCachePolicy},
		{"malformed part omission", func(e *prompty.PromptExecution) {
			part := e.Messages[1].Content[3].(prompty.ReasoningPart)
			part.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit, Provider: "anthropic"}
			e.Messages[1].Content[3] = part
		}, prompty.ErrCachePolicy},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				http.Error(w, "must not dispatch", http.StatusInternalServerError)
			}))
			defer server.Close()
			sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			identity := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"}
			profile := prompty.CapabilityProfile{Identity: identity, Provenance: "fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityProviderState: prompty.CapabilitySupported,
				}}
			a := New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile))
			var raw anthropic.Message
			require.NoError(t, raw.UnmarshalJSON([]byte(orderedThinkingFixture)))
			response, err := a.ParseResponseForProfile(&raw, identity)
			require.NoError(t, err)
			message, err := response.AssistantMessage()
			require.NoError(t, err)
			exec := prompty.SimplePrompt("fixture").AddMessage(message)
			test.mutate(exec)
			// Act.
			_, err = adapter.NewClient(a).Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, test.err)
			require.Zero(t, calls.Load())
		})
	}
}

func TestThinkingLoggingOmitsOpaqueContinuationAndCannotBeReused(t *testing.T) {
	t.Parallel()
	// Arrange.
	var raw anthropic.Message
	require.NoError(t, raw.UnmarshalJSON([]byte(orderedThinkingFixture)))
	a := New()
	response, err := a.ParseResponse(&raw)
	require.NoError(t, err)
	// Act.
	wire, err := prompty.LoggingResponse(response)
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(wire)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	_, translateErr := a.Translate(prompty.SimplePrompt("fixture").AddMessage(message))
	// Assert.
	require.NotContains(t, string(data), "opaque-signature")
	require.NotContains(t, string(data), "opaque-redacted")
	require.Contains(t, string(data), "exposed summary")
	require.True(t, restored.ContinuationUnavailable)
	require.ErrorIs(t, translateErr, prompty.ErrStateUnavailable)
}

func TestMalformedThinkingResponseFailsClosed(t *testing.T) {
	t.Parallel()
	for _, block := range []string{
		`{"type":"thinking","thinking":"text"}`,
		`{"type":"thinking","thinking":"text","signature":""}`,
		`{"type":"thinking","signature":"signature"}`,
		`{"type":"thinking","thinking":"text","signature":"signature","future":true}`,
		`{"type":"thinking","thinking":"text","signature":"signature","data":null}`,
		`{"type":"thinking","Thinking":"text","signature":"signature"}`,
		`{"type":"thinking","thinking":"text","signature":"discarded","signature":"signature"}`,
		`{"type":"redacted_thinking","data":""}`,
		`{"type":"redacted_thinking","data":"opaque","thinking":"invented"}`,
		`{"type":"redacted_thinking","data":"opaque","thinking":null}`,
	} {
		// Arrange.
		var raw anthropic.Message
		require.NoError(t, raw.UnmarshalJSON([]byte(`{"content":[`+block+`],"stop_reason":"end_turn"}`)))
		// Act.
		_, err := New().ParseResponse(&raw)
		// Assert.
		require.ErrorIs(t, err, prompty.ErrProviderState)
	}
}

func TestThinkingCodecExactShape(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		payload string
		valid   bool
	}{
		{"empty exposed summary", `{"type":"thinking","thinking":"","signature":"signed"}`, true},
		{"reordered escaped strings", ` {"signature":"sig\\n","thinking":"text\n","type":"thinking"} `, true},
		{"redacted", `{"data":"opaque","type":"redacted_thinking"}`, true},
		{"escaped key", `{"type":"thinking","think\u0069ng":"text","signature":"sig"}`, true},
		{"null document", `null`, false},
		{"array", `[]`, false},
		{"trailing document", `{"type":"redacted_thinking","data":"opaque"} {}`, false},
		{"duplicate type", `{"type":"thinking","type":"redacted_thinking","data":"opaque"}`, false},
		{"escaped duplicate", `{"type":"thinking","thinking":"text","think\u0069ng":"text","signature":"sig"}`, false},
		{"case alias", `{"Type":"redacted_thinking","data":"opaque"}`, false},
		{"cross variant null", `{"type":"thinking","thinking":"text","signature":"sig","data":null}`, false},
		{"redacted cross variant null", `{"type":"redacted_thinking","data":"opaque","signature":null}`, false},
		{"thinking null", `{"type":"thinking","thinking":null,"signature":"sig"}`, false},
		{"signature null", `{"type":"thinking","thinking":"text","signature":null}`, false},
		{"nonstring data", `{"type":"redacted_thinking","data":123}`, false},
		{"unknown null", `{"type":"redacted_thinking","data":"opaque","future":null}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			payload := []byte(test.payload)
			// Act.
			_, err := decodeThinking(payload)
			// Assert.
			if test.valid {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, prompty.ErrProviderState)
			}
		})
	}
}
