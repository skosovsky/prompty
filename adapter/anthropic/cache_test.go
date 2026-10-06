package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestCheckedCacheTTLThroughSDK(t *testing.T) {
	t.Parallel()
	// Arrange: this profile declares both supported TTL values for a local fixture.
	var calls atomic.Int32
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failed", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fixture","type":"message","role":"assistant","model":"fixture",`+
			`"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn",`+
			`"usage":{"input_tokens":1,"cache_read_input_tokens":2,"cache_creation_input_tokens":3,"output_tokens":4}}`)
	}))
	t.Cleanup(server.Close)
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture-secret"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK cache fixture, not live endpoint proof",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityCacheMarker: prompty.CapabilitySupported,
			"cache_control:ttl":           prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired,
		Provider: "anthropic", Controls: prompty.JSONDocument(`{"ttl":"1h"}`),
	}
	// Act.
	response, err := client.Execute(context.Background(), exec)
	// Assert: selected TTL reaches the request, observed counters keep their meaning.
	require.NoError(t, err)
	require.Equal(t, int32(1), calls.Load())
	require.Equal(t, prompty.CacheAccepted, response.Diagnostics.Cache[0].Disposition)
	require.Equal(t, 6, response.Usage.PromptTokens)
	require.Equal(t, 10, response.Usage.TotalTokens)
	require.Equal(t, 2, response.Usage.PromptTokensCached)
	require.Equal(t, 3, response.Usage.PromptTokensCacheCreation)
	var request map[string]any
	require.NoError(t, json.Unmarshal(<-requests, &request))
	content := request["messages"].([]any)[0].(map[string]any)["content"].([]any)
	require.Equal(t, map[string]any{"type": "ephemeral", "ttl": "1h"}, content[0].(map[string]any)["cache_control"])
	// Missing knowledge or unsupported controls fail before another SDK call.
	for _, tc := range []struct {
		name     string
		controls prompty.JSONDocument
		profile  prompty.CapabilityProfile
	}{
		{"unknown TTL", prompty.JSONDocument(`{"ttl":"1h"}`), prompty.CapabilityProfile{
			Identity: profile.Identity, Provenance: profile.Provenance,
			Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityCacheMarker: prompty.CapabilitySupported},
		}},
		{"unsupported TTL", prompty.JSONDocument(`{"ttl":"2h"}`), profile},
		{"unsupported bypass", prompty.JSONDocument(`{"bypass":true}`), profile},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			candidate := exec.Clone()
			candidate.Messages[0].CachePolicy.Controls = tc.controls
			checked := adapter.NewClient(
				New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, tc.profile)),
			)
			// Act.
			_, requestErr := checked.Execute(context.Background(), candidate)
			// Assert.
			require.ErrorIs(t, requestErr, prompty.ErrCacheRequirement)
			require.Equal(t, int32(1), calls.Load())
		})
	}
}

func TestCacheMarkerLimitAndTTLOrderNeverDispatch(t *testing.T) {
	t.Parallel()
	for _, ttls := range [][]string{{"5m", "1h"}, {"1h", "1h", "5m", "5m", "5m"}} {
		// Arrange.
		exec := prompty.SimplePrompt("fixture")
		exec.Messages[0].Content = nil
		for _, ttl := range ttls {
			exec.Messages[0].Content = append(exec.Messages[0].Content, prompty.TextPart{Text: "block",
				CachePolicy: &prompty.CachePolicy{Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired,
					Provider: "anthropic", Controls: prompty.JSONDocument(`{"ttl":"` + ttl + `"}`)}})
		}
		// Act: absent SDK client cannot be reached for this invalid mapping.
		_, err := New().Translate(exec)
		// Assert.
		require.ErrorIs(t, err, prompty.ErrCacheRequirement)
		require.NotErrorIs(t, err, adapter.ErrNoClient)
	}
}
