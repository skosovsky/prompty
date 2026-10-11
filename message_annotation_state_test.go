package prompty

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func scopedAnnotationFixture() MessageAnnotation {
	return MessageAnnotation{Kind: "fixture.history", Value: JSONDocument(`{"opaque":"preserved"}`), Required: true,
		Scope: new(ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "model"})}
}

func TestScopedMessageAnnotationCloneWireAndLogging(t *testing.T) {
	t.Parallel()
	// Arrange: state lives at message level, with no artificial content/state part.
	annotation := scopedAnnotationFixture()
	exec := SimplePrompt("fixture")
	exec.Messages[0].MessageAnnotations = []MessageAnnotation{annotation,
		{Kind: "host.trace", Value: JSONDocument(`{"trace":"owned"}`)},
		{Kind: "fixture.optional", Value: JSONDocument(`{"optional":true}`), Scope: annotation.Scope}}
	// Act: clone, wire restore, response projection and explicit logging omission.
	owned := exec.Clone()
	wire, err := MarshalExecution(exec, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(encoded)
	require.NoError(t, err)
	response := NewResponse(exec.Messages[0].Content)
	response.MessageAnnotations = exec.Messages[0].MessageAnnotations
	responseWire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	logging, err := LoggingExecution(exec)
	require.NoError(t, err)
	loggedResponse, err := LoggingResponse(response)
	require.NoError(t, err)
	owned.Messages[0].MessageAnnotations[0].Scope.Endpoint = "https://changed.invalid"
	wire.Messages[0].MessageAnnotations[0].Scope.Model = "changed"
	responseWire.MessageAnnotations[0].Scope.Provider = "changed"
	// Assert: markers and bytes survive and every ownership boundary is isolated.
	require.Equal(t, annotation, restored.Messages[0].MessageAnnotations[0])
	require.Equal(t, "model", exec.Messages[0].MessageAnnotations[0].Scope.Model)
	require.Equal(t, "fixture", response.MessageAnnotations[0].Scope.Provider)
	require.Equal(t, "https://fixture.invalid", exec.Messages[0].MessageAnnotations[0].Scope.Endpoint)
	require.Empty(t, restored.Messages[0].ProviderState)
	require.True(t, logging.Messages[0].ContinuationUnavailable)
	require.True(t, loggedResponse.ContinuationUnavailable)
	require.Equal(
		t,
		[]MessageAnnotation{exec.Messages[0].MessageAnnotations[1]},
		logging.Messages[0].MessageAnnotations,
	)
	require.Equal(t, logging.Messages[0].MessageAnnotations, loggedResponse.MessageAnnotations)
	require.Len(t, exec.Messages[0].MessageAnnotations, 3)
}

func TestScopedMessageAnnotationShapeAndDestination(t *testing.T) {
	t.Parallel()
	// Arrange.
	annotation := scopedAnnotationFixture()
	for _, invalid := range []MessageAnnotation{
		{Kind: "fixture", Value: JSONDocument(`{}`), Required: true},
		{Kind: "fixture", Value: JSONDocument(`{}`), Scope: new(ProfileIdentity{})},
		{Kind: "fixture", Value: JSONDocument(`{}`), Scope: new(ProfileIdentity{Provider: "fixture", Endpoint: "https://user:secret@example.com", Model: "model"})},
	} {
		// Act / Assert: incomplete/credential-bearing scope never becomes valid state.
		require.ErrorIs(t, ValidateMessageAnnotations([]MessageAnnotation{invalid}), ErrProviderState)
	}
	message := NewAssistantMessage("fixture")
	message.MessageAnnotations = []MessageAnnotation{annotation}
	for _, destination := range []ProfileIdentity{
		{Provider: "other", Endpoint: annotation.Scope.Endpoint, Model: "model"},
		{Provider: "fixture", Endpoint: "https://other.invalid", Model: "model"},
		{Provider: "fixture", Endpoint: annotation.Scope.Endpoint, Model: "other"},
	} {
		require.ErrorIs(t, ValidateContinuation([]ChatMessage{message}, destination, time.Now()), ErrStateDestination)
	}
	require.NoError(t, ValidateContinuation([]ChatMessage{message}, *annotation.Scope, time.Now()))
	message.MessageAnnotations[0].Scope.ModelFamily = "family"
	require.NoError(t, ValidateContinuation([]ChatMessage{message}, ProfileIdentity{
		Provider: "fixture", Endpoint: annotation.Scope.Endpoint, Model: "other", ModelFamily: "family",
	}, time.Now()))
	// Optional scoped state may be omitted without revoking continuation.
	message.MessageAnnotations[0].Required = false
	projected, err := projectMessages([]ChatMessage{message}, WirePolicy{State: StateOmit})
	require.NoError(t, err)
	require.Empty(t, projected[0].MessageAnnotations)
	require.False(t, projected[0].ContinuationUnavailable)
}

func TestScopedMessageAnnotationWireRejectsUnscopedRequiredMarker(t *testing.T) {
	t.Parallel()
	// Arrange.
	wire, err := MarshalExecution(SimplePrompt("fixture"), WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	wire.Messages[0].MessageAnnotations = []MessageAnnotation{
		{Kind: "fixture", Value: JSONDocument(`{}`), Required: true},
	}
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	// Act.
	_, err = DecodeExecutionJSON(encoded)
	// Assert: the executable schema and in-memory shape both enforce the marker.
	require.Error(t, err)
	require.ErrorIs(t, ValidateMessageAnnotations(wire.Messages[0].MessageAnnotations), ErrProviderState)
}
