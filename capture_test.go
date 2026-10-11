package prompty

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

type fixtureCaptureSink struct {
	durable bool
	write   func(context.Context, CaptureRecord) (CaptureAck, error)
}

func (s fixtureCaptureSink) Capabilities() CaptureSinkCapabilities {
	return CaptureSinkCapabilities{Durable: s.durable}
}

func (s fixtureCaptureSink) WriteCapture(ctx context.Context, record CaptureRecord) (CaptureAck, error) {
	return s.write(ctx, record)
}

type fixtureCaptureProjector struct {
	accuracy CaptureAccuracy
	project  func(context.Context, CaptureRecord) (CaptureProjection, error)
}

func (p fixtureCaptureProjector) MaxAccuracy() CaptureAccuracy { return p.accuracy }

func (p fixtureCaptureProjector) ProjectCapture(ctx context.Context, record CaptureRecord) (CaptureProjection, error) {
	return p.project(ctx, record)
}

func semanticCaptureProjector() fixtureCaptureProjector {
	return fixtureCaptureProjector{accuracy: CaptureSemantic,
		project: func(_ context.Context, record CaptureRecord) (CaptureProjection, error) {
			return CaptureProjection{
				Payload: record.Projection.Payload, MediaType: captureJSONMediaType, Codec: "fixture.json",
				Accuracy: CaptureSemantic, Complete: true,
			}, nil
		}}
}

func fixtureCaptureCoverage(accuracy CaptureAccuracy, stages ...CaptureStage) CaptureCoverage {
	coverage := CaptureCoverage{}
	coverage.Stages = make(map[CaptureStage]CaptureAccuracy, len(stages))
	for _, stage := range stages {
		coverage.Stages[stage] = accuracy
	}
	return coverage
}

func TestCapturePreflightDoesNotInventCoverage(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		req     CaptureRequirement
		binding *CaptureBinding
	}{
		{"no sink", CaptureRequirement{Stages: []CaptureStage{CaptureExecution}}, nil},
		{"transport", CaptureRequirement{Stages: []CaptureStage{CaptureTransportRequest}}, &CaptureBinding{}},
		{"physical attempts", CaptureRequirement{PhysicalAttempts: true}, &CaptureBinding{}},
		{"durable", CaptureRequirement{Durable: true}, &CaptureBinding{}},
		{"bytes", CaptureRequirement{Stages: []CaptureStage{CaptureExecution}, Accuracy: CaptureBytes}, &CaptureBinding{}},
		{"no projector", CaptureRequirement{Accuracy: CaptureSemantic}, &CaptureBinding{}},
		{"unknown stage", CaptureRequirement{Stages: []CaptureStage{"invented"}}, &CaptureBinding{}},
		{"duplicate stage", CaptureRequirement{Stages: []CaptureStage{CaptureExecution, CaptureExecution}}, &CaptureBinding{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			writes := 0
			if test.binding != nil {
				test.binding.Sink = fixtureCaptureSink{write: func(context.Context, CaptureRecord) (CaptureAck, error) {
					writes++
					return CaptureAck{}, nil
				}}
			}
			coverage := fixtureCaptureCoverage(CaptureSemantic, CaptureExecution)
			// Act.
			_, err := NewCaptureSession(test.req, test.binding, coverage, ProfileIdentity{})
			// Assert.
			require.ErrorIs(t, err, ErrCaptureUnsupported)
			require.Zero(t, writes)
		})
	}
}

func TestCaptureAbsentBindingIsOptional(t *testing.T) {
	t.Parallel()
	// Arrange / Act.
	session, err := NewCaptureSession(CaptureRequirement{}, nil, CaptureCoverage{}, ProfileIdentity{})
	// Assert.
	require.NoError(t, err)
	require.Empty(t, session.Report())
	require.NoError(t, session.Emit(context.Background(), CaptureRecord{}))
}

func TestCapturePayloadSequenceAndSnapshotIsolation(t *testing.T) {
	t.Parallel()
	// Arrange: callbacks may inspect the report without a held state mutex.
	payload := []byte(`{"value":1}`)
	var received []CaptureRecord
	var session *CaptureSession
	binding := &CaptureBinding{Projector: semanticCaptureProjector(), CorrelationID: "caller-correlation"}
	binding.Sink = fixtureCaptureSink{write: func(_ context.Context, record CaptureRecord) (CaptureAck, error) {
		_ = session.Report()
		received = append(received, record)
		record.Projection.Payload[0] = 'x'
		return CaptureAck{}, nil
	}}
	stages := []CaptureStage{CaptureExecution, CaptureTerminal}
	coverage := fixtureCaptureCoverage(CaptureSemantic, CaptureExecution, CaptureTerminal)
	coverage.SDKBuild = "fixture-build"
	var err error
	session, err = NewCaptureSession(CaptureRequirement{Stages: stages}, binding, coverage,
		ProfileIdentity{Provider: "fixture", Endpoint: "https://user:secret@example.org?token=secret"})
	require.NoError(t, err)
	coverage.Stages[CaptureTerminal] = "invalid mutation"
	stages[1] = "invalid mutation"
	// Act.
	require.NoError(t, session.Emit(context.Background(), CaptureRecord{
		Stage: CaptureExecution, Projection: CaptureProjection{Payload: payload},
	}))
	require.False(t, session.Report().Complete)
	require.NoError(t, session.Emit(context.Background(), CaptureRecord{
		Stage: CaptureTerminal, AfterDispatch: true, Projection: CaptureProjection{Payload: payload},
	}))
	// Assert.
	require.JSONEq(t, `{"value":1}`, string(payload))
	require.Len(t, received, 2)
	require.Equal(t, received[0].InvocationID, received[1].InvocationID)
	require.NotEmpty(t, received[0].InvocationID)
	require.EqualValues(t, 1, received[0].Sequence)
	require.EqualValues(t, 2, received[1].Sequence)
	require.Empty(t, received[0].PhysicalAttemptID)
	require.Empty(t, received[0].Profile.Endpoint)
	require.Equal(t, "caller-correlation", received[0].CorrelationID)
	report := session.Report()
	require.True(t, report.Closed)
	require.True(t, report.Complete)
	report.Observed[0] = "mutated"
	require.Equal(t, CaptureExecution, session.Report().Observed[0])
	require.ErrorIs(t, session.Emit(context.Background(), CaptureRecord{Stage: CaptureExecution}), ErrCapture)
}

func TestCaptureFailurePreservesDispatchClassificationWithoutSecrets(t *testing.T) {
	t.Parallel()
	for _, after := range []bool{false, true} {
		// Arrange.
		sentinel := errors.New("sink failure with secret credential")
		sink := fixtureCaptureSink{write: func(context.Context, CaptureRecord) (CaptureAck, error) {
			return CaptureAck{}, sentinel
		}}
		session, err := NewCaptureSession(CaptureRequirement{Stages: []CaptureStage{CaptureExecution}},
			&CaptureBinding{Sink: sink}, fixtureCaptureCoverage(CaptureSemantic, CaptureExecution), ProfileIdentity{})
		require.NoError(t, err)
		// Act.
		err = session.Emit(context.Background(), CaptureRecord{Stage: CaptureExecution, AfterDispatch: after})
		// Assert.
		require.ErrorIs(t, err, sentinel)
		var failure *CaptureError
		require.ErrorAs(t, err, &failure)
		require.Equal(t, after, failure.AfterDispatch)
		require.NotContains(t, err.Error(), "secret")
		require.NotContains(t, session.Report().Failures[0].Kind, "secret")
	}
}

func TestCaptureAdvisoryFailureAndDefaultMetadataProjection(t *testing.T) {
	t.Parallel()
	// Arrange.
	var got CaptureRecord
	sink := fixtureCaptureSink{write: func(_ context.Context, record CaptureRecord) (CaptureAck, error) {
		got = record
		return CaptureAck{}, errors.New("advisory failure")
	}}
	session, err := NewCaptureSession(CaptureRequirement{}, &CaptureBinding{Sink: sink},
		fixtureCaptureCoverage(CaptureSemantic, CaptureProviderResponse), ProfileIdentity{})
	require.NoError(t, err)
	// Act.
	err = session.Emit(context.Background(), CaptureRecord{Stage: CaptureProviderResponse,
		Projection: CaptureProjection{Payload: []byte(`{"secret":"private output"}`)}})
	// Assert.
	require.NoError(t, err)
	require.Empty(t, got.Projection.Payload)
	require.True(t, got.Projection.Redacted)
	require.False(t, got.Projection.Complete)
	require.Len(t, session.Report().Failures, 1)
}

func TestCaptureRejectsFalseAckAccuracyAndOversizedProjection(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name       string
		projection CaptureProjection
		ack        CaptureAck
		durable    bool
	}{
		{"false durable ack", CaptureProjection{Accuracy: CaptureMetadata}, CaptureAck{}, true},
		{"unadvertised durable ack", CaptureProjection{Accuracy: CaptureMetadata}, CaptureAck{Durable: true}, false},
		{"redacted bytes", CaptureProjection{Accuracy: CaptureBytes, Redacted: true, Complete: true}, CaptureAck{}, false},
		{"invalid semantic JSON", CaptureProjection{Accuracy: CaptureSemantic, MediaType: captureJSONMediaType,
			Payload: []byte("invalid")}, CaptureAck{}, false},
		{"oversized", CaptureProjection{Accuracy: CaptureMetadata, Payload: []byte("too large")}, CaptureAck{}, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			binding := &CaptureBinding{MaxBytes: 8,
				Sink: fixtureCaptureSink{durable: test.durable,
					write: func(context.Context, CaptureRecord) (CaptureAck, error) { return test.ack, nil }},
				Projector: fixtureCaptureProjector{
					accuracy: CaptureBytes,
					project:  func(context.Context, CaptureRecord) (CaptureProjection, error) { return test.projection, nil },
				},
			}
			session, err := NewCaptureSession(
				CaptureRequirement{Stages: []CaptureStage{CaptureExecution}, Durable: test.durable},
				binding,
				fixtureCaptureCoverage(CaptureBytes, CaptureExecution),
				ProfileIdentity{},
			)
			require.NoError(t, err)
			// Act.
			err = session.Emit(context.Background(), CaptureRecord{Stage: CaptureExecution})
			// Assert.
			require.ErrorIs(t, err, ErrCapture)
			require.False(t, session.Report().Complete)
		})
	}
}

func TestCaptureTerminalCannotHideMissingRequiredStage(t *testing.T) {
	t.Parallel()
	// Arrange.
	sink := fixtureCaptureSink{
		write: func(context.Context, CaptureRecord) (CaptureAck, error) { return CaptureAck{}, nil },
	}
	session, err := NewCaptureSession(CaptureRequirement{Stages: []CaptureStage{CaptureProviderResponse}},
		&CaptureBinding{Sink: sink, Projector: semanticCaptureProjector()},
		fixtureCaptureCoverage(CaptureSemantic, CaptureProviderResponse, CaptureTerminal),
		ProfileIdentity{})
	require.NoError(t, err)
	// Act.
	err = session.Emit(context.Background(), CaptureRecord{Stage: CaptureTerminal, AfterDispatch: true,
		Projection: CaptureProjection{Payload: []byte(`{}`)}})
	// Assert.
	require.ErrorIs(t, err, ErrCapture)
	require.True(t, session.Report().Closed)
	require.False(t, session.Report().Complete)
	require.NoError(t, session.Report().Validate())
	require.Equal(t, "missing_stage", session.Report().Failures[0].Kind)
}

func TestCaptureSerializesConcurrentEmissionAndAllowsReportReads(t *testing.T) {
	t.Parallel()
	// Arrange.
	var sequence []uint64
	var session *CaptureSession
	sink := fixtureCaptureSink{write: func(_ context.Context, record CaptureRecord) (CaptureAck, error) {
		_ = session.Report()
		sequence = append(sequence, record.Sequence)
		return CaptureAck{}, nil
	}}
	var err error
	session, err = NewCaptureSession(CaptureRequirement{}, &CaptureBinding{Sink: sink},
		fixtureCaptureCoverage(CaptureSemantic, CaptureCanonicalParts), ProfileIdentity{})
	require.NoError(t, err)
	// Act.
	var workers sync.WaitGroup
	for range 20 {
		workers.Go(func() {
			require.NoError(t, session.Emit(context.Background(), CaptureRecord{Stage: CaptureCanonicalParts}))
		})
	}
	workers.Wait()
	// Assert.
	require.Len(t, sequence, 20)
	for index, value := range sequence {
		require.EqualValues(t, index+1, value)
	}
}
