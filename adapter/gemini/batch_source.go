package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

const batchJSONDepthLimit = 256

var batchResourceName = regexp.MustCompile(`^batches/[A-Za-z0-9_-]+$`)

var batchFileName = regexp.MustCompile(`^files/[A-Za-z0-9_-]+$`)

func validateBatchResources(resources []prompty.BatchResourceRef) error {
	for _, resource := range resources {
		if (resource.Kind != wireInput && resource.Kind != wireOutput) ||
			!batchFileName.MatchString(resource.ProviderID) {
			return prompty.ErrBatchScope
		}
	}
	return nil
}

// SDK-MUTATION-ID: SDK-owned acquisition, no body identity recovery or panic catch.
func observeBatchOperation(
	ctx context.Context,
	limit int,
	call func(context.Context) (*genai.BatchJob, error),
) (*responseSource, error) {
	source := &responseSource{
		limit:    limit,
		data:     bytes.Buffer{},
		overflow: false,
		complete: false,
		status:   0,
		job:      nil,
		cache:    nil,
	}
	if limit <= 0 {
		return source, prompty.ErrBatch
	}
	job, err := call(ctx)
	source.job = job
	source.status = sdkOperationHTTPStatus(err)
	if job != nil {
		encoded, encodingErr := json.Marshal(job)
		if encodingErr == nil && len(encoded) <= limit {
			_, _ = source.data.Write(encoded)
			source.complete = true
		} else {
			source.overflow = true
		}
	}
	if err != nil || source.overflow {
		return source, errors.Join(prompty.ErrBatchResults, ctx.Err())
	}
	// Cancel returns a public acknowledgement with no job DTO.
	if job == nil {
		source.complete = true
	}
	return source, nil
}

func batchSourceJSON(source *responseSource) prompty.JSONDocument {
	if source == nil || !source.complete || source.overflow || !json.Valid(source.data.Bytes()) {
		return nil
	}
	return prompty.CloneJSONDocument(source.data.Bytes())
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func parseBatchSource(source *responseSource, previous prompty.BatchHandle) (prompty.BatchHandle, error) {
	handle := prompty.CloneBatchHandle(previous)
	job := source.job
	if job == nil || !batchResourceName.MatchString(job.Name) ||
		(handle.ProviderID != "" && job.Name != handle.ProviderID) {
		return handle, prompty.ErrBatchScope
	}
	handle.ProviderID = job.Name
	if len(handle.Items) == 0 ||
		job.Model != "" &&
			strings.TrimPrefix(job.Model, "models/") != strings.TrimPrefix(handle.Items[0].Identity.Model, "models/") {
		return handle, prompty.ErrBatchResults
	}
	status, err := geminiBatchStatus(string(job.State))
	if err != nil {
		return handle, err
	}
	handle.Status, handle.ProviderStatus = status, string(job.State)
	var outputFile string
	// SDK-GENAI-BATCH-DEST: map only the returned SDK destination; no REST recovery.
	if job.Dest != nil {
		outputFile = job.Dest.FileName
		if outputFile != "" && job.Dest.InlinedResponses != nil {
			return handle, prompty.ErrBatchResults
		}
	}
	if outputFile != "" {
		if !batchFileName.MatchString(outputFile) {
			return handle, prompty.ErrBatchScope
		}
		resources := make([]prompty.BatchResourceRef, 0, len(handle.Resources)+1)
		for _, resource := range handle.Resources {
			if resource.Kind == wireInput {
				resources = append(resources, resource)
			}
			if resource.Kind == wireOutput && resource.ProviderID != outputFile {
				return handle, prompty.ErrBatchScope
			}
		}
		resources = append(resources, prompty.BatchResourceRef{Kind: wireOutput, ProviderID: outputFile})
		handle.Resources = resources
	}
	return handle, nil
}

func validateBatchJSON(payload []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	depth := 0
	for {
		token, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return prompty.ErrBatchResults
		}
		if delimiter, ok := token.(json.Delim); ok {
			switch delimiter {
			case '{', '[':
				depth++
			case '}', ']':
				depth--
			}
			if depth > batchJSONDepthLimit {
				return prompty.ErrBatchResults
			}
		}
	}
	if uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil {
		return prompty.ErrBatchResults
	}
	return nil
}

func geminiBatchStatus(status string) (prompty.BatchStatus, error) {
	switch status {
	case "JOB_STATE_PENDING", "JOB_STATE_QUEUED":
		return prompty.BatchQueued, nil
	case "JOB_STATE_RUNNING", "JOB_STATE_UPDATING":
		return prompty.BatchRunning, nil
	case "JOB_STATE_CANCELLING":
		return prompty.BatchCanceling, nil
	case "JOB_STATE_SUCCEEDED", "JOB_STATE_PARTIALLY_SUCCEEDED":
		return prompty.BatchEnded, nil
	case "JOB_STATE_FAILED":
		return prompty.BatchFailed, nil
	case "JOB_STATE_EXPIRED":
		return prompty.BatchExpired, nil
	case "JOB_STATE_CANCELLED":
		return prompty.BatchCanceled, nil
	case "JOB_STATE_UNSPECIFIED", "JOB_STATE_PAUSED":
		return prompty.BatchUnknown, nil
	default:
		return prompty.BatchUnknown, prompty.ErrBatchResults
	}
}
