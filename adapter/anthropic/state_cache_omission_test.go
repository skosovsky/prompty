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

func TestThinkingCacheOmissionPreservesSDKContinuation(t *testing.T) {
	t.Parallel()
	for _, placement := range []string{"message", "parts", "both", "shadowed required default"} {
		t.Run(placement, func(t *testing.T) {
			t.Parallel()
			// Arrange: real SDK parsing creates the required signed envelopes.
			requests := make(chan []byte, 1)
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
			t.Cleanup(server.Close)
			sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			identity := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"}
			profile := prompty.CapabilityProfile{Identity: identity, Provenance: "local SDK fixture, not live evidence",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityProviderState: prompty.CapabilitySupported,
				}}
			a := New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile))
			var raw anthropic.Message
			require.NoError(t, raw.UnmarshalJSON([]byte(orderedThinkingFixture)))
			response, err := a.ParseResponseForProfile(&raw, identity)
			require.NoError(t, err)
			message, err := response.AssistantMessage()
			require.NoError(t, err)
			if placement == "shadowed required default" {
				message.CachePolicy = &prompty.CachePolicy{
					Mechanism: prompty.CacheBlockMarker, Mode: prompty.CacheRequired, Provider: "anthropic",
					Controls: prompty.JSONDocument(`{"ttl":"1h"}`),
				}
				omitThinkingFixtureBlocks(t, &message)
			}
			if placement != "parts" && placement != "shadowed required default" {
				message.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit}
			}
			if placement != "message" {
				for _, index := range []int{1, 3} {
					part := message.Content[index].(prompty.ReasoningPart)
					part.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit}
					message.Content[index] = part
				}
			}
			exec := prompty.SimplePrompt("fixture").AddMessage(message)
			before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			// Act: persist/restore the omission, then dispatch through the checked client.
			restored, err := prompty.UnmarshalExecution(before)
			require.NoError(t, err)
			result, err := adapter.NewClient(a).Execute(context.Background(), restored)
			// Assert: ordered signed blocks are unchanged and have no cache controls.
			require.NoError(t, err)
			if placement == "shadowed required default" {
				require.NotNil(t, result)
				require.Contains(t, result.Diagnostics.Cache, prompty.CacheDecision{
					Path: "/messages/1/cache_policy", Mechanism: prompty.CacheBlockMarker,
					Disposition: prompty.CacheNotApplicable, Reason: "no content block inherits this message default",
				})
			}
			var request struct {
				Messages []struct {
					Content []json.RawMessage `json:"content"`
				} `json:"messages"`
			}
			require.NoError(t, json.Unmarshal(<-requests, &request))
			require.Len(t, request.Messages[1].Content, 5)
			for _, state := range message.ProviderState {
				block := request.Messages[1].Content[state.PartIndex]
				require.JSONEq(t, string(state.Envelope.Payload), string(block))
				require.NotContains(t, string(block), "cache_control")
			}
			after, err := prompty.MarshalExecution(restored, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func omitThinkingFixtureBlocks(t *testing.T, message *prompty.ChatMessage) {
	t.Helper()
	for index, content := range message.Content {
		switch part := content.(type) {
		case prompty.TextPart:
			part.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit}
			message.Content[index] = part
		case prompty.ReasoningPart:
			part.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit}
			message.Content[index] = part
		default:
			t.Fatalf("unexpected fixture part %T", content)
		}
	}
}

func TestThinkingStateValidatorRejectsCacheControlsAndMalformedOmission(t *testing.T) {
	t.Parallel()
	for _, placement := range []string{"message", "part", "shadowed message"} {
		for _, tc := range []struct {
			name   string
			policy *prompty.CachePolicy
		}{
			{"marker", &prompty.CachePolicy{Mechanism: prompty.CacheBlockMarker}},
			{"implicit", &prompty.CachePolicy{Mechanism: prompty.CacheImplicitPrefix}},
			{"omit mode", &prompty.CachePolicy{Mechanism: prompty.CacheOmit, Mode: prompty.CacheRequired}},
			{"omit provider", &prompty.CachePolicy{Mechanism: prompty.CacheOmit, Provider: "anthropic"}},
			{"omit controls", &prompty.CachePolicy{Mechanism: prompty.CacheOmit, Controls: prompty.JSONDocument(`{}`)}},
		} {
			if placement == "shadowed message" && (tc.name == "marker" || tc.name == "implicit") {
				continue // Valid unused defaults are not signed-block controls.
			}
			t.Run(placement+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				// Arrange: call the public state validator without generic cache preparation.
				identity := prompty.ProfileIdentity{
					Provider: "anthropic",
					Endpoint: "https://fixture.invalid",
					Model:    "fixture",
				}
				a := New()
				var raw anthropic.Message
				require.NoError(t, raw.UnmarshalJSON([]byte(orderedThinkingFixture)))
				response, err := a.ParseResponseForProfile(&raw, identity)
				require.NoError(t, err)
				message, err := response.AssistantMessage()
				require.NoError(t, err)
				if placement == "message" || placement == "shadowed message" {
					message.CachePolicy = tc.policy
					if placement == "shadowed message" {
						omitThinkingFixtureBlocks(t, &message)
					}
				} else {
					part := message.Content[1].(prompty.ReasoningPart)
					part.CachePolicy = tc.policy
					message.Content[1] = &part
				}
				// Act.
				err = a.ValidateProviderState(prompty.SimplePrompt("fixture").AddMessage(message), identity)
				// Assert: omission shape cannot bypass the signed-state boundary.
				require.ErrorIs(t, err, prompty.ErrProviderState)
			})
		}
	}
}
