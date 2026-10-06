package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCaptureMetadataIsolatedAcrossCallbacks(t *testing.T) {
	t.Parallel()
	// Arrange.
	status := 200
	usage := Usage{TotalTokens: 9, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative}
	metadata := CaptureRecordMetadata{
		ProviderResponseID: "response",
		RequestID:          "request",
		HTTPStatus:         &status,
		Usage:              &usage,
		Outcome:            OutcomeCompleted,
		SchemaDigests: []CaptureSchemaDigest{
			{Role: SchemaOutput, SourceDigest: "source", TranslatedDigest: "translated"},
		},
	}
	writes := 0
	binding := &CaptureBinding{
		Projector: fixtureCaptureProjector{accuracy: CaptureSemantic,
			project: func(_ context.Context, record CaptureRecord) (CaptureProjection, error) {
				require.Equal(t, 200, *record.Projection.Metadata.HTTPStatus)
				require.Equal(t, []UsageCounter{UsageTotal}, record.Projection.Metadata.Usage.Known)
				require.Equal(t, "source", record.Projection.Metadata.SchemaDigests[0].SourceDigest)
				*record.Projection.Metadata.HTTPStatus = 201
				return record.Projection, nil
			}},
		Sink: fixtureCaptureSink{write: func(_ context.Context, record CaptureRecord) (CaptureAck, error) {
			writes++
			require.Equal(t, 201, *record.Projection.Metadata.HTTPStatus)
			*record.Projection.Metadata.HTTPStatus = 599
			record.Projection.Metadata.Usage.Known[0] = UsagePrompt
			record.Projection.Metadata.SchemaDigests[0].SourceDigest = "changed"
			return CaptureAck{}, nil
		}},
	}
	session, err := NewCaptureSession(CaptureRequirement{Accuracy: CaptureSemantic}, binding,
		fixtureCaptureCoverage(CaptureSemantic, CaptureCanonicalParts, CaptureTerminal), ProfileIdentity{})
	require.NoError(t, err)
	// Act.
	for _, stage := range []CaptureStage{CaptureCanonicalParts, CaptureTerminal} {
		err = session.Emit(context.Background(), CaptureRecord{Stage: stage, Projection: CaptureProjection{
			Metadata: metadata, Payload: []byte(`{}`), MediaType: captureJSONMediaType,
			Accuracy: CaptureSemantic, Complete: true,
		}})
		require.NoError(t, err)
	}
	// Assert: host edits apply to its projection only, never caller/source state or later records.
	require.Equal(t, 2, writes)
	require.Equal(t, 200, status)
	require.Equal(t, []UsageCounter{UsageTotal}, usage.Known)
	require.Equal(t, "source", metadata.SchemaDigests[0].SourceDigest)
}

func TestDefaultCaptureMetadataOmitsResponseAndRequestIdentity(t *testing.T) {
	t.Parallel()
	// Arrange.
	usage := Usage{TotalTokens: 0, Known: []UsageCounter{UsageTotal}, Mode: UsageCumulative}
	var observed CaptureRecord
	session, err := NewCaptureSession(CaptureRequirement{}, &CaptureBinding{
		Sink: fixtureCaptureSink{write: func(_ context.Context, record CaptureRecord) (CaptureAck, error) {
			observed = record
			return CaptureAck{}, nil
		}},
	}, fixtureCaptureCoverage(CaptureSemantic, CaptureTerminal), ProfileIdentity{})
	require.NoError(t, err)
	// Act.
	err = session.Emit(context.Background(), CaptureRecord{Stage: CaptureTerminal, Projection: CaptureProjection{
		Metadata: CaptureRecordMetadata{ProviderResponseID: "private-response", RequestID: "private-request",
			Usage: &usage, Outcome: OutcomeCompleted},
	}})
	// Assert.
	require.NoError(t, err)
	require.Empty(t, observed.Projection.Metadata.ProviderResponseID)
	require.Empty(t, observed.Projection.Metadata.RequestID)
	require.Nil(t, observed.Projection.Metadata.HTTPStatus)
	require.Equal(t, usage, *observed.Projection.Metadata.Usage)
	require.Equal(t, OutcomeCompleted, observed.Projection.Metadata.Outcome)
	require.Empty(t, observed.Projection.Payload)
	require.False(t, session.Report().Complete)
}

func TestCaptureRejectsInvalidMetadataBeforeSink(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name     string
		metadata CaptureRecordMetadata
	}{
		{"usage", CaptureRecordMetadata{Usage: &Usage{TotalTokens: -1}}},
		{"outcome", CaptureRecordMetadata{Outcome: "invented"}},
		{"status", CaptureRecordMetadata{HTTPStatus: new(int)}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			writes := 0
			session, err := NewCaptureSession(
				CaptureRequirement{Stages: []CaptureStage{CaptureTerminal}},
				&CaptureBinding{
					Sink: fixtureCaptureSink{write: func(context.Context, CaptureRecord) (CaptureAck, error) {
						writes++
						return CaptureAck{}, nil
					}},
				},
				fixtureCaptureCoverage(CaptureSemantic, CaptureTerminal),
				ProfileIdentity{},
			)
			require.NoError(t, err)
			// Act.
			err = session.Emit(context.Background(), CaptureRecord{Stage: CaptureTerminal,
				Projection: CaptureProjection{Metadata: test.metadata}})
			// Assert.
			require.ErrorIs(t, err, ErrCapture)
			require.Zero(t, writes)
			require.False(t, session.Report().Complete)
		})
	}
}
