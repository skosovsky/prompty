package adapter

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"github.com/skosovsky/prompty"
)

// clientImpl implements prompty.Invoker via ProviderAdapter.
type clientImpl[Req any, Resp any] struct {
	adapter ProviderAdapter[Req, Resp]
}

// Execute performs profile preflight → Translate → Execute → ParseResponse.
func (c *clientImpl[Req, Resp]) Execute(
	ctx context.Context,
	exec *prompty.PromptExecution,
) (*prompty.Response, error) {
	return c.execute(ctx, exec, nil)
}

func (c *clientImpl[Req, Resp]) execute(
	ctx context.Context, exec *prompty.PromptExecution, active **PreparedRequest[Req],
) (*prompty.Response, error) {
	prepared, err := Prepare(c.adapter, exec, "")
	if err != nil {
		return nil, err
	}
	if active != nil {
		*active = prepared
	}
	if contextErr := ctx.Err(); contextErr != nil {
		terminalErr := captureSyncTerminal(ctx, prepared, nil, "canceled", active != nil)
		return nil, errors.Join(contextErr, terminalErr)
	}
	if captureErr := captureBeforeDispatch(ctx, prepared); captureErr != nil {
		terminalErr := captureSyncTerminal(ctx, prepared, nil, captureErrorRecording, active != nil)
		return nil, errors.Join(captureErr, terminalErr)
	}
	if contextErr := ctx.Err(); contextErr != nil {
		terminalErr := captureSyncTerminal(ctx, prepared, nil, "canceled", active != nil)
		return nil, errors.Join(contextErr, terminalErr)
	}
	dispatchErr := validatePreparedDispatch(c.adapter, prepared)
	if dispatchErr != nil {
		terminalErr := captureSyncTerminal(ctx, prepared, nil, captureErrorValidation, active != nil)
		return nil, errors.Join(dispatchErr, terminalErr)
	}
	prepared.dispatched = true
	resp, err := c.adapter.Execute(ctx, prepared.request)
	if err != nil {
		captureErr := captureSyncTerminal(ctx, prepared, nil, captureStreamErrorKind(err), active != nil)
		return nil, errors.Join(err, captureErr)
	}
	captureErr := captureProviderResponse(ctx, prepared.capture, c.adapter, resp, prepared.diagnostics.Schemas)
	var result *prompty.Response
	if parser, ok := any(c.adapter).(ProfileResponseParser[Resp]); ok {
		result, err = parser.ParseResponseForProfile(resp, prepared.Diagnostics().Profile)
	} else {
		result, err = c.adapter.ParseResponse(resp)
	}
	if err != nil {
		return finishParseFailure(ctx, prepared, result, active != nil, errors.Join(err, captureErr))
	}
	if result == nil {
		terminalErr := captureSyncTerminal(ctx, prepared, nil, captureErrorParse, active != nil)
		return nil, errors.Join(ErrInvalidResponse, captureErr, terminalErr)
	}
	result.Diagnostics = prepared.Diagnostics()
	metadataErr := prompty.ValidateResponseMetadata(result)
	validationErr := metadataErr
	if metadataErr == nil {
		validationErr = prepared.schemas.validate(result)
	}
	if active != nil {
		return result, errors.Join(validationErr, captureErr)
	}
	partsErr := captureCanonical(
		ctx,
		prepared.capture,
		prompty.CaptureCanonicalParts,
		result,
		"",
		prepared.diagnostics.Schemas,
	)
	errorKind := ""
	if validationErr != nil {
		errorKind = captureErrorValidation
		if errors.Is(validationErr, prompty.ErrSchemaJSONDecode) {
			errorKind = captureErrorParse
		}
		if metadataErr != nil {
			errorKind = "metadata"
		}
	} else if captureErr != nil || partsErr != nil {
		errorKind = captureErrorRecording
	}
	terminalErr := captureCanonical(
		ctx,
		prepared.capture,
		prompty.CaptureTerminal,
		result,
		errorKind,
		prepared.diagnostics.Schemas,
	)
	result.Diagnostics = prepared.Diagnostics()
	return result, errors.Join(validationErr, captureErr, partsErr, terminalErr)
}

// finishParseFailure keeps the parser's known result and refreshes capture diagnostics
// after closeout. Buffered closeout belongs to the original stream handle instead.
func finishParseFailure[Req any](
	ctx context.Context, prepared *PreparedRequest[Req], result *prompty.Response, streaming bool, parseErr error,
) (*prompty.Response, error) {
	if result != nil {
		result.Diagnostics = prepared.Diagnostics()
	}
	terminalErr := captureSyncTerminal(ctx, prepared, result, captureErrorParse, streaming)
	if result != nil {
		result.Diagnostics = prepared.Diagnostics()
	}
	return result, errors.Join(parseErr, terminalErr)
}

// ExecuteStream constructs a lazy, single-use stream with an explicit mode.
func (c *clientImpl[Req, Resp]) ExecuteStream(
	ctx context.Context,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	// Snapshot at handle creation, while keeping all translation/dispatch lazy.
	exec = exec.Clone()
	var schemas preparedSchemas
	var activePrepared *PreparedRequest[Req]
	return prompty.NewStream(ctx, mode, func(streamCtx context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
		if mode == prompty.StreamNative {
			return c.nativeEvents(streamCtx, exec, &schemas, &activePrepared)
		}
		return c.bufferedEvents(streamCtx, exec, &activePrepared)
	}, prompty.WithStreamFinalizer(func(_ context.Context, response *prompty.Response) error {
		return schemas.validate(response)
	}), prompty.WithStreamFrameObserver(func(observerCtx context.Context, chunk *prompty.ResponseChunk) error {
		if activePrepared == nil || chunk.Kind == prompty.StreamStart {
			return nil
		}
		return captureCanonicalChunk(observerCtx, activePrepared.capture, chunk)
	}), prompty.WithStreamCaptureObserver(func(
		observerCtx context.Context, status prompty.StreamStatus, streamErr error,
	) (*prompty.CaptureReport, error) {
		if activePrepared == nil || !activePrepared.capture.Enabled() {
			return nil, nil
		}
		kind := captureStreamErrorKind(streamErr)
		captureErr := captureCanonicalAt(
			observerCtx,
			activePrepared.capture,
			prompty.CaptureTerminal,
			status.Result,
			kind,
			activePrepared.dispatched,
			prompty.StreamTerminal,
			&status.Identity,
			activePrepared.diagnostics.Schemas,
		)
		report := activePrepared.capture.Report()
		return &report, captureErr
	}))
}

func (c *clientImpl[Req, Resp]) nativeEvents(
	ctx context.Context, exec *prompty.PromptExecution, schemas *preparedSchemas, active **PreparedRequest[Req],
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		prepared, err := Prepare(c.adapter, exec, prompty.StreamNative)
		if err != nil {
			yield(nil, err)
			return
		}
		streamer, ok := any(c.adapter).(StreamerAdapter[Req])
		if !ok {
			yield(nil, ErrStreamNotImplemented)
			return
		}
		*schemas = prepared.schemas
		*active = prepared
		if captureErr := captureBeforeDispatch(ctx, prepared); captureErr != nil {
			yield(nil, captureErr)
			return
		}
		if !yield(&prompty.ResponseChunk{Kind: prompty.StreamStart, Diagnostics: prepared.Diagnostics()}, nil) {
			return
		}
		if contextErr := ctx.Err(); contextErr != nil {
			yield(nil, contextErr)
			return
		}
		if dispatchErr := validatePreparedDispatch(c.adapter, prepared); dispatchErr != nil {
			yield(nil, dispatchErr)
			return
		}
		prepared.dispatched = true
		forwardNative(ctx, prepared, streamer, yield)
	}
}

func forwardNative[Req any](
	ctx context.Context, prepared *PreparedRequest[Req], streamer StreamerAdapter[Req],
	yield func(*prompty.ResponseChunk, error) bool,
) {
	streamCtx := captureContext(ctx, prepared.capture, prepared.diagnostics.Schemas)
	events := streamer.StreamEvents(streamCtx, prepared.request)
	if scoped, ok := streamer.(ProfileStreamerAdapter[Req]); ok {
		events = scoped.StreamEventsForProfile(streamCtx, prepared.request, prepared.Diagnostics().Profile)
	}
	for chunk, streamErr := range events {
		if chunk != nil {
			copyChunk := *chunk
			chunk = &copyChunk
			chunk.Diagnostics = prepared.Diagnostics()
		}
		if !yield(chunk, streamErr) {
			return
		}
	}
}

func (c *clientImpl[Req, Resp]) bufferedEvents(
	ctx context.Context, exec *prompty.PromptExecution, active **PreparedRequest[Req],
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		resp, err := c.execute(ctx, exec, active)
		if err != nil && resp == nil {
			yield(nil, err)
			return
		}
		if resp == nil {
			yield(nil, ErrInvalidResponse)
			return
		}
		kind := prompty.StreamFinish
		if err != nil && resp.Outcome == "" {
			kind = prompty.StreamDelta
		}
		if !yield(&prompty.ResponseChunk{
			Identity: prompty.StreamIdentity{MessageID: "", PartIDs: bufferedPartIDs(len(resp.Content))},
			Model:    resp.Model, Content: resp.Content, Usage: resp.Usage, Kind: kind,
			ProviderState: resp.ProviderState, Annotations: resp.Annotations,
			MessageAnnotations:      resp.MessageAnnotations,
			ContinuationUnavailable: resp.ContinuationUnavailable,
			Outcome:                 resp.Outcome, FinishReason: resp.FinishReason,
			Diagnostics: resp.Diagnostics,
		}, nil) {
			return
		}
		if err != nil {
			yield(nil, err)
		}
	}
}

func bufferedPartIDs(count int) []string {
	ids := make([]string, count)
	for i := range ids {
		ids[i] = fmt.Sprintf("buffered:%d", i)
	}
	return ids
}

// NewClient creates a prompty.Invoker from ProviderAdapter. Middlewares wrap the base Invoker.
func NewClient[Req any, Resp any](adp ProviderAdapter[Req, Resp], mws ...prompty.Middleware) prompty.Invoker {
	base := &clientImpl[Req, Resp]{adapter: adp}
	if len(mws) == 0 {
		return base
	}
	return prompty.Chain(base, mws...)
}
