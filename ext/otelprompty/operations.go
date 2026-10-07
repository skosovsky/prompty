package otelprompty

import (
	"context"

	"github.com/skosovsky/prompty"
)

const responseResource = "response"

// WrapRemoteResponses instruments explicit remote operations. Create records one
// generation; Get and Cancel record resource observations without token usage.
// It does not poll, retry, store handles, or implicitly expose streaming.
func WrapRemoteResponses(next prompty.RemoteResponseManager, opts ...Option) prompty.RemoteResponseManager {
	return &responseOperations{next: next, cfg: resolveConfig(opts)}
}

type responseOperations struct {
	next prompty.RemoteResponseManager
	cfg  *config
}

func (t *responseOperations) Create(
	ctx context.Context,
	request prompty.RemoteResponseRequest,
) (*prompty.RemoteResponseResult, error) {
	obs := invocationObservation(request.Execution)
	obs.ResourceKind = responseResource
	ctx, span := begin(ctx, t.cfg, obs)
	result, err := t.next.Create(ctx, request)
	obs.Err, obs.Resource = err, ownedOperationResource(result)
	if result != nil {
		obs.Response = result.Response
		remoteObservation(&obs, result)
	}
	span.finish(obs)
	return result, err
}

func (t *responseOperations) Get(
	ctx context.Context,
	handle prompty.RemoteResponseHandle,
	opts ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	return t.resource(ctx, "response.get", handle, func(ctx context.Context) (*prompty.RemoteResponseResult, error) {
		return t.next.Get(ctx, handle, opts...)
	})
}

func (t *responseOperations) Cancel(
	ctx context.Context,
	handle prompty.RemoteResponseHandle,
	opts ...prompty.RemoteResponseOperationOptions,
) (*prompty.RemoteResponseResult, error) {
	return t.resource(ctx, "response.cancel", handle, func(ctx context.Context) (*prompty.RemoteResponseResult, error) {
		return t.next.Cancel(ctx, handle, opts...)
	})
}

func (t *responseOperations) resource(
	ctx context.Context,
	operation string,
	handle prompty.RemoteResponseHandle,
	call func(context.Context) (*prompty.RemoteResponseResult, error),
) (*prompty.RemoteResponseResult, error) {
	var obs Observation
	obs.Operation, obs.Provider, obs.RequestModel, obs.ResourceKind = operation, handle.Identity.Provider, handle.Identity.Model, responseResource
	obs.SuppressGenerationUsage = true
	ctx, span := begin(ctx, t.cfg, obs)
	result, err := call(ctx)
	obs.Err, obs.Resource = err, ownedOperationResource(result)
	if result != nil {
		obs.Response = result.Response
		remoteObservation(&obs, result)
	}
	span.finish(obs)
	return result, err
}

func remoteObservation(obs *Observation, result *prompty.RemoteResponseResult) {
	if result.Handle != nil {
		obs.Provider = result.Handle.Identity.Provider
		if obs.RequestModel == "" {
			obs.RequestModel = result.Handle.Identity.Model
		}
		obs.ResourceStatus = string(result.Handle.Status)
	} else if result.Diagnostics.Profile.Provider != "" {
		obs.Provider = result.Diagnostics.Profile.Provider
	}
}

// WrapResponseStreaming instruments the native lazy CreateStream lifecycle on
// its original handle. Merely creating or closing an unused handle produces no
// provider generation span. ObserveLifecycle supplies the original cancellation
// and consumer-stop semantics without a second producer loop.
func WrapResponseStreaming(next prompty.RemoteResponseStreamer, opts ...Option) prompty.RemoteResponseStreamer {
	return &responseStreaming{next: next, cfg: resolveConfig(opts)}
}

type responseStreaming struct {
	next prompty.RemoteResponseStreamer
	cfg  *config
}

func (t *responseStreaming) CreateStream(
	ctx context.Context,
	request prompty.RemoteResponseRequest,
) prompty.RemoteResponseStream {
	obs := invocationObservation(request.Execution)
	obs.ResourceKind = responseResource
	stream := t.next.CreateStream(ctx, request)
	if stream == nil {
		stream = &observedResponseStreamError{
			Stream: prompty.ErrorStream(ctx, prompty.StreamNative, prompty.ErrStreamProtocol), original: nil,
		}
	}
	observer := func(ctx context.Context) (context.Context, func(prompty.StreamStatus)) {
		streamCtx, span := begin(ctx, t.cfg, obs)
		return streamCtx, func(status prompty.StreamStatus) {
			obs.Response, obs.Err, obs.StreamStatus = status.Result, status.Err, &status
			result := stream.RemoteResult()
			obs.Resource = ownedOperationResource(result)
			if result != nil {
				remoteObservation(&obs, result)
			}
			span.finish(obs)
		}
	}
	if err := stream.ObserveLifecycle(observer); err != nil {
		_ = stream.Close()
		fallback := &observedResponseStreamError{
			Stream:   prompty.ErrorStream(ctx, prompty.StreamNative, err),
			original: stream,
		}
		_ = fallback.ObserveLifecycle(observer) // Fresh common stream always permits observer registration.
		return fallback
	}
	return stream
}

// A failed observer registration is reported before dispatch; the original
// handle remains available for inspecting an already-known remote identity.
type observedResponseStreamError struct {
	*prompty.Stream

	original prompty.RemoteResponseStream
}

func (s *observedResponseStreamError) RemoteResult() *prompty.RemoteResponseResult {
	if s.original == nil {
		return nil
	}
	return s.original.RemoteResult()
}

// WrapBatch instruments job/resource operations. Item results stay on the
// returned DTO and are available to explicit host projection; repeated Results
// retrieval never records generation token usage a second time.
func WrapBatch(next prompty.BatchManager, opts ...Option) prompty.BatchManager {
	return &batchOperations{next: next, cfg: resolveConfig(opts)}
}

type batchOperations struct {
	next prompty.BatchManager
	cfg  *config
}

func (t *batchOperations) Submit(
	ctx context.Context,
	request prompty.BatchSubmitRequest,
) (*prompty.BatchResult, error) {
	return t.call(
		ctx,
		"batch.submit",
		"",
		func(ctx context.Context) (*prompty.BatchResult, error) { return t.next.Submit(ctx, request) },
	)
}

func (t *batchOperations) Get(
	ctx context.Context,
	handle prompty.BatchHandle,
	opts ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return t.call(
		ctx,
		"batch.get",
		handle.Provider,
		func(ctx context.Context) (*prompty.BatchResult, error) { return t.next.Get(ctx, handle, opts...) },
	)
}

func (t *batchOperations) Cancel(
	ctx context.Context,
	handle prompty.BatchHandle,
	opts ...prompty.BatchOperationOptions,
) (*prompty.BatchResult, error) {
	return t.call(
		ctx,
		"batch.cancel",
		handle.Provider,
		func(ctx context.Context) (*prompty.BatchResult, error) { return t.next.Cancel(ctx, handle, opts...) },
	)
}

func (t *batchOperations) Results(
	ctx context.Context,
	handle prompty.BatchHandle,
	opts prompty.BatchResultsOptions,
) (*prompty.BatchResult, error) {
	return t.call(
		ctx,
		"batch.results",
		handle.Provider,
		func(ctx context.Context) (*prompty.BatchResult, error) { return t.next.Results(ctx, handle, opts) },
	)
}

func (t *batchOperations) call(
	ctx context.Context,
	operation, provider string,
	call func(context.Context) (*prompty.BatchResult, error),
) (*prompty.BatchResult, error) {
	var obs Observation
	obs.Operation, obs.Provider, obs.ResourceKind = operation, provider, "batch"
	ctx, span := begin(ctx, t.cfg, obs)
	result, err := call(ctx)
	obs.Err, obs.Resource = err, ownedOperationResource(result)
	if result != nil && result.Handle != nil {
		obs.Provider = result.Handle.Provider
		obs.ResourceStatus = string(result.Handle.Status)
	}
	span.finish(obs)
	return result, err
}

// WrapCache instruments explicit resource operations. Resource token_count is
// storage size, never generation/billing usage. No expiry-renewal loop is added.
func WrapCache(next prompty.CacheResourceManager, opts ...Option) prompty.CacheResourceManager {
	return &cacheOperations{next: next, cfg: resolveConfig(opts)}
}

type cacheOperations struct {
	next prompty.CacheResourceManager
	cfg  *config
}

func (t *cacheOperations) Create(
	ctx context.Context,
	request prompty.CacheResourceCreateRequest,
) (*prompty.CacheResourceResult, error) {
	return t.call(
		ctx,
		"cache.create",
		"",
		func(ctx context.Context) (*prompty.CacheResourceResult, error) { return t.next.Create(ctx, request) },
	)
}

func (t *cacheOperations) Get(
	ctx context.Context,
	handle prompty.CacheResourceHandle,
	opts ...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	return t.call(
		ctx,
		"cache.get",
		handle.Identity.Provider,
		func(ctx context.Context) (*prompty.CacheResourceResult, error) {
			return t.next.Get(ctx, handle, opts...)
		},
	)
}

func (t *cacheOperations) UpdateExpiry(
	ctx context.Context,
	handle prompty.CacheResourceHandle,
	expiry prompty.CacheResourceExpiry,
	opts ...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	return t.call(
		ctx,
		"cache.update_expiry",
		handle.Identity.Provider,
		func(ctx context.Context) (*prompty.CacheResourceResult, error) {
			return t.next.UpdateExpiry(ctx, handle, expiry, opts...)
		},
	)
}

func (t *cacheOperations) Delete(
	ctx context.Context,
	handle prompty.CacheResourceHandle,
	opts ...prompty.CacheResourceOperationOptions,
) (*prompty.CacheResourceResult, error) {
	return t.call(
		ctx,
		"cache.delete",
		handle.Identity.Provider,
		func(ctx context.Context) (*prompty.CacheResourceResult, error) {
			return t.next.Delete(ctx, handle, opts...)
		},
	)
}

func (t *cacheOperations) call(
	ctx context.Context,
	operation, provider string,
	call func(context.Context) (*prompty.CacheResourceResult, error),
) (*prompty.CacheResourceResult, error) {
	var obs Observation
	obs.Operation, obs.Provider, obs.ResourceKind = operation, provider, "cache"
	ctx, span := begin(ctx, t.cfg, obs)
	result, err := call(ctx)
	obs.Err, obs.Resource = err, ownedOperationResource(result)
	if result != nil {
		obs.ResourceTokenCount = result.TokenCount
		if result.Handle != nil {
			obs.Provider = result.Handle.Identity.Provider
			obs.ResourceStatus = string(result.Handle.State)
		}
	}
	span.finish(obs)
	return result, err
}

func ownedOperationResource(resource any) any {
	switch result := resource.(type) {
	case *prompty.RemoteResponseResult:
		if result == nil {
			return (*prompty.RemoteResponseResult)(nil)
		}
		out := *result
		out.Response = prompty.CloneResponse(result.Response)
		out.Diagnostics = prompty.CloneExecutionDiagnostics(result.Diagnostics)
		if result.Handle != nil {
			handle := prompty.CloneRemoteResponseHandle(*result.Handle)
			out.Handle = &handle
		}
		return &out
	case *prompty.CacheResourceResult:
		if result == nil {
			return (*prompty.CacheResourceResult)(nil)
		}
		out := *result
		out.Diagnostics = prompty.CloneExecutionDiagnostics(result.Diagnostics)
		if result.Handle != nil {
			handle := *result.Handle
			out.Handle = &handle
		}
		if result.TokenCount != nil {
			count := *result.TokenCount
			out.TokenCount = &count
		}
		return &out
	case *prompty.BatchResult:
		return ownedBatchResult(result)
	default:
		return nil
	}
}

func ownedBatchResult(result *prompty.BatchResult) *prompty.BatchResult {
	if result == nil {
		return nil
	}
	out := *result
	if result.Handle != nil {
		handle := prompty.CloneBatchHandle(*result.Handle)
		out.Handle = &handle
	}
	out.Items = append([]prompty.BatchItemResult(nil), result.Items...)
	for i := range out.Items {
		item := &out.Items[i]
		item.Response = prompty.CloneResponse(item.Response)
		item.SourceJSON = append(prompty.JSONDocument(nil), item.SourceJSON...)
		if item.Binding != nil {
			binding := *item.Binding
			item.Binding = &binding
		}
	}
	out.Completeness.MissingIDs = append([]string(nil), result.Completeness.MissingIDs...)
	out.Completeness.DuplicateIDs = append([]string(nil), result.Completeness.DuplicateIDs...)
	out.Completeness.UnexpectedIDs = append([]string(nil), result.Completeness.UnexpectedIDs...)
	out.Diagnostics = make([]prompty.ExecutionDiagnostics, len(result.Diagnostics))
	for i, diagnostics := range result.Diagnostics {
		out.Diagnostics[i] = prompty.CloneExecutionDiagnostics(diagnostics)
	}
	out.Resources = append([]prompty.BatchResourceRef(nil), result.Resources...)
	if result.Capture != nil {
		capture := prompty.CloneCaptureReport(*result.Capture)
		out.Capture = &capture
	}
	return &out
}
