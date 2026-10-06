package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestStreamRejectsContradictoryCaptureObserverReport(t *testing.T) {
	t.Parallel()
	// Arrange: a trusted port still must obey the report contract.
	sourceErr := errors.New("provider disconnected")
	stream := NewStream(context.Background(), StreamNative, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) {
			if !yield(
				&ResponseChunk{
					Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
					Kind:     StreamDelta,
					Content:  []ContentPart{TextPart{Text: "known"}},
				},
				nil,
			) {
				return
			}
			yield(nil, sourceErr)
		}
	}, WithStreamCaptureObserver(func(context.Context, StreamStatus, error) (*CaptureReport, error) {
		report := validFixtureCaptureReport()
		report.Observed = nil // closed/complete claim contradicts acknowledged records
		return &report, nil
	}))
	// Act.
	terminals := 0
	for event := range stream.Events() {
		if event.Kind == StreamTerminal {
			terminals++
		}
	}
	// Assert: retain the execution error/content, do not install false persistence evidence.
	status := stream.Status()
	require.Equal(t, 1, terminals)
	require.Equal(t, StreamFailed, status.State)
	require.ErrorIs(t, status.Err, sourceErr)
	require.ErrorIs(t, status.Err, ErrCaptureReport)
	require.Equal(t, TextPart{Text: "known"}, status.Result.Content[0])
	require.Nil(t, status.Result.Diagnostics.Capture)
}

func TestInvalidCaptureHookInputDoesNotCorruptReport(t *testing.T) {
	t.Parallel()
	// Arrange.
	writes := 0
	session, err := NewCaptureSession(CaptureRequirement{}, &CaptureBinding{
		Sink: fixtureCaptureSink{write: func(context.Context, CaptureRecord) (CaptureAck, error) {
			writes++
			return CaptureAck{}, nil
		}},
	}, fixtureCaptureCoverage(CaptureMetadata, CaptureExecution), ProfileIdentity{})
	require.NoError(t, err)
	before := session.Report()
	// Act.
	emitErr := session.Emit(context.Background(), CaptureRecord{Stage: "invented"})
	stageErr := session.RecordFailure("invented", false, "encoding", ErrCaptureUnsupported)
	kindErr := session.RecordFailure(CaptureExecution, false, " \t", ErrCaptureUnsupported)
	// Assert: invalid caller input is rejected without inventing a valid-stage event.
	require.ErrorIs(t, emitErr, ErrCaptureUnsupported)
	require.ErrorIs(t, stageErr, ErrCaptureUnsupported)
	require.ErrorIs(t, kindErr, ErrCaptureUnsupported)
	require.Zero(t, writes)
	require.Equal(t, before, session.Report())
	require.NoError(t, session.Report().Validate())
}

func validFixtureCaptureReport() CaptureReport {
	return CaptureReport{InvocationID: "fixture", Closed: true, Complete: true,
		Coverage: []CaptureStageCoverage{
			{Stage: CaptureExecution, Accuracy: CaptureSemantic}, {Stage: CaptureTerminal, Accuracy: CaptureMetadata},
		}, Observed: []CaptureStage{CaptureExecution, CaptureTerminal}}
}

func TestCaptureReportContradictionsCannotPersist(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*CaptureReport)
	}{
		{"blank identity", func(r *CaptureReport) { r.InvocationID = " \t" }},
		{"closed without terminal", func(r *CaptureReport) { r.Observed = []CaptureStage{CaptureExecution} }},
		{"terminal but open", func(r *CaptureReport) { r.Closed, r.Complete = false, false }},
		{"complete but open", func(r *CaptureReport) { r.Closed, r.Observed = false, nil }},
		{"complete with failure", func(r *CaptureReport) {
			r.Failures = []CaptureFailure{{Stage: CaptureTerminal, Kind: "sink"}}
		}},
		{"duplicate coverage", func(r *CaptureReport) { r.Coverage = append(r.Coverage, r.Coverage[0]) }},
		{"duplicate stage different accuracy", func(r *CaptureReport) {
			r.Coverage = append(r.Coverage, CaptureStageCoverage{Stage: CaptureExecution, Accuracy: CaptureBytes})
		}},
		{"invalid accuracy", func(r *CaptureReport) { r.Coverage[0].Accuracy = "invented" }},
		{"unknown coverage", func(r *CaptureReport) { r.Coverage[0].Stage = "invented" }},
		{"uncovered observed", func(r *CaptureReport) { r.Observed[0] = CaptureProviderResponse }},
		{"duplicate observed", func(r *CaptureReport) { r.Observed = append(r.Observed, CaptureExecution) }},
		{"unknown failure stage", func(r *CaptureReport) {
			r.Complete = false
			r.Failures = []CaptureFailure{{Stage: "invented", Kind: "sink"}}
		}},
		{"blank failure classification", func(r *CaptureReport) {
			r.Complete = false
			r.Failures = []CaptureFailure{{Stage: CaptureTerminal, Kind: " \t"}}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: construct structurally typed but semantically contradictory metadata.
			report := validFixtureCaptureReport()
			test.mutate(&report)
			response := NewResponse([]ContentPart{TextPart{Text: "known result"}})
			wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
			require.NoError(t, err)
			wire.Diagnostics.Capture = &report
			response.Diagnostics.Capture = &report
			// Act.
			_, marshalErr := MarshalResponse(response, WirePolicy{State: StatePreserve})
			_, restoreErr := UnmarshalResponse(wire)
			data, encodeErr := json.Marshal(wire)
			require.NoError(t, encodeErr)
			_, decodeErr := DecodeResponseJSON(data)
			// Assert: JSON may reject structure first; typed APIs retain the semantic sentinel.
			require.ErrorIs(t, marshalErr, ErrCaptureReport)
			require.ErrorIs(t, restoreErr, ErrCaptureReport)
			require.Error(t, decodeErr)
		})
	}
}

func TestCaptureReportIncompleteSnapshotsRemainPersistable(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*CaptureReport)
	}{
		{"complete", func(*CaptureReport) {}},
		{"pending", func(r *CaptureReport) {
			r.Closed, r.Complete, r.Observed = false, false, []CaptureStage{CaptureExecution}
		}},
		{"redacted terminal", func(r *CaptureReport) { r.Complete = false }},
		{"failed unavailable stage", func(r *CaptureReport) {
			r.Complete = false
			r.Failures = []CaptureFailure{{Stage: CaptureTransportResponse, Kind: "coverage", AfterDispatch: true}}
		}},
		{"crash before observations", func(r *CaptureReport) {
			r.Closed, r.Complete, r.Observed = false, false, nil
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			report := validFixtureCaptureReport()
			test.mutate(&report)
			response := NewResponse(nil)
			response.Diagnostics.Capture = &report
			// Act.
			wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
			require.NoError(t, err)
			data, err := json.Marshal(wire)
			require.NoError(t, err)
			restored, err := DecodeResponseJSON(data)
			// Assert: incomplete is not corrupt and unavailable coverage is a valid failed attempt.
			require.NoError(t, err)
			require.Equal(t, &report, restored.Diagnostics.Capture)
		})
	}
}
