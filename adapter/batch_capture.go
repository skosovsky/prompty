package adapter

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/skosovsky/prompty"
)

// BatchCapture is a semantic operation recorder shared by optional batch ports.
// It never claims HTTP bytes, retries or physical-attempt coverage.
type BatchCapture struct {
	session    *prompty.CaptureSession
	operation  string
	dispatched bool
}

func NewBatchCapture(
	operation, dependency string,
	profile prompty.ProfileIdentity,
	options prompty.BatchOperationOptions,
) (*BatchCapture, error) {
	coverage := prompty.CaptureCoverage{
		SDKBuild: DependencyBuildIdentity(dependency), PhysicalAttempts: false,
		Stages: make(map[prompty.CaptureStage]prompty.CaptureAccuracy),
	}
	for _, stage := range []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest,
		prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal} {
		coverage.Stages[stage] = prompty.CaptureSemantic
	}
	session, err := prompty.NewCaptureSession(
		options.Capture,
		prompty.CloneCaptureBinding(options.Binding),
		coverage,
		profile,
	)
	if err != nil {
		return nil, err
	}
	return &BatchCapture{session: session, operation: operation, dispatched: false}, nil
}

func (c *BatchCapture) MarkDispatched() { c.dispatched = true }

func (c *BatchCapture) Emit(ctx context.Context, stage prompty.CaptureStage, value any) error {
	if !c.session.Enabled() {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return c.session.RecordFailure(stage, c.dispatched, "encoding", err)
	}
	var projection prompty.CaptureProjection
	projection.Payload, projection.Complete, projection.Accuracy = data, true, prompty.CaptureSemantic
	projection.Codec, projection.MediaType = "prompty.batch.operation", captureJSONMediaType
	return c.emitProjection(ctx, stage, projection)
}

func (c *BatchCapture) emitProjection(
	ctx context.Context,
	stage prompty.CaptureStage,
	projection prompty.CaptureProjection,
) error {
	var record prompty.CaptureRecord
	record.Stage, record.AfterDispatch, record.Projection = stage, c.dispatched, projection
	return c.session.Emit(ctx, record)
}

func (c *BatchCapture) Before(ctx context.Context, canonical, translated any) error {
	if err := c.Emit(ctx, prompty.CaptureExecution, canonical); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Emit(ctx, prompty.CaptureTranslatedRequest, translated)
}

// Provider observes complete semantic JSON or explicitly unavailable source.
// HTTP status is included only when the transport actually observed it.
func (c *BatchCapture) Provider(ctx context.Context, payload prompty.JSONDocument, status int, complete bool) error {
	if !c.session.Enabled() {
		return nil
	}
	var metadata prompty.CaptureRecordMetadata
	if status > 0 {
		metadata.HTTPStatus = &status
	}
	var projection prompty.CaptureProjection
	projection.Metadata, projection.Accuracy = metadata, prompty.CaptureMetadata
	if json.Valid(payload) {
		projection.Payload, projection.Accuracy, projection.Complete = prompty.CloneJSONDocument(
			payload,
		), prompty.CaptureSemantic, complete
		projection.Codec, projection.MediaType = "prompty.batch.provider", captureJSONMediaType
	}
	if status <= 0 && !json.Valid(payload) {
		return c.session.RecordFailure(
			prompty.CaptureProviderResponse,
			c.dispatched,
			"source_unavailable",
			prompty.ErrCaptureUnsupported,
		)
	}
	return c.emitProjection(ctx, prompty.CaptureProviderResponse, projection)
}

func (c *BatchCapture) Finish(
	ctx context.Context,
	result *prompty.BatchResult,
	operationErr error,
) (*prompty.BatchResult, error) {
	if !c.session.Enabled() {
		return result, operationErr
	}
	type itemWire struct {
		ID         string                    `json:"id"`
		Binding    *prompty.BatchItemBinding `json:"binding,omitempty"`
		Outcome    prompty.BatchItemOutcome  `json:"outcome"`
		Response   *prompty.ResponseWire     `json:"response,omitempty"`
		SourceJSON prompty.JSONDocument      `json:"source_json,omitempty"`
		ErrorKind  string                    `json:"error_kind,omitempty"`
	}
	items := make([]itemWire, 0, len(result.Items))
	var wireErr error
	for _, item := range result.Items {
		projected := itemWire{
			ID:         item.ID,
			Binding:    item.Binding,
			Outcome:    item.Outcome,
			SourceJSON: item.SourceJSON,
			ErrorKind:  item.ErrorKind,
			Response:   nil,
		}
		if item.Response != nil {
			wire, err := prompty.MarshalResponse(item.Response, prompty.WirePolicy{State: prompty.StatePreserve})
			if err != nil {
				wireErr = errors.Join(
					wireErr,
					c.session.RecordFailure(prompty.CaptureCanonicalParts, c.dispatched, "encoding", err),
				)
			} else {
				projected.Response = &wire
			}
		}
		items = append(items, projected)
	}
	canonical := struct {
		Operation           string                     `json:"operation"`
		Handle              *prompty.BatchHandle       `json:"handle,omitempty"`
		Items               []itemWire                 `json:"items"`
		Completeness        prompty.BatchCompleteness  `json:"completeness"`
		ErrorKind           string                     `json:"error_kind,omitempty"`
		Resources           []prompty.BatchResourceRef `json:"resources,omitempty"`
		SubmissionAttempted bool                       `json:"submission_attempted"`
	}{Operation: c.operation, Handle: result.Handle, Items: items, Completeness: result.Completeness, ErrorKind: "", Resources: result.Resources, SubmissionAttempted: result.SubmissionAttempted}
	if operationErr != nil {
		canonical.ErrorKind = "operation_failed"
	}
	if errors.Is(operationErr, prompty.ErrBatchUnknown) {
		canonical.ErrorKind = "remote_outcome_unknown"
	}
	partsErr := c.Emit(ctx, prompty.CaptureCanonicalParts, canonical)
	if operationErr == nil && (wireErr != nil || partsErr != nil) {
		canonical.ErrorKind = captureErrorRecording
	}
	terminalErr := c.Emit(ctx, prompty.CaptureTerminal, canonical)
	report := c.session.Report()
	result.Capture = &report
	return result, errors.Join(operationErr, wireErr, partsErr, terminalErr)
}
