package openai

import (
	"iter"
	"net/http"
	"sync"

	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
)

// responseStreamHandle keeps remote identity separate from local consumption.
// Close cancels the transport; it does not cancel the provider resource.
type responseStreamHandle struct {
	stream *prompty.Stream
	mu     sync.Mutex
	handle prompty.RemoteResponseHandle
	known  bool
	report prompty.ExecutionDiagnostics
}

var _ prompty.RemoteResponseStream = (*responseStreamHandle)(nil)

func (h *responseStreamHandle) Events() iter.Seq2[*prompty.ResponseChunk, error] {
	return h.stream.Events()
}

func (h *responseStreamHandle) ObserveLifecycle(observer prompty.StreamLifecycleObserver) error {
	return h.stream.ObserveLifecycle(observer)
}

func (h *responseStreamHandle) Close() error { return h.stream.Close() }

func (h *responseStreamHandle) Status() prompty.StreamStatus { return h.stream.Status() }

func (h *responseStreamHandle) RemoteResult() *prompty.RemoteResponseResult {
	status := h.stream.Status()
	h.mu.Lock()
	defer h.mu.Unlock()
	result := &prompty.RemoteResponseResult{Handle: nil,
		Response:    status.Result,
		Diagnostics: prompty.CloneExecutionDiagnostics(h.report),
	}
	if h.known {
		owned := prompty.CloneRemoteResponseHandle(h.handle)
		result.Handle = &owned
	}
	if status.Result != nil {
		observed := prompty.CloneExecutionDiagnostics(status.Result.Diagnostics)
		if observed.Snapshot != "" {
			if observed.Capture == nil {
				observed.Capture = result.Diagnostics.Capture
			}
			result.Diagnostics = observed
		} else if observed.Capture != nil {
			result.Diagnostics.Capture = observed.Capture
		}
		result.Response.Diagnostics = prompty.CloneExecutionDiagnostics(result.Diagnostics)
	}
	return result
}

// observeResponse retains an independently safe resource ID even when the rest
// of a received response is malformed. It never overwrites a known identity or
// claims a remote cancellation merely because the local stream was closed.
func (h *responseStreamHandle) observeResponse(raw *responses.Response) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if raw == nil {
		return prompty.ErrRemoteResponseUnknown
	}
	source := responseSource{data: []byte(raw.RawJSON()), status: http.StatusOK, complete: true}
	identity := source.acceptedIdentity()
	if identity != "" && (!h.known || h.handle.ProviderID == identity) {
		h.handle.ProviderID = identity
		h.known = true
	}
	if !h.known || raw.ID != h.handle.ProviderID || !raw.JSON.ID.Valid() ||
		!raw.JSON.Model.Valid() || !raw.JSON.Status.Valid() || raw.Model != h.handle.Identity.Model ||
		!uniqueResponseJSON([]byte(raw.RawJSON())) {
		return prompty.ErrRemoteResponseUnknown
	}
	status := prompty.RemoteResponseStatus(raw.Status)
	if raw.Status == responses.ResponseStatusCancelled {
		status = prompty.RemoteResponseCanceled
	}
	switch status {
	case prompty.RemoteResponseQueued, prompty.RemoteResponseRunning,
		prompty.RemoteResponseCompleted, prompty.RemoteResponseFailed,
		prompty.RemoteResponseIncomplete, prompty.RemoteResponseCanceled:
	case prompty.RemoteResponseUnknown:
		fallthrough
	default:
		return prompty.ErrRemoteResponseUnknown
	}
	if h.handle.Status != prompty.RemoteResponseUnknown && h.handle.Status != status &&
		h.handle.Status != prompty.RemoteResponseQueued && h.handle.Status != prompty.RemoteResponseRunning {
		return prompty.ErrStreamProtocol
	}
	if h.handle.Status == prompty.RemoteResponseRunning && status == prompty.RemoteResponseQueued {
		return prompty.ErrStreamProtocol
	}
	h.handle.Status = status
	return nil
}
