package prompty

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHostLoggingPolicyDoesNotChangePersistenceSource(t *testing.T) {
	t.Parallel()
	// Arrange: credentials in arbitrary content are host data, not detectable by core.
	response := stateFixture()
	response.Content[0] = TextPart{Text: "content-secret"}
	response.Annotations[0].Value = JSONDocument(`{"token":"annotation-secret"}`)
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	execution := NewExecution([]ChatMessage{message})
	execution.ModelOptions = &ModelOptions{ProviderSettings: JSONDocument(`{"token":"settings-secret"}`)}
	before, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	// Act: application logging policy omits content/annotations and provider settings.
	logging, err := LoggingExecution(execution)
	require.NoError(t, err)
	// State omission alone explicitly is not a general credentials filter.
	require.Contains(t, string(logging.Provider), "settings-secret")
	logging.Provider = nil
	for i := range logging.Messages {
		logging.Messages[i].Content = nil
		logging.Messages[i].Annotations = nil
	}
	encoded, err := json.Marshal(logging)
	require.NoError(t, err)
	after, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	// Assert: explicit host logging policy is isolated, with no reusable state claim.
	for _, secret := range []string{"content-secret", "annotation-secret", "settings-secret",
		base64.StdEncoding.EncodeToString([]byte("opaque-signature"))} {
		require.NotContains(t, string(encoded), secret)
	}
	require.True(t, logging.Messages[0].ContinuationUnavailable)
	require.Equal(t, before, after)
	require.Equal(t, response.ProviderState, after.Messages[0].ProviderState)
}

func TestHostCaptureAllowlistOmitsCredentialsBodiesAndState(t *testing.T) {
	t.Parallel()
	// Arrange: the same raw secret-bearing payload can arrive at any exposed stage.
	stages := []CaptureStage{CaptureExecution, CaptureTranslatedRequest, CaptureProviderResponse, CaptureTerminal}
	payload := []byte(
		`{"authorization":"auth-secret","error_body":"body-secret","output":"output-secret","provider_state":"state-secret"}`,
	)
	original := string(payload)
	usage := Usage{TotalTokens: 3, Known: []UsageCounter{UsageTotal}}
	metadata := CaptureRecordMetadata{RequestID: "request-secret", ProviderResponseID: "response-secret",
		Usage: &usage, Outcome: OutcomeCompleted}
	var records []CaptureRecord
	binding := &CaptureBinding{
		Sink: fixtureCaptureSink{write: func(_ context.Context, record CaptureRecord) (CaptureAck, error) {
			records = append(records, record)
			return CaptureAck{}, nil
		}},
		Projector: fixtureCaptureProjector{accuracy: CaptureSemantic,
			project: func(_ context.Context, record CaptureRecord) (CaptureProjection, error) {
				// Explicit application allowlist, not generic substring-based secret detection.
				projection := CaptureProjection{Payload: []byte(`{"content":"omitted by host policy"}`),
					MediaType: captureJSONMediaType, Codec: "host.safe-summary", Accuracy: CaptureSemantic,
					Redacted: true, Complete: false}
				projection.Metadata.Usage = record.Projection.Metadata.Usage
				projection.Metadata.Outcome = record.Projection.Metadata.Outcome
				return projection, nil
			}},
	}
	session, err := NewCaptureSession(
		CaptureRequirement{Stages: stages, Accuracy: CaptureSemantic},
		binding,
		fixtureCaptureCoverage(
			CaptureSemantic,
			stages...),
		ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid"},
	)
	require.NoError(t, err)
	// Act.
	for _, stage := range stages {
		require.NoError(t, session.Emit(context.Background(), CaptureRecord{Stage: stage,
			Projection: CaptureProjection{Payload: payload, Metadata: metadata}}))
	}
	// Assert: only approved observations reach the sink; redaction is never complete replay.
	require.Len(t, records, len(stages))
	for _, record := range records {
		require.NotContains(t, string(record.Projection.Payload), "secret")
		require.Empty(t, record.Profile.Endpoint)
		require.Empty(t, record.Projection.Metadata.RequestID)
		require.Empty(t, record.Projection.Metadata.ProviderResponseID)
		require.Equal(t, usage, *record.Projection.Metadata.Usage)
		require.True(t, record.Projection.Redacted)
		require.False(t, record.Projection.Complete)
	}
	require.True(t, session.Report().Closed)
	require.False(t, session.Report().Complete)
	require.Empty(t, session.Report().Failures)
	require.Equal(t, original, string(payload))
	require.Equal(t, "request-secret", metadata.RequestID)
	require.Equal(t, "response-secret", metadata.ProviderResponseID)
}
