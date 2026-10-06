package anthropic

import (
	"context"
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

func TestThinkingShadowedMalformedCacheDefaultNeverDispatches(t *testing.T) {
	t.Parallel()
	// Arrange: every block omits a malformed message default.
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	identity := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"}
	profile := prompty.CapabilityProfile{Identity: identity, Provenance: "local fixture, not live evidence",
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
	message.CachePolicy = &prompty.CachePolicy{Mechanism: prompty.CacheOmit, Mode: prompty.CacheRequired}
	omitThinkingFixtureBlocks(t, &message)
	exec := prompty.SimplePrompt("fixture").AddMessage(message)
	// Act: both direct and checked boundaries must validate unused policy shape.
	stateErr := a.ValidateProviderState(exec, identity)
	_, checkedErr := adapter.NewClient(a).Execute(context.Background(), exec)
	// Assert: no request, no repair of the invalid source default.
	require.ErrorIs(t, stateErr, prompty.ErrProviderState)
	require.ErrorIs(t, checkedErr, prompty.ErrCachePolicy)
	require.Zero(t, calls.Load())
	require.Equal(t, prompty.CacheRequired, exec.Messages[1].CachePolicy.Mode)
}
