package anthropic

import (
	"encoding/json"
	"slices"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func mutateHostedManifest(t *testing.T, message *prompty.ChatMessage, mutate func(*hostedHistoryManifest)) {
	t.Helper()
	for index, annotation := range message.MessageAnnotations {
		if annotation.Kind != hostedHistoryAnnotation {
			continue
		}
		var manifest hostedHistoryManifest
		require.NoError(t, json.Unmarshal(annotation.Value, &manifest))
		mutate(&manifest)
		encoded, err := json.Marshal(manifest)
		require.NoError(t, err)
		message.MessageAnnotations[index].Value = encoded
		return
	}
	t.Fatal("fixture manifest unavailable")
}

func TestHostedHistoryChangesFailBeforeIO(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		change func(*testing.T, *prompty.ChatMessage)
	}{
		{"unmarked legacy", func(_ *testing.T, m *prompty.ChatMessage) {
			for index := range m.MessageAnnotations {
				m.MessageAnnotations[index].Required = false
				m.MessageAnnotations[index].Scope = nil
			}
		}},
		{"role", func(_ *testing.T, m *prompty.ChatMessage) { m.Role = prompty.RoleUser }},
		{"text", func(_ *testing.T, m *prompty.ChatMessage) { m.Content[1] = prompty.TextPart{Text: "changed"} }},
		{"citation", func(_ *testing.T, m *prompty.ChatMessage) {
			m.Annotations[0].Value = prompty.JSONDocument(`{"type":"web_search_result_location","title":null,"encrypted_index":"changed","url":"https://example.com","cited_text":"source"}`)
		}},
		{"signature", func(_ *testing.T, m *prompty.ChatMessage) {
			m.ProviderState[0].Envelope.Payload = []byte(`{"type":"thinking","thinking":"plan","signature":"changed"}`)
		}},
		{"local ID", func(_ *testing.T, m *prompty.ChatMessage) {
			call := m.Content[3].(prompty.ToolCallPart)
			call.ID = "changed"
			m.Content[3] = call
		}},
		{"missing manifest", func(_ *testing.T, m *prompty.ChatMessage) {
			m.MessageAnnotations = slices.DeleteFunc(m.MessageAnnotations, func(a prompty.MessageAnnotation) bool { return a.Kind == hostedHistoryAnnotation })
		}},
		{"missing projection", func(_ *testing.T, m *prompty.ChatMessage) { m.MessageAnnotations = m.MessageAnnotations[1:] }},
		{"duplicate manifest", func(_ *testing.T, m *prompty.ChatMessage) {
			m.MessageAnnotations = append(m.MessageAnnotations, m.MessageAnnotations[len(m.MessageAnnotations)-1])
		}},
		{"unknown annotation", func(_ *testing.T, m *prompty.ChatMessage) {
			m.MessageAnnotations = append(m.MessageAnnotations, prompty.MessageAnnotation{Kind: "future", Value: prompty.JSONDocument(`{}`)})
		}},
		{"scope", func(t *testing.T, m *prompty.ChatMessage) {
			mutateHostedManifest(t, m, func(h *hostedHistoryManifest) { h.Scope.Endpoint = "https://other.example.com" })
		}},
		{"missing source", func(t *testing.T, m *prompty.ChatMessage) {
			mutateHostedManifest(t, m, func(h *hostedHistoryManifest) { h.Items = h.Items[1:] })
		}},
		{"reordered source", func(t *testing.T, m *prompty.ChatMessage) {
			mutateHostedManifest(t, m, func(h *hostedHistoryManifest) { h.Items[2], h.Items[3] = h.Items[3], h.Items[2] })
		}},
		{"duplicated part", func(t *testing.T, m *prompty.ChatMessage) {
			mutateHostedManifest(t, m, func(h *hostedHistoryManifest) { h.Items[10].Part = new(0) })
		}},
		{"public signed copy", func(t *testing.T, m *prompty.ChatMessage) {
			mutateHostedManifest(t, m, func(h *hostedHistoryManifest) {
				h.Items[0].Part = nil
				h.Items[0].Source = m.ProviderState[0].Envelope.Payload
			})
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: source-proven hosted history, no new hosted declaration.
			a, calls, _ := hostedRequestFixture(t)
			a.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
			response := parseHostedFixture(t, a, a.profiles[0].Identity)
			message, err := response.AssistantMessage()
			require.NoError(t, err)
			test.change(t, &message)
			exec := prompty.SimplePrompt("fixture")
			exec.Messages = append(exec.Messages, message)
			// Act.
			_, err = adapter.NewClient(a).Execute(t.Context(), exec)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrProviderState)
			require.Zero(t, calls.Load())
		})
	}
}

func TestHostedHistoryRequiresFeatureKnowledgeWithoutNewDeclarations(t *testing.T) {
	t.Parallel()
	for _, native := range []bool{false, true} {
		t.Run(map[bool]string{false: "sync", true: "native"}[native], func(t *testing.T) {
			t.Parallel()
			// Arrange.
			a, calls, _ := hostedRequestFixture(t)
			a.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
			a.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			response := parseHostedFixture(t, a, a.profiles[0].Identity)
			message, err := response.AssistantMessage()
			require.NoError(t, err)
			exec := prompty.SimplePrompt("fixture")
			exec.Messages = append(exec.Messages, message)
			client := adapter.NewClient(a)
			// Act / Assert: unsupported history gate; then native-only combination.
			a.profiles[0].Capabilities[prompty.CapabilityHostedWebSearch] = prompty.CapabilityUnsupported
			_, err = client.Execute(t.Context(), exec)
			require.ErrorIs(t, err, adapter.ErrUnsupportedCapability)
			a.profiles[0].Capabilities[prompty.CapabilityHostedWebSearch] = prompty.CapabilitySupported
			a.profiles[0].ForbiddenCombinations = [][]string{
				{prompty.CapabilityHostedWebFetch, prompty.CapabilityNativeStream},
			}
			if native {
				stream := client.ExecuteStream(t.Context(), exec, prompty.StreamNative)
				for _, err := range stream.Events() {
					require.ErrorIs(t, err, adapter.ErrUnsupportedCombination)
				}
			} else {
				_, err = a.Translate(exec)
				require.NoError(t, err)
			}
			require.Zero(t, calls.Load())
		})
	}
}
