package prompty

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStateHistoryPortsCloneExpiryPayloadAndAnnotations(t *testing.T) {
	t.Parallel()
	for _, port := range []string{"NewExecution", "AddMessage", "WithHistory", "WithMessages", "Clone", "Normalize"} {
		t.Run(port, func(t *testing.T) {
			t.Parallel()
			// Arrange: every mutable state component is caller-owned on input.
			response := stateFixture()
			expiry := time.Date(2035, time.January, 1, 0, 0, 0, 0, time.UTC)
			response.ProviderState[0].Envelope.ExpiresAt = &expiry
			message, err := response.AssistantMessage()
			require.NoError(t, err)
			input := NewExecution([]ChatMessage{message})
			// Act.
			output := projectStateHistoryPort(port, input)
			require.NotNil(t, output)
			last := &output.Messages[len(output.Messages)-1]
			require.Equal(t, message.Content, last.Content)
			require.Equal(t, message.ProviderState, last.ProviderState)
			require.Equal(t, message.Annotations, last.Annotations)
			last.ProviderState[0].Envelope.Payload[0] = 'X'
			last.Annotations[0].Value[0] = 'X'
			*last.ProviderState[0].Envelope.ExpiresAt = expiry.Add(time.Hour)
			// Assert: mutation of the new history cannot change any earlier ownership layer.
			for _, states := range [][]PartState{response.ProviderState, message.ProviderState, input.Messages[0].ProviderState} {
				require.Equal(t, "opaque-signature", string(states[0].Envelope.Payload))
				require.Equal(t, expiry, *states[0].Envelope.ExpiresAt)
			}
			for _, annotations := range [][]PartAnnotation{response.Annotations, message.Annotations, input.Messages[0].Annotations} {
				require.JSONEq(t, `{"source":"fixture"}`, string(annotations[0].Value))
			}
		})
	}
}

func TestLoggingExecutionOmitsStateWithoutChangingPersistenceSource(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	execution := NewExecution([]ChatMessage{message})
	// Act.
	logging, err := LoggingExecution(execution)
	require.NoError(t, err)
	encoded, err := json.Marshal(logging)
	require.NoError(t, err)
	restored, err := DecodeExecutionJSON(encoded)
	require.NoError(t, err)
	continuationErr := ValidateContinuation(restored.Messages, response.ProviderState[0].Envelope.Scope, time.Now())
	persistence, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	_, missingPolicyErr := MarshalExecution(execution, WirePolicy{})
	// Assert: logging is not a silently reusable continuation or a replacement for preservation.
	require.Empty(t, logging.Messages[0].ProviderState)
	require.True(t, logging.Messages[0].ContinuationUnavailable)
	require.ErrorIs(t, continuationErr, ErrStateUnavailable)
	require.ErrorIs(t, missingPolicyErr, ErrWirePolicy)
	require.NotContains(t, string(encoded), base64.StdEncoding.EncodeToString([]byte("opaque-signature")))
	require.Equal(t, execution.Messages[0].ProviderState, persistence.Messages[0].ProviderState)
	require.Equal(t, response.ProviderState, execution.Messages[0].ProviderState)
	require.False(t, execution.Messages[0].ContinuationUnavailable)
}

func projectStateHistoryPort(port string, input *PromptExecution) *PromptExecution {
	switch port {
	case "NewExecution":
		return NewExecution(input.Messages)
	case "AddMessage":
		return SimplePrompt("fixture").AddMessage(input.Messages[0])
	case "WithHistory":
		return SimplePrompt("fixture").WithHistory(input.Messages)
	case "WithMessages":
		return SimplePrompt("fixture").WithMessages(input.Messages)
	case "Clone":
		return input.Clone()
	case "Normalize":
		return input.Normalize()
	default:
		return nil
	}
}

func TestNormalizePreservesEveryAssociatedMessageBoundary(t *testing.T) {
	t.Parallel()
	for _, association := range []string{"state", "annotation", "unavailable"} {
		t.Run(association, func(t *testing.T) {
			t.Parallel()
			for _, boundIndex := range []int{0, 1} {
				// Arrange: both messages otherwise qualify for system/developer merging.
				source := stateFixture()
				messages := []ChatMessage{
					{
						Role: RoleSystem,
						Content: []ContentPart{
							TextPart{Text: "first"},
							ReasoningPart{Text: "summary"},
							TextPart{Text: "last"},
						},
					},
					{
						Role: RoleDeveloper,
						Content: []ContentPart{
							TextPart{Text: "first"},
							ReasoningPart{Text: "summary"},
							TextPart{Text: "last"},
						},
					},
				}
				switch association {
				case "state":
					messages[boundIndex].ProviderState = source.ProviderState
				case "annotation":
					messages[boundIndex].Annotations = source.Annotations
				case "unavailable":
					messages[boundIndex].ContinuationUnavailable = true
				}
				input := NewExecution(messages)
				// Act.
				output := input.Normalize()
				// Assert: no inserted separator, reindexing or merge across an associated boundary.
				require.Equal(t, input.Messages, output.Messages)
				require.Len(t, output.Messages, 2)
			}
		})
	}
}

func TestStateWireClonesExpiryAtEveryBoundary(t *testing.T) {
	t.Parallel()
	// Arrange.
	response := stateFixture()
	expiry := time.Date(2035, time.January, 1, 0, 0, 0, 0, time.UTC)
	deadline := expiry
	response.ProviderState[0].Envelope.ExpiresAt = &deadline
	// Act.
	responseWire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	restored, err := UnmarshalResponse(responseWire)
	require.NoError(t, err)
	message, err := restored.AssistantMessage()
	require.NoError(t, err)
	execution := NewExecution([]ChatMessage{message})
	executionWire, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	recovered, err := UnmarshalExecution(executionWire)
	require.NoError(t, err)
	// Assert: every wire/history/runtime layer owns a different expiry allocation.
	states := [][]PartState{response.ProviderState, responseWire.ProviderState, restored.ProviderState,
		message.ProviderState, execution.Messages[0].ProviderState, executionWire.Messages[0].ProviderState,
		recovered.Messages[0].ProviderState}
	for i, current := range states {
		*current[0].Envelope.ExpiresAt = expiry.Add(time.Hour)
		for j, other := range states {
			if i != j {
				require.Equal(t, expiry, *other[0].Envelope.ExpiresAt)
			}
		}
		*current[0].Envelope.ExpiresAt = expiry
	}
}
