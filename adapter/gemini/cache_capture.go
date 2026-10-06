package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"slices"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type cacheOperationCapture struct {
	session    *prompty.CaptureSession
	operation  string
	source     *responseSource
	dispatched bool
	report     prompty.ExecutionDiagnostics
}

func newCacheOperationCapture(
	operation string,
	report prompty.ExecutionDiagnostics,
	options ...prompty.CacheResourceOperationOptions,
) (*cacheOperationCapture, error) {
	if len(options) > 1 {
		return nil, prompty.ErrCaptureUnsupported
	}
	var selected prompty.CacheResourceOperationOptions
	if len(options) == 1 {
		selected = options[0]
	}
	selected.Capture.Stages = slices.Clone(selected.Capture.Stages)
	coverage := prompty.CaptureCoverage{
		Stages:   make(map[prompty.CaptureStage]prompty.CaptureAccuracy),
		SDKBuild: adapter.DependencyBuildIdentity("google.golang.org/genai"),
	}
	for _, stage := range []prompty.CaptureStage{
		prompty.CaptureExecution, prompty.CaptureTranslatedRequest, prompty.CaptureProviderResponse,
		prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
	} {
		coverage.Stages[stage] = prompty.CaptureSemantic
	}
	session, err := prompty.NewCaptureSession(
		selected.Capture,
		prompty.CloneCaptureBinding(selected.Binding),
		coverage,
		report.Profile,
	)
	if err != nil {
		return nil, err
	}
	return &cacheOperationCapture{
		session:   session,
		operation: operation,
		report:    prompty.CloneExecutionDiagnostics(report),
	}, nil
}

func (c *cacheOperationCapture) emit(ctx context.Context, stage prompty.CaptureStage, value any) error {
	if !c.session.Enabled() {
		return nil
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return c.session.RecordFailure(stage, c.dispatched, "encoding", err)
	}
	return c.emitJSON(ctx, stage, payload)
}

func (c *cacheOperationCapture) emitJSON(ctx context.Context, stage prompty.CaptureStage, payload []byte) error {
	if !c.session.Enabled() {
		return nil
	}
	var metadata prompty.CaptureRecordMetadata
	metadata.SchemaDigests = prompty.CaptureSchemaDigests(c.report.Schemas)
	if stage == prompty.CaptureProviderResponse && c.source != nil && c.source.status > 0 {
		status := c.source.status
		metadata.HTTPStatus = &status
	}
	var record prompty.CaptureRecord
	record.Stage, record.AfterDispatch = stage, c.dispatched
	record.Projection = prompty.CaptureProjection{Metadata: metadata, Payload: payload,
		MediaType: "application/json", Codec: "gemini." + c.operation + "." + string(stage) + ".json",
		Accuracy: prompty.CaptureSemantic, Complete: true}
	return c.session.Emit(ctx, record)
}

func (c *cacheOperationCapture) before(ctx context.Context, canonical, translated any) error {
	if err := c.emit(ctx, prompty.CaptureExecution, canonical); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.emit(ctx, prompty.CaptureTranslatedRequest, translated)
}

func (c *cacheOperationCapture) parsed(
	ctx context.Context,
	source *responseSource,
	handle prompty.CacheResourceHandle,
	result *prompty.CacheResourceResult,
) (*prompty.CacheResourceResult, error) {
	parsed, err := cacheResourceResult(source, handle, result)
	return c.finish(ctx, parsed, err)
}

func (c *cacheOperationCapture) finish(
	ctx context.Context,
	result *prompty.CacheResourceResult,
	operationErr error,
) (*prompty.CacheResourceResult, error) {
	if !c.session.Enabled() {
		return result, operationErr
	}
	var providerErr error
	if c.dispatched {
		if c.source == nil || c.source.overflow || !c.source.complete || !json.Valid(c.source.data.Bytes()) {
			if c.source != nil && c.source.status > 0 {
				providerErr = c.providerMetadata(ctx)
			} else {
				providerErr = c.session.RecordFailure(prompty.CaptureProviderResponse, true,
					"source_unavailable", prompty.ErrCaptureUnsupported)
			}
		} else {
			providerErr = c.emitJSON(ctx, prompty.CaptureProviderResponse, c.source.data.Bytes())
		}
	}
	canonical := struct {
		Operation string                       `json:"operation"`
		Result    *prompty.CacheResourceResult `json:"result,omitempty"`
		ErrorKind string                       `json:"error_kind,omitempty"`
	}{Operation: c.operation, Result: result, ErrorKind: cacheCaptureErrorKind(operationErr)}
	partsErr := c.emit(ctx, prompty.CaptureCanonicalParts, canonical)
	if operationErr == nil && (providerErr != nil || partsErr != nil) {
		canonical.ErrorKind = "capture"
	}
	terminalErr := c.emit(ctx, prompty.CaptureTerminal, canonical)
	if result == nil {
		result = &prompty.CacheResourceResult{Diagnostics: prompty.CloneExecutionDiagnostics(c.report)}
	}
	report := c.session.Report()
	result.Diagnostics.Capture = &report
	return result, errors.Join(operationErr, providerErr, partsErr, terminalErr)
}

func (c *cacheOperationCapture) providerMetadata(ctx context.Context) error {
	status := c.source.status
	var record prompty.CaptureRecord
	record.Stage, record.AfterDispatch = prompty.CaptureProviderResponse, true
	record.Projection = prompty.CaptureProjection{
		Metadata: prompty.CaptureRecordMetadata{HTTPStatus: &status,
			SchemaDigests: prompty.CaptureSchemaDigests(c.report.Schemas)},
		Accuracy: prompty.CaptureMetadata, Complete: false,
	}
	return c.session.Emit(ctx, record)
}

func cacheCaptureErrorKind(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, prompty.ErrCapture):
		return "capture"
	case errors.Is(err, prompty.ErrCacheResourceUnknown):
		return "remote_outcome_unknown"
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	default:
		return "validation"
	}
}
