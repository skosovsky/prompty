package prompty

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"iter"
	"slices"
	"sync"
)

// StreamMode is chosen explicitly; native never falls back to a buffered call.
type StreamMode string

const (
	StreamNative   StreamMode = "native"
	StreamBuffered StreamMode = "buffered"
)

// StreamEventKind distinguishes content/control markers from lifecycle completion.
type StreamEventKind string

const (
	StreamStart    StreamEventKind = "start"
	StreamDelta    StreamEventKind = "delta"
	StreamUsage    StreamEventKind = "usage"
	StreamFinish   StreamEventKind = "finish"
	StreamTerminal StreamEventKind = "terminal"
)

// StreamState is the observed lifecycle of a single consumption.
type StreamState string

const (
	StreamCreated   StreamState = "created"
	StreamRunning   StreamState = "streaming"
	StreamCompleted StreamState = "completed"
	StreamFailed    StreamState = "failed"
	StreamCanceled  StreamState = "canceled"
)

var (
	ErrStreamConsumed = errors.New("prompty: stream is single-use")
	ErrStreamProtocol = errors.New("prompty: invalid or incomplete stream protocol")
	ErrStreamMode     = errors.New("prompty: explicit native or buffered stream mode required")
)

// StreamStatus remains readable after early termination. Result is a cloned snapshot.
type StreamStatus struct {
	Identity StreamIdentity
	State    StreamState
	Mode     StreamMode
	Result   *Response
	Err      error
}

// StreamFinalizer validates a cloned assembled result after source EOF and trailing
// usage, before terminal success. It must honor cancellation and must not retry.
type StreamFinalizer func(context.Context, *Response) error

// StreamCaptureObserver records final/failed snapshots without changing content.
// It runs outside the status mutex and must honor invocation cancellation.
type StreamCaptureObserver func(context.Context, StreamStatus, error) (*CaptureReport, error)

// StreamFrameObserver observes an isolated accepted frame with canonical identity.
// An observation failure retains and yields the frame before failing the stream.
// It must honor cancellation; it runs without the status mutex.
type StreamFrameObserver func(context.Context, *ResponseChunk) error

// StreamLifecycleObserver starts lazy observation and returns a context with
// added values and a closeout callback. It must retain the same Done channel.
// Closeout sees an isolated final/canceled status without the status mutex held.
type StreamLifecycleObserver func(context.Context) (context.Context, func(StreamStatus))

// WithStreamFrameObserver binds optional canonical frame observation before consumption.
func WithStreamFrameObserver(observer StreamFrameObserver) StreamOption {
	return func(stream *Stream) { stream.frameObserver = observer }
}

// WithStreamCaptureObserver installs optional observation before terminal delivery.
func WithStreamCaptureObserver(observer StreamCaptureObserver) StreamOption {
	return func(stream *Stream) { stream.captureObserver = observer }
}

// StreamOption configures a handle before consumption.
type StreamOption func(*Stream)

// WithStreamFinalizer installs a final validation port. It never runs after partial
// transport/protocol failure or consumer cancellation, and does not mutate the result.
func WithStreamFinalizer(finalizer StreamFinalizer) StreamOption {
	return func(stream *Stream) { stream.finalizer = finalizer }
}

// Stream owns a lazy, single-use source and its cancellation context. It creates
// no producer goroutine. Sources must honor context cancellation and release
// transport resources when yield returns false.
type Stream struct {
	mu                 sync.Mutex
	ctx                context.Context
	cancel             context.CancelFunc
	source             func(context.Context) iter.Seq2[*ResponseChunk, error]
	status             StreamStatus
	used               bool
	finalizer          StreamFinalizer
	chunkValidator     func(*ResponseChunk) error
	captureObserver    StreamCaptureObserver
	frameObserver      StreamFrameObserver
	partBindings       map[string]streamPartBinding
	toolParts          map[string]string
	lifecycleObservers []StreamLifecycleObserver
	sourceCtx          context.Context
	sourceCancel       context.CancelFunc
}

// NewStream constructs a lazy lifecycle handle. Creating a handle does not dispatch.
// The source emits data/control events and a finish marker, not a terminal event.
func NewStream(
	ctx context.Context,
	mode StreamMode,
	source func(context.Context) iter.Seq2[*ResponseChunk, error],
	options ...StreamOption,
) *Stream {
	child, cancel := context.WithCancel(ctx)
	sourceCtx, sourceCancel := context.WithCancel(child)
	stream := &Stream{
		ctx:    child,
		cancel: cancel,
		source: source,
		mu:     sync.Mutex{},
		used:   false,
		status: StreamStatus{
			Identity: StreamIdentity{MessageID: rand.Text(), PartIDs: nil},
			State:    StreamCreated,
			Mode:     mode,
			Result:   &Response{},
			Err:      nil,
		},
		finalizer:          nil,
		chunkValidator:     nil,
		captureObserver:    nil,
		sourceCtx:          sourceCtx,
		sourceCancel:       sourceCancel,
		frameObserver:      nil,
		partBindings:       make(map[string]streamPartBinding),
		toolParts:          make(map[string]string),
		lifecycleObservers: nil,
	}
	for _, option := range options {
		option(stream)
	}
	return stream
}

// ObserveLifecycle attaches middleware to this handle without replaying or
// reassembling its events. Attachment is permitted only before consumption.
func (s *Stream) ObserveLifecycle(observer StreamLifecycleObserver) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used {
		return ErrStreamConsumed
	}
	if s.status.State != StreamCreated {
		return s.status.Err
	}
	if observer == nil {
		return fmt.Errorf("%w: nil lifecycle observer", ErrStreamProtocol)
	}
	s.lifecycleObservers = append(s.lifecycleObservers, observer)
	return nil
}

// ErrorStream reports a pre-dispatch failure when consumed.
func ErrorStream(ctx context.Context, mode StreamMode, err error) *Stream {
	return NewStream(ctx, mode, func(context.Context) iter.Seq2[*ResponseChunk, error] {
		return func(yield func(*ResponseChunk, error) bool) { yield(nil, err) }
	})
}

// Status returns an isolated partial/final result. Terminal state is immutable.
func (s *Stream) Status() StreamStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := s.status
	result.Result = cloneResponse(result.Result)
	result.Identity = cloneStreamIdentity(result.Identity)
	return result
}

// Close requests cancellation without waiting. A created handle becomes canceled
// immediately; a running iterator publishes final status after unwind/closeout.
// It is safe to call concurrently and repeatedly, including from a callback.
func (s *Stream) Close() error {
	s.cancel()
	s.mu.Lock()
	if s.status.State == StreamCreated {
		s.status.State, s.status.Err = StreamCanceled, context.Canceled
	}
	s.mu.Unlock()
	return nil
}

// Events starts dispatch on first consumption and yields exactly one terminal event
// unless the consumer stops. In that case Status retains the canceled outcome.
func (s *Stream) Events() iter.Seq2[*ResponseChunk, error] {
	return func(yield func(*ResponseChunk, error) bool) {
		if err := s.begin(); err != nil {
			yield(nil, err)
			return
		}
		defer s.cancel()
		closeout, observerErr := s.startLifecycleObservation()
		defer closeout()
		if observerErr != nil {
			observerErr = s.observeCapture(observerErr)
			s.deliverTerminal(yield, StreamFailed, observerErr)
			return
		}
		stopped, err := s.consume(yield)
		if stopped {
			err = s.observeCapture(errors.Join(context.Canceled, err))
			s.complete(StreamCanceled, err)
			return
		}
		err = errors.Join(err, s.ctx.Err())
		if err == nil && s.finalizer != nil {
			err = s.finalizer(s.ctx, s.Status().Result)
			if contextErr := s.ctx.Err(); contextErr != nil {
				err = errors.Join(err, contextErr)
			}
		}
		err = s.observeCapture(err)
		err = errors.Join(err, s.ctx.Err())
		state := StreamCompleted
		if err != nil {
			state = failureState(err)
		}
		s.deliverTerminal(yield, state, err)
	}
}

func (s *Stream) startLifecycleObservation() (func(), error) {
	callbacks := make([]func(StreamStatus), 0, len(s.lifecycleObservers))
	closeout := func() {
		for _, callback := range slices.Backward(callbacks) {
			callback(s.Status())
		}
	}
	for _, observer := range slices.Backward(s.lifecycleObservers) {
		finish, err := s.startLifecycleObserver(observer)
		if finish != nil {
			callbacks = append(callbacks, finish)
		}
		if err != nil {
			return closeout, err
		}
	}
	if len(callbacks) != 0 {
		s.sourceCancel()
		s.sourceCtx, s.sourceCancel = context.WithCancel(s.ctx)
	}
	return closeout, nil
}

func (s *Stream) startLifecycleObserver(observer StreamLifecycleObserver) (func(StreamStatus), error) {
	ctx, finish := observer(s.ctx)
	if ctx == nil || finish == nil || ctx.Done() != s.ctx.Done() {
		return finish, fmt.Errorf("%w: invalid lifecycle observer binding", ErrStreamProtocol)
	}
	s.ctx = ctx
	return finish, nil
}

func (s *Stream) observeCapture(sourceErr error) error {
	if s.captureObserver == nil {
		return sourceErr
	}
	report, captureErr := s.captureObserver(s.ctx, s.Status(), sourceErr)
	if report != nil {
		copyReport := CloneCaptureReport(*report)
		if reportErr := copyReport.Validate(); reportErr != nil {
			return errors.Join(sourceErr, captureErr, reportErr)
		}
		s.mu.Lock()
		if s.status.State == StreamRunning {
			s.status.Result.Diagnostics.Capture = &copyReport
		}
		s.mu.Unlock()
	}
	return errors.Join(sourceErr, captureErr)
}

func (s *Stream) consume(yield func(*ResponseChunk, error) bool) (bool, error) {
	if err := s.ctx.Err(); err != nil {
		return false, err
	}
	if s.status.Mode != StreamNative && s.status.Mode != StreamBuffered {
		return false, ErrStreamMode
	}
	if s.source == nil {
		return false, ErrStreamProtocol
	}
	finished := false
	for chunk, sourceErr := range s.source(s.sourceCtx) {
		if sourceErr != nil {
			s.sourceCancel()
			return false, sourceErr
		}
		if contextErr := s.ctx.Err(); contextErr != nil {
			return false, contextErr
		}
		var protocolErr error
		finished, protocolErr = checkStreamEvent(chunk, finished)
		if protocolErr != nil {
			s.sourceCancel() // release producer while keeping final observation context live
			return false, protocolErr
		}
		stopped, err := s.acceptFrame(chunk, yield)
		if stopped {
			return true, err
		}
		if err != nil {
			s.sourceCancel()
			return false, err
		}
	}
	if err := s.ctx.Err(); err != nil {
		return false, err
	}
	if !finished {
		return false, fmt.Errorf("%w: missing finish marker", ErrStreamProtocol)
	}
	return false, s.materialize()
}

func (s *Stream) acceptFrame(chunk *ResponseChunk, yield func(*ResponseChunk, error) bool) (bool, error) {
	chunk = cloneResponseChunk(chunk)
	if err := s.accumulate(chunk); err != nil {
		return false, err
	}
	var observationErr error
	if s.frameObserver != nil {
		observationErr = s.frameObserver(s.ctx, cloneResponseChunk(chunk))
	}
	if err := s.validateChunk(chunk); err != nil {
		return false, errors.Join(err, observationErr)
	}
	if !yield(cloneResponseChunk(chunk), nil) {
		s.sourceCancel()
		return true, observationErr
	}
	return false, observationErr
}

func (s *Stream) validateChunk(chunk *ResponseChunk) error {
	if s.chunkValidator == nil {
		return nil
	}
	return s.chunkValidator(cloneResponseChunk(chunk))
}

// Attach validation while the handle is still lazy. It is part of the same
// lifecycle, not a consumer break or a replacement for the prepared finalizer.
func (s *Stream) attachValidation(chunk func(*ResponseChunk) error, final StreamFinalizer) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used {
		return ErrStreamConsumed
	}
	if s.status.State != StreamCreated {
		return s.status.Err
	}
	previous := s.finalizer
	s.chunkValidator = chunk
	s.finalizer = func(ctx context.Context, response *Response) error {
		if err := final(ctx, response); err != nil {
			return err
		}
		if previous != nil {
			return previous(ctx, response)
		}
		return nil
	}
	return nil
}

func checkStreamEvent(chunk *ResponseChunk, finished bool) (bool, error) {
	if chunk == nil {
		return finished, ErrStreamProtocol
	}
	switch chunk.Kind {
	case StreamStart, StreamDelta, StreamUsage:
		if finished && (chunk.Kind != StreamUsage || len(chunk.Content) != 0) {
			return finished, ErrStreamProtocol
		}
	case StreamFinish:
		if finished || !validOutcome(chunk.Outcome) {
			return finished, ErrStreamProtocol
		}
		return true, nil
	case StreamTerminal:
		return finished, ErrStreamProtocol
	default:
		return finished, ErrStreamProtocol
	}
	return finished, nil
}

func validOutcome(outcome Outcome) bool {
	return slices.Contains([]Outcome{OutcomeCompleted, OutcomeToolCalls, OutcomeRefusal, OutcomeIncomplete}, outcome)
}

func (s *Stream) begin() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.used {
		return ErrStreamConsumed
	}
	s.used = true
	if s.status.State != StreamCreated {
		return s.status.Err
	}
	s.status.State = StreamRunning
	return nil
}

func (s *Stream) accumulate(chunk *ResponseChunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.State != StreamRunning {
		return s.status.Err
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	result := s.status.Result
	bindings, err := checkStreamIdentity(chunk, s.status.Identity.MessageID, s.partBindings, s.toolParts)
	if err != nil {
		return err
	}
	if err = ValidatePartAssociations(chunk.Content, chunk.ProviderState, chunk.Annotations); err != nil {
		return err
	}
	if err = ValidateMessageAnnotations(chunk.MessageAnnotations); err != nil {
		return err
	}
	states, err := s.updatedPartStates(chunk)
	if err != nil {
		return err
	}
	usage, err := mergeUsage(result.Usage, chunk.Usage)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrStreamProtocol, err)
	}
	offset := len(result.Content)
	annotations := clonePartAnnotations(chunk.Annotations)
	for i := range annotations {
		annotations[i].PartIndex += offset
	}
	result.ProviderState = states
	result.Annotations = append(result.Annotations, annotations...)
	result.MessageAnnotations = append(result.MessageAnnotations, cloneMessageAnnotations(chunk.MessageAnnotations)...)
	result.ContinuationUnavailable = result.ContinuationUnavailable || chunk.ContinuationUnavailable
	result.Content = append(result.Content, cloneContentParts(chunk.Content)...)
	chunk.Identity.MessageID = s.status.Identity.MessageID
	s.status.Identity.PartIDs = append(s.status.Identity.PartIDs, chunk.Identity.PartIDs...)
	for id, binding := range bindings {
		s.partBindings[id] = binding
		if binding.toolID != "" {
			s.toolParts[binding.toolID] = id
		}
	}
	result.Usage = usage
	if chunk.Outcome != "" {
		result.Outcome = chunk.Outcome
	}
	if chunk.FinishReason != "" {
		result.FinishReason = chunk.FinishReason
	}
	if chunk.Diagnostics.Snapshot != "" {
		result.Diagnostics = CloneExecutionDiagnostics(chunk.Diagnostics)
	}
	return nil
}

func (s *Stream) materialize() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.State != StreamRunning {
		return s.status.Err
	}
	if err := s.ctx.Err(); err != nil {
		return err
	}
	result := s.status.Result
	parts, association, err := assembleToolCallPartsWithReferences(result.Content, s.status.Identity.PartIDs)
	if err != nil {
		return err
	}
	states := clonePartStates(result.ProviderState)
	for i := range states {
		oldIndex := states[i].PartIndex
		newIndex := association[oldIndex]
		if !sameStateBoundContent(result.Content[oldIndex], parts[newIndex]) {
			return fmt.Errorf("%w: assembly would alter state-bound content", ErrProviderState)
		}
		states[i].PartIndex = newIndex
	}
	annotations := clonePartAnnotations(result.Annotations)
	for i := range annotations {
		annotations[i].PartIndex = association[annotations[i].PartIndex]
	}
	if err = ValidatePartAssociations(parts, states, annotations); err != nil {
		return err
	}
	result.Content, result.ProviderState, result.Annotations = parts, states, annotations
	s.status.Identity.PartIDs = materializedPartIDs(s.status.Identity.PartIDs, association, len(parts))
	return nil
}

func (s *Stream) complete(state StreamState, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.status.State != StreamCreated && s.status.State != StreamRunning {
		return
	}
	s.status.State, s.status.Err = state, err
}

func (s *Stream) deliverTerminal(yield func(*ResponseChunk, error) bool, state StreamState, err error) {
	s.complete(state, err)
	status := s.Status()
	yield(&ResponseChunk{
		Identity: status.Identity,
		Kind:     StreamTerminal, Result: status.Result, Outcome: status.Result.Outcome,
		Usage: status.Result.Usage, FinishReason: status.Result.FinishReason,
	}, status.Err)
}

func failureState(err error) StreamState {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return StreamCanceled
	}
	return StreamFailed
}

func cloneResponse(resp *Response) *Response {
	if resp == nil {
		return nil
	}
	out := *resp
	out.Content = cloneContentParts(resp.Content)
	out.ProviderState = clonePartStates(resp.ProviderState)
	out.Annotations = clonePartAnnotations(resp.Annotations)
	out.MessageAnnotations = cloneMessageAnnotations(resp.MessageAnnotations)
	out.Usage.Known = slices.Clone(resp.Usage.Known)
	out.Diagnostics = CloneExecutionDiagnostics(resp.Diagnostics)
	return &out
}

func cloneResponseChunk(chunk *ResponseChunk) *ResponseChunk {
	out := *chunk
	out.Identity = cloneStreamIdentity(chunk.Identity)
	out.Content = cloneContentParts(chunk.Content)
	out.ProviderState = clonePartStates(chunk.ProviderState)
	out.StateUpdates = clonePartStates(chunk.StateUpdates)
	out.Annotations = clonePartAnnotations(chunk.Annotations)
	out.MessageAnnotations = cloneMessageAnnotations(chunk.MessageAnnotations)
	out.Usage.Known = slices.Clone(chunk.Usage.Known)
	out.Diagnostics = CloneExecutionDiagnostics(chunk.Diagnostics)
	out.Result = cloneResponse(chunk.Result)
	return &out
}

func mergeUsage(acc, update Usage) (Usage, error) {
	if err := update.Validate(); err != nil {
		return acc, err
	}
	acc.Known = slices.Clone(acc.Known)
	for _, field := range update.Known {
		target, value := usageCounter(&acc, field), usageCounter(&update, field)
		if target == nil || value == nil {
			continue
		}
		if update.Mode == UsageDelta {
			const maxInt = int(^uint(0) >> 1)
			if *value > maxInt-*target {
				return Usage{}, fmt.Errorf("%w: counter accumulation overflow", ErrUsage)
			}
			*target += *value
		} else {
			*target = *value
		}
		if !slices.Contains(acc.Known, field) {
			acc.Known = append(acc.Known, field)
		}
	}
	acc.Mode = UsageCumulative
	return acc, nil
}

func usageCounter(usage *Usage, field UsageCounter) *int {
	switch field {
	case UsagePrompt:
		return &usage.PromptTokens
	case UsageCompletion:
		return &usage.CompletionTokens
	case UsageTotal:
		return &usage.TotalTokens
	case UsageCacheRead:
		return &usage.PromptTokensCached
	case UsageCacheWrite:
		return &usage.PromptTokensCacheCreation
	case UsageReasoning:
		return &usage.CompletionTokensReasoning
	default:
		return nil
	}
}
