package hostdispatch

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/skosovsky/contexty"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func richResponse() (*prompty.Response, prompty.ProfileIdentity) {
	destination := prompty.ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"}
	expires := time.Now().Add(time.Hour).Round(0).UTC()
	response := prompty.NewResponse([]prompty.ContentPart{
		prompty.ReasoningPart{Text: "signed summary"},
		prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{0, 255, 1}},
		prompty.ToolCallPart{ID: "native", Name: "lookup", Args: `{"id":9007199254740993}`},
		prompty.ToolResultPart{
			ToolCallID: "native",
			Name:       "lookup",
			IsError:    true,
			Content:    []prompty.ContentPart{prompty.TextPart{Text: "failed"}},
		},
	})
	response.ProviderState = []prompty.PartState{
		{
			PartIndex: 0,
			Envelope: prompty.ProviderEnvelope{
				Format:    prompty.ProviderStateFormat,
				Scope:     destination,
				Codec:     "fixture.signed",
				Payload:   []byte{255, 0, 3},
				Required:  true,
				ExpiresAt: &expires,
			},
		},
	}
	response.Annotations = []prompty.PartAnnotation{
		{PartIndex: 1, Kind: "citation", Value: prompty.JSONDocument(`{"source":"host"}`)},
	}
	response.MessageAnnotations = []prompty.MessageAnnotation{
		{Kind: "fixture.message", Value: prompty.JSONDocument(`{"opaque":true}`), Scope: &destination, Required: true},
	}
	return response, destination
}

func TestOpaqueCheckpointPreservesRichResponse(t *testing.T) {
	t.Parallel()
	// Arrange.
	response, destination := richResponse()
	// Act.
	raw, err := SaveResponse(response, destination)
	require.NoError(t, err)
	restored, err := RestoreResponse(raw, destination)
	// Assert: canonical parts/associations/scopes/bytes/expiry all survive.
	require.NoError(t, err)
	require.Equal(t, response, restored)
	restored.ProviderState[0].Envelope.Payload[0] = 9
	require.EqualValues(t, 255, response.ProviderState[0].Envelope.Payload[0])
}

func TestCheckpointRejectsUnsupportedRestore(t *testing.T) {
	t.Parallel()
	for _, name := range []string{"destination", "expired", "codec", "binding", "omitted", "association"} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			response, destination := richResponse()
			raw, err := SaveResponse(response, destination)
			require.NoError(t, err)
			codec := checkpointCodec()
			carrier, err := contexty.UnmarshalMessageJSON(raw, codec)
			require.NoError(t, err)
			state := carrier.Extensions[0].(contexty.OpaqueState)
			switch name {
			case "destination":
				destination.Model = "other"
			case "codec":
				raw = bytes.ReplaceAll(raw, []byte(checkpointType), []byte("unregistered.codec"))
			case "binding":
				state.Binding.Profile = contexty.Descriptor{ID: "different", Revision: "1"}
			case "expired":
				past := time.Now().Add(-time.Hour)
				response.ProviderState[0].Envelope.ExpiresAt = &past
			case "omitted":
				wire, wireErr := prompty.LoggingResponse(response)
				require.NoError(t, wireErr)
				payload, payloadErr := json.Marshal(wire)
				require.NoError(t, payloadErr)
				state.Payload = responsePayload{Wire: payload}
			case "association":
				raw = bytes.ReplaceAll(raw, []byte(`"part_index":1`), []byte(`"part_index":999`))
			}
			if name == "expired" {
				wire, wireErr := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
				require.NoError(t, wireErr)
				payload, payloadErr := json.Marshal(wire)
				require.NoError(t, payloadErr)
				state.Payload = responsePayload{Wire: payload}
			}
			if name == "binding" || name == "expired" || name == "omitted" {
				carrier.Extensions[0] = state
				raw, err = contexty.MarshalMessageJSON(carrier, codec)
				require.NoError(t, err)
			}
			// Act.
			restored, restoreErr := RestoreResponse(raw, destination)
			// Assert.
			require.Error(t, restoreErr)
			require.Nil(t, restored)
		})
	}
}

func TestSemanticContextRoundtrip(t *testing.T) {
	t.Parallel()
	// Arrange: exact argument bytes and distinct roles pass through real compilation.
	messages := []prompty.ChatMessage{
		{Role: prompty.RoleDeveloper, Content: []prompty.ContentPart{prompty.TextPart{Text: "rule"}}},
		{
			Role: prompty.RoleUser,
			Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image", URL: "https://fixture.invalid/image"},
				prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{255, 0}},
			},
		},
		{
			Role: prompty.RoleAssistant,
			Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "call", Name: "lookup", Args: `{ "id": 9007199254740993 }`},
			},
		},
		{
			Role: prompty.RoleTool,
			Content: []prompty.ContentPart{
				prompty.ToolResultPart{
					ToolCallID: "call",
					Name:       "lookup",
					IsError:    true,
					Content:    []prompty.ContentPart{prompty.TextPart{Text: "failed"}},
				},
			},
		},
	}
	// Act.
	mapped, err := ToContext(messages, []string{"dev", "user", "assistant", "result"})
	require.NoError(t, err)
	compiled, err := contexty.NewEngine().CompileSnapshot(t.Context(), contexty.CompileRequest{History: mapped})
	require.NoError(t, err)
	semanticWire, err := contexty.MarshalMessages(compiled.Payload.History, ContextCodec())
	require.NoError(t, err)
	persisted, err := contexty.UnmarshalMessages(semanticWire, ContextCodec())
	require.NoError(t, err)
	back, err := FromContext(persisted)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, messages, back)
	require.Equal(t, "assistant", compiled.Payload.History[2].ID)
	mapped[1].Parts[1].(contexty.MediaPart).Data[0] = 3
	require.EqualValues(t, 255, messages[1].Content[1].(prompty.MediaPart).Data[0])
}

func TestNativeSemanticUnsupported(t *testing.T) {
	t.Parallel()
	response, _ := richResponse()
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	for _, tc := range []struct {
		name    string
		message prompty.ChatMessage
	}{
		{"opaque state", message},
		{"annotations", prompty.ChatMessage{Role: prompty.RoleAssistant, Annotations: response.Annotations}},
		{"message annotations", prompty.ChatMessage{Role: prompty.RoleAssistant, MessageAnnotations: response.MessageAnnotations}},
		{"reasoning", prompty.ChatMessage{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.ReasoningPart{Text: "private"}}}},
		{"audio", prompty.ChatMessage{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.MediaPart{MediaType: "audio", MIMEType: "audio/wav", Data: []byte{1}}}}},
		{"URL bytes", prompty.ChatMessage{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.MediaPart{MediaType: "image", URL: "https://fixture.invalid/a", Data: []byte{1}}}}},
		{"URL MIME", prompty.ChatMessage{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://fixture.invalid/a"}}}},
		{"nested result", prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{prompty.ToolResultPart{Content: []prompty.ContentPart{prompty.MediaPart{MediaType: "image"}}}}}},
		{"chunks", prompty.ChatMessage{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.ToolCallPart{ArgsChunk: `{`}}}},
		{"cache", prompty.ChatMessage{Role: prompty.RoleUser, CachePolicy: &prompty.CachePolicy{}}},
		{"provenance", prompty.ChatMessage{Role: prompty.RoleUser, Provenance: &prompty.MessageProvenance{LayerID: "caller"}}},
		{"unavailable", prompty.ChatMessage{Role: prompty.RoleAssistant, ContinuationUnavailable: true}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Act / Assert.
			mapped, mapErr := ToContext([]prompty.ChatMessage{tc.message}, []string{"message"})
			require.ErrorIs(t, mapErr, ErrProjection)
			require.Nil(t, mapped)
		})
	}
}

func TestConsumerSemanticUnsupported(t *testing.T) {
	t.Parallel()
	for _, part := range []contexty.ContentPart{
		contexty.ImagePart{URL: "https://fixture.invalid/a", Detail: "high"},
		contexty.MediaPart{MIMEType: "audio/wav", Data: []byte{1}},
		contexty.ToolCallPart{Arguments: contexty.ToolPayload{Data: json.RawMessage(`{"id":1}`), MIMEType: "application/x-sensitive-json"}},
		contexty.ToolCallPart{Arguments: contexty.ToolPayload{Data: json.RawMessage(`{"id":1}`), MIMEType: "application/json"}},
		contexty.ToolResultPart{Payload: contexty.JSONPayload(`{"ok":true}`)},
		contexty.ToolResultPart{Payload: contexty.BinaryPayload([]byte{1}, "application/octet-stream")},
		contexty.ToolResultPart{Payload: contexty.ToolPayload{Error: &contexty.ToolError{Code: "private"}}},
		contexty.ToolResultPart{Payload: contexty.ToolPayload{Progress: &contexty.ToolProgress{}}},
		contexty.ToolResultPart{Payload: contexty.ToolPayload{Control: &contexty.ToolControl{}}},
	} {
		// Arrange / Act.
		mapped, err := FromContext(
			[]contexty.Message{{ID: "message", Role: contexty.RoleTool, Parts: []contexty.ContentPart{part}}},
		)
		// Assert.
		require.ErrorIs(t, err, ErrProjection)
		require.Nil(t, mapped)
	}
	carrier := contexty.TextMessage(contexty.RoleAssistant, "carrier")
	carrier.Extensions = []contexty.Extension{responsePayload{}}
	mapped, err := FromContext([]contexty.Message{carrier})
	require.ErrorIs(t, err, ErrProjection)
	require.Nil(t, mapped)
}

func TestArgumentSidecarRejectsChangedSemantics(t *testing.T) {
	t.Parallel()
	// Arrange.
	messages := []prompty.ChatMessage{
		{
			Role: prompty.RoleAssistant,
			Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "call", Name: "lookup", Args: `{ "id":9007199254740993 }`},
			},
		},
	}
	mapped, err := ToContext(messages, []string{"assistant"})
	require.NoError(t, err)
	part := mapped[0].Parts[0].(contexty.ToolCallPart)
	part.Arguments = contexty.JSONPayload(`{"id":9007199254740992}`)
	mapped[0].Parts[0] = part
	// Act / Assert: stale raw sidecar cannot overwrite changed semantic arguments.
	restored, err := FromContext(mapped)
	require.ErrorIs(t, err, ErrProjection)
	require.Nil(t, restored)
	wire, err := contexty.MarshalMessages(mapped, ContextCodec())
	require.NoError(t, err)
	_, err = contexty.UnmarshalMessages(wire, contexty.DefaultMessageCodec())
	require.Error(t, err)
}

func TestSemanticMappingRejectsInvalidUTF8(t *testing.T) {
	t.Parallel()
	for _, part := range []prompty.ContentPart{
		prompty.TextPart{Text: string([]byte{255})},
		prompty.ToolResultPart{ToolCallID: "call", Name: "lookup", Content: []prompty.ContentPart{prompty.TextPart{Text: string([]byte{255})}}},
		prompty.ToolCallPart{ID: "call", Name: "lookup", Args: string([]byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'})},
	} {
		// Arrange / Act.
		mapped, err := ToContext(
			[]prompty.ChatMessage{{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{part}}},
			[]string{"assistant"},
		)
		// Assert.
		require.ErrorIs(t, err, ErrProjection)
		require.Nil(t, mapped)
	}
	for _, part := range []contexty.ContentPart{
		contexty.TextPart{Text: string([]byte{255})},
		contexty.ToolResultPart{ToolCallID: "call", Name: "lookup", Payload: contexty.TextPayload(string([]byte{255}))},
		contexty.ToolCallPart{ID: "call", Name: "lookup", Arguments: contexty.JSONPayload(string([]byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'}))},
	} {
		// Act / Assert.
		mapped, err := FromContext(
			[]contexty.Message{{ID: "assistant", Role: contexty.RoleAssistant, Parts: []contexty.ContentPart{part}}},
		)
		require.ErrorIs(t, err, ErrProjection)
		require.Nil(t, mapped)
	}
}
