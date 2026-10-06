package adapter

import (
	"context"
	"encoding/json"
	"errors"
	"runtime/debug"

	"github.com/skosovsky/prompty"
)

const (
	captureErrorValidation = "validation"
	captureErrorParse      = "parse"
	captureErrorRecording  = "capture"
	captureJSONMediaType   = "application/json"
)

// CaptureDescriber declares actual adapter hooks, not provider-wide support.
// Generic canonical/translated stages are added by the checked executor.
type CaptureDescriber interface {
	CaptureCoverage(prompty.StreamMode) prompty.CaptureCoverage
}

// NativeRequestPreparer applies pure SDK stream options before translated capture.
// It must not dispatch, mutate caller-owned data or substitute buffered execution.
type NativeRequestPreparer[Req any] interface {
	PrepareNativeRequest(Req) (Req, error)
}

// ProviderCaptureMetadata observes SDK fields without a second parse/dispatch.
// HTTP fields must remain absent unless the implementation actually sees them.
type ProviderCaptureMetadata[Resp any] interface {
	CaptureResponseMetadata(Resp) prompty.CaptureRecordMetadata
}

func captureStreamErrorKind(err error) string {
	var validationErr *prompty.ValidationError
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return "canceled"
	case errors.Is(err, prompty.ErrCapture):
		return captureErrorRecording
	case errors.Is(err, prompty.ErrResponseOutcome):
		return "provider_outcome"
	case errors.Is(err, prompty.ErrSchemaJSONDecode):
		return captureErrorParse
	case errors.Is(err, prompty.ErrSchemaMismatch):
		return "schema_validation"
	case errors.As(err, &validationErr):
		return captureErrorValidation
	case errors.Is(err, prompty.ErrStreamProtocol):
		return "protocol"
	case errors.Is(err, ErrInvalidResponse), errors.Is(err, ErrMalformedArgs),
		errors.Is(err, prompty.ErrUsage),
		errors.Is(err, prompty.ErrStructuredStream), errors.Is(err, prompty.ErrNonTextResponse),
		errors.Is(err, prompty.ErrEmptyTextResponse):
		return captureErrorParse
	default:
		return "transport"
	}
}

func prepareCapture(
	adp any,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
	profile prompty.ProfileIdentity,
) (*prompty.CaptureSession, error) {
	var coverage prompty.CaptureCoverage
	coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
	for _, stage := range []prompty.CaptureStage{
		prompty.CaptureExecution, prompty.CaptureTranslatedRequest,
		prompty.CaptureCanonicalParts, prompty.CaptureTerminal,
	} {
		coverage.Stages[stage] = prompty.CaptureSemantic
	}
	if mode != prompty.StreamNative {
		coverage.Stages[prompty.CaptureProviderResponse] = prompty.CaptureSemantic
	}
	if describer, ok := adp.(CaptureDescriber); ok {
		description := describer.CaptureCoverage(mode)
		coverage.SDKBuild = description.SDKBuild
		if mode == prompty.StreamNative && description.Stages[prompty.CaptureProviderChunk] == prompty.CaptureSemantic {
			coverage.Stages[prompty.CaptureProviderChunk] = prompty.CaptureSemantic
		}
	}
	return prompty.NewCaptureSession(exec.Requirements.Capture, exec.Capture, coverage, profile)
}

type captureContextKey struct{}

type captureInvocation struct {
	session *prompty.CaptureSession
	digests []prompty.CaptureSchemaDigest
}

func captureContext(
	ctx context.Context,
	session *prompty.CaptureSession,
	reports []prompty.SchemaCompilationReport,
) context.Context {
	if !session.Enabled() {
		return ctx
	}
	return context.WithValue(ctx, captureContextKey{}, captureInvocation{
		session: session, digests: prompty.CaptureSchemaDigests(reports),
	})
}

// CaptureProviderChunk is the trusted adapter hook before canonical normalization.
// SDK JSON is semantic capture, not transport/SSE framing or a physical attempt.
func CaptureProviderChunk(ctx context.Context, payload string, metadata prompty.CaptureRecordMetadata) error {
	invocation, _ := ctx.Value(captureContextKey{}).(captureInvocation)
	metadata.SchemaDigests = invocation.digests
	return emitCaptureMetadata(
		ctx,
		invocation.session,
		prompty.CaptureProviderChunk,
		true,
		metadata,
		func() ([]byte, error) {
			return []byte(payload), nil
		},
	)
}

// DependencyBuildIdentity reports embedded build metadata; absence is explicit.
func DependencyBuildIdentity(path string) string {
	info, ok := debug.ReadBuildInfo()
	if ok {
		for _, dependency := range info.Deps {
			if dependency.Path == path {
				if dependency.Replace != nil {
					return dependency.Path + "@" + dependency.Version + " => " +
						dependency.Replace.Path + "@" + dependency.Replace.Version
				}
				return dependency.Path + "@" + dependency.Version
			}
		}
	}
	return path + "@unknown"
}

func emitCaptureMetadata(
	ctx context.Context, session *prompty.CaptureSession, stage prompty.CaptureStage,
	after bool, metadata prompty.CaptureRecordMetadata, payload func() ([]byte, error),
) error {
	if !session.Enabled() {
		return nil
	}
	data, err := payload()
	if err != nil {
		return session.RecordFailure(stage, after, "encoding", err)
	}
	var record prompty.CaptureRecord
	record.Stage, record.AfterDispatch = stage, after
	record.Projection.Metadata = prompty.CloneCaptureRecordMetadata(metadata)
	record.Projection.Payload, record.Projection.MediaType = data, captureJSONMediaType
	record.Projection.Codec, record.Projection.Accuracy = string(stage)+".json", prompty.CaptureSemantic
	record.Projection.Complete = true
	return session.Emit(ctx, record)
}

func captureBeforeDispatch[Req any](ctx context.Context, prepared *PreparedRequest[Req]) error {
	var metadata prompty.CaptureRecordMetadata
	metadata.SchemaDigests = prompty.CaptureSchemaDigests(prepared.diagnostics.Schemas)
	if err := emitCaptureMetadata(
		ctx,
		prepared.capture,
		prompty.CaptureExecution,
		false,
		metadata,
		func() ([]byte, error) {
			wire, wireErr := prompty.MarshalExecution(
				prepared.execution,
				prompty.WirePolicy{State: prompty.StatePreserve},
			)
			if wireErr != nil {
				return nil, wireErr
			}
			return json.Marshal(wire)
		},
	); err != nil {
		return err
	}
	return emitCaptureMetadata(
		ctx,
		prepared.capture,
		prompty.CaptureTranslatedRequest,
		false,
		metadata,
		func() ([]byte, error) {
			return json.Marshal(prepared.request)
		},
	)
}

func captureProviderResponse[Resp any](
	ctx context.Context, session *prompty.CaptureSession, adp any, raw Resp, reports []prompty.SchemaCompilationReport,
) error {
	if !session.Enabled() {
		return nil
	}
	var metadata prompty.CaptureRecordMetadata
	if observer, ok := adp.(ProviderCaptureMetadata[Resp]); ok {
		metadata = observer.CaptureResponseMetadata(raw)
	}
	metadata.SchemaDigests = prompty.CaptureSchemaDigests(reports)
	return emitCaptureMetadata(ctx, session, prompty.CaptureProviderResponse, true, metadata, func() ([]byte, error) {
		if response, ok := any(raw).(interface{ RawJSON() string }); ok && response.RawJSON() != "" {
			return []byte(response.RawJSON()), nil
		}
		return json.Marshal(raw)
	})
}

func captureCanonical(
	ctx context.Context,
	session *prompty.CaptureSession,
	stage prompty.CaptureStage,
	response *prompty.Response,
	errorKind string,
	reports []prompty.SchemaCompilationReport,
) error {
	return captureCanonicalAt(ctx, session, stage, response, errorKind, true, "", nil, reports)
}

// A buffered stream records its canonical terminal through the original handle,
// after identity validation; ordinary sync calls retain their own closeout.
func captureSyncTerminal[Req any](
	ctx context.Context, prepared *PreparedRequest[Req], response *prompty.Response, kind string, streaming bool,
) error {
	if streaming {
		return nil
	}
	return captureCanonicalAt(
		ctx,
		prepared.capture,
		prompty.CaptureTerminal,
		response,
		kind,
		prepared.dispatched,
		"",
		nil,
		prepared.diagnostics.Schemas,
	)
}

func captureCanonicalAt(
	ctx context.Context, session *prompty.CaptureSession, stage prompty.CaptureStage,
	response *prompty.Response, errorKind string, after bool, streamKind prompty.StreamEventKind,
	identity *prompty.StreamIdentity,
	reports []prompty.SchemaCompilationReport,
) error {
	return captureCanonicalAtWithUpdates(
		ctx,
		session,
		stage,
		response,
		errorKind,
		after,
		streamKind,
		identity,
		reports,
		nil,
	)
}

func captureCanonicalAtWithUpdates(
	ctx context.Context, session *prompty.CaptureSession, stage prompty.CaptureStage,
	response *prompty.Response, errorKind string, after bool, streamKind prompty.StreamEventKind,
	identity *prompty.StreamIdentity, reports []prompty.SchemaCompilationReport, stateUpdateParts []int,
) error {
	var metadata prompty.CaptureRecordMetadata
	metadata.SchemaDigests = prompty.CaptureSchemaDigests(reports)
	if response != nil {
		metadata.Usage, metadata.Outcome = &response.Usage, response.Outcome
	}
	return emitCaptureMetadata(ctx, session, stage, after, metadata, func() ([]byte, error) {
		var wire *prompty.ResponseWire
		var partial *prompty.PromptExecutionWire
		var partialUsage *prompty.Usage
		partialFinishReason := ""
		if response != nil && response.Outcome == "" {
			message, messageErr := response.AssistantMessage()
			if messageErr != nil {
				return nil, messageErr
			}
			message.ContinuationUnavailable = true
			value, wireErr := prompty.MarshalExecution(
				prompty.NewExecution([]prompty.ChatMessage{message}), prompty.WirePolicy{State: prompty.StatePreserve},
			)
			if wireErr != nil {
				return nil, wireErr
			}
			partial = &value
			usage := response.Usage
			partialUsage, partialFinishReason = &usage, response.FinishReason
		} else if response != nil {
			value, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
			if err != nil {
				return nil, err
			}
			wire = &value
		}
		return json.Marshal(struct {
			Response            *prompty.ResponseWire        `json:"response,omitempty"`
			Partial             *prompty.PromptExecutionWire `json:"partial,omitempty"`
			PartialUsage        *prompty.Usage               `json:"partial_usage,omitempty"`
			PartialFinishReason string                       `json:"partial_finish_reason,omitempty"`
			ErrorKind           string                       `json:"error_kind,omitempty"`
			StreamKind          prompty.StreamEventKind      `json:"stream_kind,omitempty"`
			StreamIdentity      *prompty.StreamIdentity      `json:"stream_identity,omitempty"`
			StateUpdateParts    []int                        `json:"state_update_parts,omitempty"`
		}{
			Response:            wire,
			Partial:             partial,
			PartialUsage:        partialUsage,
			PartialFinishReason: partialFinishReason,
			ErrorKind:           errorKind,
			StreamKind:          streamKind,
			StreamIdentity:      identity,
			StateUpdateParts:    stateUpdateParts,
		})
	})
}

func captureCanonicalChunk(ctx context.Context, session *prompty.CaptureSession, chunk *prompty.ResponseChunk) error {
	if !session.Enabled() {
		return nil
	}
	response := new(prompty.Response)
	response.Content, response.ProviderState, response.Annotations = chunk.Content, chunk.ProviderState, chunk.Annotations
	response.ProviderState = append(append([]prompty.PartState(nil), chunk.ProviderState...), chunk.StateUpdates...)
	response.MessageAnnotations = chunk.MessageAnnotations
	response.ContinuationUnavailable = chunk.ContinuationUnavailable
	response.Usage, response.Outcome, response.FinishReason = chunk.Usage, chunk.Outcome, chunk.FinishReason
	response.Diagnostics = chunk.Diagnostics
	updateParts := make([]int, 0, len(chunk.StateUpdates))
	for _, state := range chunk.StateUpdates {
		updateParts = append(updateParts, state.PartIndex)
	}
	return captureCanonicalAtWithUpdates(ctx, session, prompty.CaptureCanonicalParts, response, "", true, chunk.Kind,
		&chunk.Identity, chunk.Diagnostics.Schemas, updateParts)
}
