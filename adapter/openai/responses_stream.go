package openai

import (
	"context"
	"encoding/json"
	"errors"
	"iter"
	"sync"
	"time"

	"github.com/openai/openai-go/v3/option"

	"github.com/skosovsky/prompty"
)

var _ prompty.RemoteResponseStreamer = (*Responses)(nil)

// CreateStream constructs one lazy native foreground operation. It never polls,
// retries, reconnects, runs local handlers or issues a synchronous Create.
func (r *Responses) CreateStream(
	ctx context.Context,
	input prompty.RemoteResponseRequest,
) prompty.RemoteResponseStream {
	creation, err := r.prepareCreate(input, prompty.StreamNative)
	if err != nil {
		return &responseStreamHandle{
			mu: sync.Mutex{},
			handle: prompty.RemoteResponseHandle{
				Format:                "",
				Identity:              prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
				AccountBinding:        "",
				ProviderID:            "",
				Status:                "",
				Background:            false,
				Store:                 false,
				ProviderStateRequired: false,
				OutputSchemaDigest:    "",
				HostedTools:           nil,
				ToolSchemaDigests:     nil,
				ForcedTool:            "",
				ToolCalls:             nil,
			},
			known: false,
			report: prompty.ExecutionDiagnostics{
				Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
				Snapshot:   "",
				Provenance: "",
				Override:   false,
				Checks:     nil,
				Schemas:    nil,
				Capture:    nil,
				Cache:      nil,
			},
			stream: prompty.ErrorStream(ctx, prompty.StreamNative, err),
		}
	}
	handle := &responseStreamHandle{
		stream: nil,
		mu:     sync.Mutex{},
		known:  false,
		handle: prompty.CloneRemoteResponseHandle(creation.handle),
		report: prompty.CloneExecutionDiagnostics(creation.report),
	}
	capture, err := newResponseCapture("response.create_stream", creation.report,
		creation.execution.Requirements.Capture, creation.execution.Capture)
	if err != nil {
		handle.stream = prompty.ErrorStream(ctx, prompty.StreamNative, err)
		return handle
	}
	capture.sseSource = new(responseSSESource)
	handle.stream = prompty.NewStream(ctx, prompty.StreamNative,
		func(invocation context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				r.consumeResponseSSE(invocation, creation, handle, capture, yield)
			}
		}, prompty.WithStreamCaptureObserver(func(invocation context.Context, status prompty.StreamStatus, sourceErr error) (*prompty.CaptureReport, error) {
			result := &prompty.RemoteResponseResult{Handle: nil, Response: status.Result,
				Diagnostics: prompty.CloneExecutionDiagnostics(creation.report)}
			handle.mu.Lock()
			if handle.known {
				owned := prompty.CloneRemoteResponseHandle(handle.handle)
				result.Handle = &owned
			}
			handle.mu.Unlock()
			result, finishErr := capture.finish(invocation, result, sourceErr)
			handle.mu.Lock()
			handle.report = prompty.CloneExecutionDiagnostics(result.Diagnostics)
			handle.mu.Unlock()
			return result.Diagnostics.Capture, finishErr
		}))
	return handle
}

func (r *Responses) consumeResponseSSE(
	ctx context.Context, creation *preparedResponseCreate, handle *responseStreamHandle,
	capture *responseCapture, yield func(*prompty.ResponseChunk, error) bool,
) {
	translated := struct {
		Request any  `json:"request"`
		Stream  bool `json:"stream"`
	}{Request: creation.request.params, Stream: true}
	if err := capture.before(ctx, creation.canonical, translated); err != nil {
		yield(nil, err)
		return
	}
	if err := ctx.Err(); err != nil {
		yield(nil, err)
		return
	}
	continuationErr := prompty.ValidateContinuation(creation.execution.Messages, creation.report.Profile, time.Now())
	if continuationErr != nil {
		yield(nil, continuationErr)
		return
	}
	capture.dispatched = true
	upstream := r.base.client.Responses.NewStreaming(ctx, creation.request.params,
		option.WithMaxRetries(0), option.WithMiddleware(capture.sseSource.observe))
	defer upstream.Close()
	machine := newResponseSSEMachine(creation, handle)
	for upstream.Next() {
		event := upstream.Current()
		capture.sseSource.events++
		frames, eventErr := machine.accept(event)
		captureErr := capture.emit(ctx, prompty.CaptureProviderResponse, json.RawMessage(event.RawJSON()))
		for _, frame := range frames {
			if !yield(frame, nil) {
				return
			}
		}
		if eventErr != nil || captureErr != nil {
			yield(nil, errors.Join(eventErr, captureErr))
			return
		}
	}
	if err := upstream.Err(); err != nil {
		yield(nil, errors.Join(prompty.ErrRemoteResponseUnknown, ctx.Err()))
		return
	}
	if !machine.finished {
		yield(nil, prompty.ErrStreamProtocol)
		return
	}
	capture.sseSource.complete = true
}
