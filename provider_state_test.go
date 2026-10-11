package prompty

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func stateFixture() *Response {
	return &Response{
		Content: []ContentPart{TextPart{Text: "first"}, ReasoningPart{Text: "summary"}, TextPart{Text: "last"}},
		ProviderState: []PartState{{PartIndex: 1, Envelope: ProviderEnvelope{Format: ProviderStateFormat,
			Scope: ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "known"},
			Codec: "fixture.signature", Payload: []byte("opaque-signature"), Required: true}}},
		Annotations: []PartAnnotation{{PartIndex: 2, Kind: "citation", Value: JSONDocument(`{"source":"fixture"}`)}},
		Outcome:     OutcomeCompleted,
		Usage:       Usage{TotalTokens: 3, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative},
	}
}

func TestResponseStateWireHistoryRoundTrip(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	policy := WirePolicy{State: StatePreserve}
	// Act.
	wire, err := MarshalResponse(response, policy)
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	var decoded ResponseWire
	require.NoError(t, json.Unmarshal(encoded, &decoded))
	restored, err := UnmarshalResponse(decoded)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	exec := SimplePrompt("fixture").AddMessage(message).Clone().Normalize()
	exec.Requirements.Capabilities = []string{CapabilityProviderState}
	executionWire, err := MarshalExecution(exec, policy)
	require.NoError(t, err)
	final, err := UnmarshalExecution(executionWire)
	require.NoError(t, err)
	// Assert.
	require.Equal(t, response.Content, final.Messages[1].Content)
	require.Equal(t, response.ProviderState, final.Messages[1].ProviderState)
	require.Equal(t, response.Annotations, final.Messages[1].Annotations)
	require.Equal(t, exec.Requirements, final.Requirements)
	require.Equal(t, []UsageCounter{UsageTotal}, restored.Usage.Known)
	require.Zero(t, restored.Usage.PromptTokens)
	final.Messages[1].ProviderState[0].Envelope.Payload[0] = 'X'
	final.Messages[1].Annotations[0].Value[0] = 'X'
	require.Equal(t, "opaque-signature", string(response.ProviderState[0].Envelope.Payload))
	require.JSONEq(t, `{"source":"fixture"}`, string(response.Annotations[0].Value))
}

func TestStateOmitProjectionCannotContinue(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	// Act.
	wire, err := MarshalResponse(response, WirePolicy{State: StateOmit})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	restored, err := UnmarshalResponse(wire)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	continuationErr := ValidateContinuation(
		[]ChatMessage{message},
		response.ProviderState[0].Envelope.Scope,
		time.Now(),
	)
	// Assert.
	require.NotContains(t, string(encoded), "opaque-signature")
	require.NotContains(t, string(encoded), base64.StdEncoding.EncodeToString([]byte("opaque-signature")))
	require.Empty(t, wire.ProviderState)
	require.True(t, wire.ContinuationUnavailable)
	require.ErrorIs(t, continuationErr, ErrStateUnavailable)
	require.NotEmpty(t, response.ProviderState)
}

func TestCanonicalJSONDoesNotPersistOpaqueState(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	// Act.
	responseJSON, err := json.Marshal(response)
	require.NoError(t, err)
	messageJSON, err := json.Marshal(message)
	require.NoError(t, err)
	// Assert: persistence requires the explicit wire preservation projection.
	for _, encoded := range [][]byte{responseJSON, messageJSON} {
		require.NotContains(t, string(encoded), "ProviderState")
		require.NotContains(t, string(encoded), "provider_state")
		require.NotContains(t, string(encoded), base64.StdEncoding.EncodeToString([]byte("opaque-signature")))
	}
}

func TestStateWireRejectsMissingFormatAndInvalidAssociation(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	// Act.
	_, responseErr := UnmarshalResponse(ResponseWire{})
	_, executionErr := UnmarshalExecution(PromptExecutionWire{})
	_, policyErr := MarshalResponse(response, WirePolicy{})
	response.ProviderState[0].PartIndex = 99
	_, associationErr := MarshalResponse(response, WirePolicy{State: StatePreserve})
	// Assert.
	require.ErrorIs(t, responseErr, ErrWireFormat)
	require.ErrorIs(t, executionErr, ErrWireFormat)
	require.ErrorIs(t, policyErr, ErrWirePolicy)
	require.ErrorIs(t, associationErr, ErrProviderState)
}

func TestStateScopeExpiryAndNormalizeBoundaries(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	message.Role = RoleSystem
	now := time.Now()
	destination := response.ProviderState[0].Envelope.Scope
	foreign := destination
	foreign.Endpoint = "https://other.invalid"
	exec := (&PromptExecution{Messages: []ChatMessage{message, {Role: RoleSystem, Content: []ContentPart{TextPart{Text: "next"}}}}}).Normalize()
	// Act.
	foreignErr := ValidateContinuation(exec.Messages, foreign, now)
	message.ProviderState[0].Envelope.ExpiresAt = &now
	expiredErr := ValidateContinuation([]ChatMessage{message}, destination, now)
	// Assert.
	require.Len(t, exec.Messages, 2)
	require.ErrorIs(t, foreignErr, ErrStateDestination)
	require.ErrorIs(t, expiredErr, ErrStateExpired)
}

func TestStructuredErrorScratchpadPreservesState(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	response.Content[1] = ReasoningPart{Text: "summary"}
	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) { return response, nil },
	}
	// Act.
	_, err := ExecuteWithStructuredOutput[valueSchemaResult](context.Background(), invoker, SimplePrompt("fixture"))
	// Assert.
	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	require.Equal(t, response.ProviderState, validation.RawAssistantMessage.ProviderState)
	require.Equal(t, response.Annotations, validation.RawAssistantMessage.Annotations)
}
