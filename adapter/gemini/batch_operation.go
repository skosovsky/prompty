package gemini

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// These native operation paths are shared by the forthcoming batch manager.
// They do not advertise a completed BatchManager before submit/results exist.
//
//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (a *Adapter) batchOperation(
	ctx context.Context,
	account string,
	handle prompty.BatchHandle,
	options prompty.BatchOperationOptions,
	cancel bool,
) (*prompty.BatchResult, error) {
	handle = prompty.CloneBatchHandle(handle)
	if err := prompty.ValidateBatchHandle(handle); err != nil {
		return nil, err
	}
	if strings.TrimSpace(account) == "" || handle.AccountBinding != account || handle.Provider != wireGemini ||
		handle.Endpoint != a.RuntimeDescriptor().Endpoint || !batchResourceName.MatchString(handle.ProviderID) {
		return nil, prompty.ErrBatchScope
	}
	if err := validateBatchResources(handle.Resources); err != nil {
		return nil, err
	}
	if a.client == nil {
		return nil, adapter.ErrNoClient
	}
	if a.client.ClientConfig().Backend != genai.BackendGeminiAPI {
		return nil, adapter.ErrUnsupportedCapability
	}
	operation, capability := "batch.inspect", prompty.CapabilityBatchInspect
	if cancel {
		operation, capability = "batch.cancel", prompty.CapabilityBatchCancel
	}
	result := &prompty.BatchResult{
		Items: nil,
		Completeness: prompty.BatchCompleteness{
			DownloadComplete:    false,
			CorrelationComplete: false,
			MissingIDs:          nil,
			DuplicateIDs:        nil,
			UnexpectedIDs:       nil,
		},
		Diagnostics:         nil,
		Capture:             nil,
		Resources:           nil,
		SubmissionAttempted: false,
		Handle:              &handle,
	}
	for _, item := range handle.Items {
		report, err := adapter.PreflightOperation(a.RuntimeDescriptor(), item.Identity.Model,
			[]string{capability}, nil)
		if err != nil {
			return result, err
		}
		if report.Profile != item.Identity || item.Identity.Model != handle.Items[0].Identity.Model {
			return result, prompty.ErrBatchScope
		}
		result.Diagnostics = append(result.Diagnostics, report)
	}
	if a.responseLimit <= 0 {
		return result, prompty.ErrBatch
	}
	capture, err := adapter.NewBatchCapture(operation, "google.golang.org/genai", handle.Items[0].Identity, options)
	if err != nil {
		return result, err
	}
	if err = capture.Before(
		ctx, handle, map[string]any{wireOperation: operation, wireName: handle.ProviderID},
	); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	capture.MarkDispatched()
	source, operationErr := observeBatchOperation(
		ctx,
		a.responseLimit,
		func(callCtx context.Context) (*genai.BatchJob, error) {
			if cancel {
				return nil, a.client.Batches.Cancel(
					callCtx,
					handle.ProviderID,
					&genai.CancelBatchJobConfig{HTTPOptions: noRetryHTTPOptions(nil)},
				)
			}
			return a.client.Batches.Get(
				callCtx,
				handle.ProviderID,
				&genai.GetBatchJobConfig{HTTPOptions: noRetryHTTPOptions(nil)},
			)
		},
	)
	switch {
	case operationErr != nil:
		kind := prompty.ErrBatchUnavailable
		if cancel {
			kind = prompty.ErrBatchUnknown
			if source.status >= http.StatusBadRequest && source.status < http.StatusInternalServerError &&
				source.status != http.StatusRequestTimeout && source.status != http.StatusTooManyRequests {
				kind = prompty.ErrBatchRejected
			} else {
				handle.Status = prompty.BatchUnknown
			}
		}
		operationErr = errors.Join(kind, operationErr)
	case cancel:
		// The cancellation API returns an empty acknowledgement, not a terminal job.
		switch handle.Status {
		case prompty.BatchEnded, prompty.BatchFailed, prompty.BatchExpired, prompty.BatchCanceled:
		case prompty.BatchQueued, prompty.BatchRunning, prompty.BatchCanceling, prompty.BatchUnknown:
			handle.Status = prompty.BatchCanceling
		}
	default:
		var observed prompty.BatchHandle
		observed, operationErr = parseBatchSource(source, handle)
		if operationErr == nil {
			handle = observed
		}
	}
	providerErr := capture.Provider(ctx, batchSourceJSON(source), source.status, source.complete && !source.overflow)
	return capture.Finish(ctx, result, errors.Join(operationErr, providerErr))
}
