package openai

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type responseCapture struct {
	session    *prompty.CaptureSession
	operation  string
	dispatched bool
	source     *responseSource
	sseSource  *responseSSESource
	report     prompty.ExecutionDiagnostics
}

func newResponseCapture(
	operation string,
	report prompty.ExecutionDiagnostics,
	requirement prompty.CaptureRequirement,
	binding *prompty.CaptureBinding,
) (*responseCapture, error) {
	coverage := prompty.CaptureCoverage{SDKBuild: adapter.DependencyBuildIdentity("github.com/openai/openai-go/v3"),
		Stages: make(map[prompty.CaptureStage]prompty.CaptureAccuracy)}
	for _, stage := range []prompty.CaptureStage{prompty.CaptureExecution, prompty.CaptureTranslatedRequest,
		prompty.CaptureProviderResponse, prompty.CaptureCanonicalParts, prompty.CaptureTerminal} {
		coverage.Stages[stage] = prompty.CaptureSemantic
	}
	session, err := prompty.NewCaptureSession(
		requirement,
		prompty.CloneCaptureBinding(binding),
		coverage,
		report.Profile,
	)
	if err != nil {
		return nil, err
	}
	return &responseCapture{
		session:   session,
		operation: operation,
		report:    prompty.CloneExecutionDiagnostics(report),
	}, nil
}

func (c *responseCapture) emit(ctx context.Context, stage prompty.CaptureStage, value any) error {
	if !c.session.Enabled() {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return c.session.RecordFailure(stage, c.dispatched, "encoding", err)
	}
	var record prompty.CaptureRecord
	record.Stage, record.AfterDispatch = stage, c.dispatched
	var metadata prompty.CaptureRecordMetadata
	metadata.SchemaDigests = prompty.CaptureSchemaDigests(c.report.Schemas)
	if stage == prompty.CaptureProviderResponse && c.source != nil && c.source.status > 0 {
		status := c.source.status
		metadata.HTTPStatus = &status
	}
	if stage == prompty.CaptureProviderResponse && c.sseSource != nil && c.sseSource.status > 0 {
		status := c.sseSource.status
		metadata.HTTPStatus = &status
	}
	record.Projection = prompty.CaptureProjection{Payload: data, Accuracy: prompty.CaptureSemantic, Complete: true,
		Codec: "openai.responses.operation", MediaType: "application/json",
		Metadata: metadata}
	if stage == prompty.CaptureProviderResponse && c.sseSource != nil {
		record.Projection.Codec = "openai.responses.sse.event"
	}
	return c.session.Emit(ctx, record)
}

func (c *responseCapture) before(ctx context.Context, canonical, translated any) error {
	if err := c.emit(ctx, prompty.CaptureExecution, canonical); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.emit(ctx, prompty.CaptureTranslatedRequest, translated)
}

func (c *responseCapture) finish(
	ctx context.Context,
	result *prompty.RemoteResponseResult,
	operationErr error,
) (*prompty.RemoteResponseResult, error) {
	if !c.session.Enabled() {
		return result, operationErr
	}
	sourceErr := c.captureSource(ctx)
	var wire *prompty.ResponseWire
	var wireErr error
	if result.Response != nil {
		var encoded prompty.ResponseWire
		encoded, wireErr = prompty.MarshalResponse(result.Response, prompty.WirePolicy{State: prompty.StatePreserve})
		if wireErr == nil {
			wire = &encoded
		} else {
			wireErr = c.session.RecordFailure(prompty.CaptureCanonicalParts, c.dispatched, "encoding", wireErr)
		}
	}
	kind := ""
	if operationErr != nil {
		kind = "operation_failed"
	}
	if errors.Is(operationErr, prompty.ErrRemoteResponseUnknown) {
		kind = "remote_outcome_unknown"
	}
	canonical := struct {
		Operation string                        `json:"operation"`
		Handle    *prompty.RemoteResponseHandle `json:"handle,omitempty"`
		Response  *prompty.ResponseWire         `json:"response,omitempty"`
		ErrorKind string                        `json:"error_kind,omitempty"`
	}{Operation: c.operation, Handle: result.Handle, Response: wire, ErrorKind: kind}
	partsErr := c.emit(ctx, prompty.CaptureCanonicalParts, canonical)
	if operationErr == nil && (sourceErr != nil || partsErr != nil || wireErr != nil) {
		canonical.ErrorKind = "capture"
	}
	terminalErr := c.emit(ctx, prompty.CaptureTerminal, canonical)
	report := c.session.Report()
	result.Diagnostics.Capture = &report
	if result.Response != nil {
		result.Response.Diagnostics = prompty.CloneExecutionDiagnostics(result.Diagnostics)
	}
	return result, errors.Join(operationErr, sourceErr, partsErr, terminalErr, wireErr)
}

func (c *responseCapture) captureSource(ctx context.Context) error {
	if !c.dispatched {
		return nil
	}
	if c.sseSource != nil && c.sseSource.status > 0 {
		data, err := json.Marshal(struct {
			Events   int  `json:"events_observed"`
			Complete bool `json:"stream_complete"`
		}{Events: c.sseSource.events, Complete: c.sseSource.complete})
		if err != nil {
			return c.session.RecordFailure(prompty.CaptureProviderResponse, true, "encoding", err)
		}
		status := c.sseSource.status
		return c.session.Emit(ctx, prompty.CaptureRecord{
			Stage: prompty.CaptureProviderResponse, AfterDispatch: true,
			Projection: prompty.CaptureProjection{Payload: data, Accuracy: prompty.CaptureSemantic,
				Complete: c.sseSource.complete, Codec: "openai.responses.sse.end", MediaType: "application/json",
				Metadata: prompty.CaptureRecordMetadata{HTTPStatus: &status,
					SchemaDigests: prompty.CaptureSchemaDigests(c.report.Schemas)}},
		})
	}
	if c.source != nil && c.source.complete && json.Valid(c.source.data) {
		return c.emit(ctx, prompty.CaptureProviderResponse, json.RawMessage(c.source.data))
	}
	if c.source == nil || c.source.status <= 0 {
		return c.session.RecordFailure(
			prompty.CaptureProviderResponse,
			true,
			"source_unavailable",
			prompty.ErrCaptureUnsupported,
		)
	}
	status := c.source.status
	var record prompty.CaptureRecord
	record.Stage, record.AfterDispatch = prompty.CaptureProviderResponse, true
	record.Projection = prompty.CaptureProjection{Accuracy: prompty.CaptureMetadata, Complete: false,
		Metadata: prompty.CaptureRecordMetadata{
			HTTPStatus:    &status,
			SchemaDigests: prompty.CaptureSchemaDigests(c.report.Schemas),
		}}
	return c.session.Emit(ctx, record)
}
