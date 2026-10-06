package adapter

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestContinuationStateRejectsBeforeTranslateAcrossCheckedModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
		for _, tc := range []struct {
			name   string
			mutate func(*prompty.PromptExecution)
			want   error
		}{
			{"foreign endpoint", func(exec *prompty.PromptExecution) {
				exec.Messages[1].ProviderState[0].Envelope.Scope.Endpoint = "https://other.invalid"
			}, prompty.ErrStateDestination},
			{"foreign model family", func(exec *prompty.PromptExecution) {
				exec.Messages[1].ProviderState[0].Envelope.Scope.ModelFamily = "other-family"
			}, prompty.ErrStateDestination},
			{"expired", func(exec *prompty.PromptExecution) {
				expiry := time.Unix(1, 0)
				exec.Messages[1].ProviderState[0].Envelope.ExpiresAt = &expiry
			}, prompty.ErrStateExpired},
			{"omitted required state", func(exec *prompty.PromptExecution) {
				exec.Messages[1].ProviderState = nil
				exec.Messages[1].ContinuationUnavailable = true
			}, prompty.ErrStateUnavailable},
			{"invalid association", func(exec *prompty.PromptExecution) {
				exec.Messages[1].ProviderState[0].PartIndex = 9
			}, prompty.ErrProviderState},
			{"no state mapping", func(*prompty.PromptExecution) {}, prompty.ErrProviderState},
		} {
			t.Run(string(mode)+"/"+tc.name, func(t *testing.T) {
				t.Parallel()
				// Arrange: supported knowledge cannot invent missing state transport.
				desc := fixtureDescriptor()
				desc.Profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
				fixture := &checkedFixture{descriptor: desc}
				exec := prompty.SimplePrompt("fixture").AddMessage(prompty.ChatMessage{
					Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "state-bound"}},
					ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
						Format: prompty.ProviderStateFormat, Scope: desc.Profiles[0].Identity,
						Codec: "fixture", Payload: []byte("opaque"), Required: true,
					}}},
				})
				tc.mutate(exec)
				before := exec.Clone()
				client := NewClient(nativeCheckedFixture{fixture})
				// Act: consume stream errors rather than treating a lazy handle as dispatch.
				var err error
				if mode == "" {
					_, err = client.Execute(context.Background(), exec)
				} else {
					stream := client.ExecuteStream(context.Background(), exec, mode)
					for _, streamErr := range stream.Events() {
						err = streamErr
					}
					require.Equal(t, prompty.StreamFailed, stream.Status().State)
				}
				// Assert: fail closed before translation with no change to persisted intent.
				require.ErrorIs(t, err, tc.want)
				require.Zero(t, fixture.translations)
				require.Zero(t, fixture.calls)
				require.Equal(t, before, exec)
			})
		}
	}
}
