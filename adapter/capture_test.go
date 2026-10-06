package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestCaptureStreamValidationClassification(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name string
		err  error
		kind string
	}{
		{"domain", &prompty.ValidationError{RawAssistantMessage: nil, FeedbackPrompt: "",
			Err: errors.New("caller-owned domain rejection")}, "validation"},
		{"schema", &prompty.ValidationError{RawAssistantMessage: nil, FeedbackPrompt: "",
			Err: prompty.ErrSchemaMismatch}, "schema_validation"},
		{"parse", fmt.Errorf("fixture: %w", prompty.ErrStructuredStream), "parse"},
		{"usage", fmt.Errorf("fixture: %w", prompty.ErrUsage), "parse"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: safe categories must not disclose the underlying caller error.
			err := fixture.err
			// Act.
			kind := captureStreamErrorKind(err)
			// Assert.
			require.Equal(t, fixture.kind, kind)
		})
	}
}

type captureBindingFixture struct {
	records  *[]prompty.CaptureRecord
	onRecord func(prompty.CaptureRecord)
}

func TestCanonicalCaptureStateUpdateEventSemantics(t *testing.T) {
	t.Parallel()
	// Arrange: a state update and ordinary state on different parts in one frame.
	var records []prompty.CaptureRecord
	ports := captureBindingFixture{records: &records}
	var coverage prompty.CaptureCoverage
	coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
	coverage.Stages[prompty.CaptureCanonicalParts] = prompty.CaptureSemantic
	session, err := prompty.NewCaptureSession(prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic},
		&prompty.CaptureBinding{Sink: ports, Projector: ports}, coverage, prompty.ProfileIdentity{})
	require.NoError(t, err)
	envelope := prompty.ProviderEnvelope{Format: prompty.ProviderStateFormat,
		Scope: prompty.ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"},
		Codec: "fixture", Payload: []byte("opaque"), Required: true}
	frame := &prompty.ResponseChunk{
		Kind:     prompty.StreamDelta,
		Identity: prompty.StreamIdentity{PartIDs: []string{"text", "tool"}},
		Content: []prompty.ContentPart{
			prompty.TextPart{Text: "text"},
			prompty.ToolCallPart{ID: "call", Name: "lookup"},
		},
		ProviderState: []prompty.PartState{{PartIndex: 0, Envelope: envelope}},
		StateUpdates:  []prompty.PartState{{PartIndex: 1, Envelope: envelope}},
	}
	// Act: canonical frame capture remains portable without flattening update intent.
	err = captureCanonicalChunk(t.Context(), session, frame)
	// Assert: updates are projected onto current content but identified separately.
	require.NoError(t, err)
	require.Len(t, records, 1)
	var record struct {
		Partial          *prompty.PromptExecutionWire `json:"partial"`
		StateUpdateParts []int                        `json:"state_update_parts"`
	}
	require.NoError(t, json.Unmarshal(records[0].Projection.Payload, &record))
	require.Equal(t, []int{1}, record.StateUpdateParts)
	require.NotNil(t, record.Partial)
	require.Equal(
		t,
		[]prompty.PartState{{PartIndex: 0, Envelope: envelope}, {PartIndex: 1, Envelope: envelope}},
		record.Partial.Messages[0].ProviderState,
	)
	require.True(t, record.Partial.Messages[0].ContinuationUnavailable)
	require.Len(t, frame.ProviderState, 1)
	require.Len(t, frame.StateUpdates, 1)
}

func TestCanonicalCapturePreservesMessageOnlyAnnotations(t *testing.T) {
	t.Parallel()
	for _, outcome := range []prompty.Outcome{"", prompty.OutcomeCompleted} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			// Arrange: a metadata-only frame with unknown or known provider outcome.
			var records []prompty.CaptureRecord
			ports := captureBindingFixture{records: &records}
			var coverage prompty.CaptureCoverage
			coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
			coverage.Stages[prompty.CaptureCanonicalParts] = prompty.CaptureSemantic
			session, err := prompty.NewCaptureSession(prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic},
				&prompty.CaptureBinding{Sink: ports, Projector: ports}, coverage, prompty.ProfileIdentity{})
			require.NoError(t, err)
			annotations := []prompty.MessageAnnotation{
				{Kind: "provider.grounding", Value: prompty.JSONDocument(`{"score":0.1234567890123456789}`)},
			}
			frame := &prompty.ResponseChunk{
				Kind:               prompty.StreamUsage,
				Outcome:            outcome,
				MessageAnnotations: annotations,
			}
			// Act.
			err = captureCanonicalChunk(t.Context(), session, frame)
			// Assert: both full response and provisional history keep exact values.
			require.NoError(t, err)
			require.Len(t, records, 1)
			var captured struct {
				Response *prompty.ResponseWire        `json:"response"`
				Partial  *prompty.PromptExecutionWire `json:"partial"`
			}
			require.NoError(t, json.Unmarshal(records[0].Projection.Payload, &captured))
			if outcome == "" {
				require.NotNil(t, captured.Partial)
				require.Equal(t, annotations, captured.Partial.Messages[0].MessageAnnotations)
				require.Empty(t, captured.Partial.Messages[0].Content)
			} else {
				require.NotNil(t, captured.Response)
				require.Equal(t, annotations, captured.Response.MessageAnnotations)
				require.Empty(t, captured.Response.Content)
			}
		})
	}
}

func TestFailedCaptureTerminalRetainsPreparedSchemaDigests(t *testing.T) {
	t.Parallel()
	// Arrange: the response is unknown after failure, but compilation identity is known.
	var records []prompty.CaptureRecord
	ports := captureBindingFixture{records: &records}
	var coverage prompty.CaptureCoverage
	coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
	coverage.Stages[prompty.CaptureTerminal] = prompty.CaptureSemantic
	session, err := prompty.NewCaptureSession(prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic},
		&prompty.CaptureBinding{Sink: ports, Projector: ports}, coverage, prompty.ProfileIdentity{})
	require.NoError(t, err)
	var report prompty.SchemaCompilationReport
	report.Role, report.SourceDigest, report.TranslatedDigest = prompty.SchemaOutput, "source", "translated"
	// Act.
	err = captureCanonical(context.Background(), session, prompty.CaptureTerminal, nil, "transport",
		[]prompty.SchemaCompilationReport{report})
	// Assert: do not invent outcome/usage for unknown execution.
	require.NoError(t, err)
	require.Len(t, records, 1)
	require.Equal(t, prompty.CaptureSchemaDigests([]prompty.SchemaCompilationReport{report}),
		records[0].Projection.Metadata.SchemaDigests)
	require.Nil(t, records[0].Projection.Metadata.Usage)
	require.Empty(t, records[0].Projection.Metadata.Outcome)
}

func (captureBindingFixture) Capabilities() prompty.CaptureSinkCapabilities {
	return prompty.CaptureSinkCapabilities{}
}

func (f captureBindingFixture) WriteCapture(
	_ context.Context, record prompty.CaptureRecord,
) (prompty.CaptureAck, error) {
	*f.records = append(*f.records, record)
	if f.onRecord != nil {
		f.onRecord(record)
	}
	return prompty.CaptureAck{}, nil
}

func (captureBindingFixture) MaxAccuracy() prompty.CaptureAccuracy { return prompty.CaptureSemantic }

func (captureBindingFixture) ProjectCapture(
	_ context.Context, record prompty.CaptureRecord,
) (prompty.CaptureProjection, error) {
	return record.Projection, nil
}

func TestCanonicalCaptureKeepsOriginalSchemaBeforeLowering(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &loweringFixture{checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
		response: prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "1"}})}
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`)}
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode: prompty.SchemaLowering, AllowedTransforms: []string{"fixture.drop-minimum"},
	}
	var records []prompty.CaptureRecord
	ports := captureBindingFixture{records: &records}
	exec.Capture = &prompty.CaptureBinding{Sink: ports, Projector: ports}
	exec.Requirements.Capture = prompty.CaptureRequirement{Accuracy: prompty.CaptureSemantic}
	// Act.
	response, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
	require.Equal(t, 1, fixture.calls)
	require.Len(t, records, 5)
	require.Len(t, response.Diagnostics.Schemas, 1)
	for _, record := range records {
		require.Equal(
			t,
			prompty.CaptureSchemaDigests(response.Diagnostics.Schemas),
			record.Projection.Metadata.SchemaDigests,
		)
	}
	require.NotEqual(t, records[0].Projection.Metadata.SchemaDigests[0].SourceDigest,
		records[0].Projection.Metadata.SchemaDigests[0].TranslatedDigest)
	var original, translated, terminal map[string]any
	require.NoError(t, json.Unmarshal(records[0].Projection.Payload, &original))
	require.NoError(t, json.Unmarshal(records[1].Projection.Payload, &translated))
	require.NoError(t, json.Unmarshal(records[4].Projection.Payload, &terminal))
	sourceSchema := original["response_format"].(map[string]any)["schema"].(map[string]any)
	nativeSchema := translated["response_format"].(map[string]any)["schema"].(map[string]any)
	require.InDelta(t, 10, sourceSchema["minimum"], 0)
	require.NotContains(t, nativeSchema, "minimum")
	require.Equal(t, "validation", terminal["error_kind"])
	require.True(t, response.Diagnostics.Capture.Complete)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
}

func TestCaptureRequiredRuntimePortsMustBeReboundAfterWireRestore(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	exec := prompty.SimplePrompt("fixture")
	exec.Requirements.Capture.Stages = []prompty.CaptureStage{prompty.CaptureExecution}
	wire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalExecution(wire)
	require.NoError(t, err)
	// Act.
	_, err = NewClient(fixture).Execute(context.Background(), restored)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}

func TestCaptureSinkCancellationBeforeDispatchDoesNotExecute(t *testing.T) {
	t.Parallel()
	// Arrange: a sink acknowledges its record but cancels the invocation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	var records []prompty.CaptureRecord
	ports := captureBindingFixture{records: &records, onRecord: func(record prompty.CaptureRecord) {
		if record.Stage == prompty.CaptureTranslatedRequest {
			cancel()
		}
	}}
	exec := prompty.SimplePrompt("fixture")
	exec.Capture = &prompty.CaptureBinding{Sink: ports}
	// Act.
	_, err := NewClient(fixture).Execute(ctx, exec)
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, fixture.calls)
	require.Len(t, records, 2)
}
