package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestOmittedPartCacheDoesNotReactivateMessageDefault(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name        string
		policy      *prompty.CachePolicy
		disposition prompty.CacheDisposition
	}{
		{"unsupported advisory", &prompty.CachePolicy{
			Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheAdvisory, Provider: "anthropic",
			Controls: prompty.JSONDocument(`{"ttl":"2h"}`),
		}, prompty.CacheUnsupported},
		{"implicit prefix", &prompty.CachePolicy{
			Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: "anthropic", Controls: nil,
		}, prompty.CacheAccepted},
		{"explicit omission", &prompty.CachePolicy{
			Mechanism: prompty.CacheOmit, Mode: "", Provider: "", Controls: nil,
		}, prompty.CacheNotApplicable},
		{"foreign advisory", &prompty.CachePolicy{
			Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheAdvisory, Provider: "foreign",
			Controls: prompty.JSONDocument(`{"ttl":"1h"}`),
		}, prompty.CacheNotApplicable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			checkSDKCacheOverride(t, tc.policy, tc.disposition, false)
		})
	}
}

func TestSDKFullyShadowedCacheDefaultDoesNotRequireTTL(t *testing.T) {
	t.Parallel()
	// Arrange/Act/Assert are performed against the actual SDK/local HTTP by the fixture helper.
	checkSDKCacheOverride(t, &prompty.CachePolicy{Mechanism: prompty.CacheOmit}, prompty.CacheNotApplicable, true)
}

func checkSDKCacheOverride(
	t *testing.T,
	override *prompty.CachePolicy,
	disposition prompty.CacheDisposition,
	shadowed bool,
) {
	t.Helper()
	// Arrange: an unsupported advisory override must not become its parent's required TTL.
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read error", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"fixture","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`,
		)
	}))
	t.Cleanup(server.Close)
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local cache inheritance fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityCacheMarker:   prompty.CapabilitySupported,
			"cache_control:ttl":             prompty.CapabilitySupported,
			prompty.CapabilityCacheImplicit: prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].CachePolicy = &prompty.CachePolicy{
		Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "anthropic",
		Controls: prompty.JSONDocument(`{"ttl":"1h"}`),
	}
	exec.Messages[0].Content = []prompty.ContentPart{
		prompty.TextPart{Text: "override", CachePolicy: override},
		prompty.TextPart{Text: "inherited", CachePolicy: nil},
	}
	if shadowed {
		delete(profile.Capabilities, "cache_control:ttl")
		client = adapter.NewClient(New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
		exec.Messages[0].Content[1] = prompty.TextPart{
			Text:        "overridden",
			CachePolicy: &prompty.CachePolicy{Mechanism: prompty.CacheOmit},
		}
	}
	before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	// Act.
	response, err := client.Execute(context.Background(), exec)
	// Assert: omission is absence of a marker, not replacement with the parent's marker.
	require.NoError(t, err)
	require.Equal(t, disposition, response.Diagnostics.Cache[1].Disposition)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(<-requests, &wire))
	content := wire["messages"].([]any)[0].(map[string]any)["content"].([]any)
	require.NotContains(t, content[0].(map[string]any), "cache_control")
	if shadowed {
		require.NotContains(t, content[1].(map[string]any), "cache_control")
		require.Equal(t, prompty.CacheNotApplicable, response.Diagnostics.Cache[0].Disposition)
	} else {
		require.Equal(t, map[string]any{"type": "ephemeral", "ttl": "1h"}, content[1].(map[string]any)["cache_control"])
	}
	policy := exec.Messages[0].Content[0].(prompty.TextPart).CachePolicy
	require.Equal(t, override, policy)
	after, marshalErr := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, marshalErr)
	require.Equal(t, before, after)
}
