package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"strings"

	"github.com/skosovsky/prompty"
)

type batchSourceKey struct{}

const batchJSONDepthLimit = 256

var batchResourceName = regexp.MustCompile(`^batches/[A-Za-z0-9_-]+$`)

var batchFileName = regexp.MustCompile(`^files/[A-Za-z0-9_-]+$`)

func validateBatchResources(resources []prompty.BatchResourceRef) error {
	for _, resource := range resources {
		if (resource.Kind != "input" && resource.Kind != "output") || !batchFileName.MatchString(resource.ProviderID) {
			return prompty.ErrBatchScope
		}
	}
	return nil
}

// The observer bounds SDK consumption, not just retained capture data.
type batchSourceBody struct {
	io.ReadCloser

	source *responseSource
}

func (b *batchSourceBody) Read(data []byte) (int, error) {
	if b.source.overflow {
		return 0, prompty.ErrBatchResults
	}
	remaining := b.source.limit - b.source.data.Len()
	if len(data) > remaining {
		data = data[:remaining+1]
	}
	n, err := b.ReadCloser.Read(data)
	_, _ = b.source.data.Write(data[:min(n, remaining)])
	if n > remaining {
		b.source.overflow = true
		return n, prompty.ErrBatchResults
	}
	if err == io.EOF {
		b.source.complete = true
	}
	return n, err
}

func (b *batchSourceBody) Close() error {
	err := b.ReadCloser.Close()
	b.source.closeFailure = err
	return err
}

func observeBatchOperation(ctx context.Context, limit int, call func(context.Context) error) (*responseSource, error) {
	source := &responseSource{limit: limit}
	if limit <= 0 {
		return source, prompty.ErrBatch
	}
	var err error
	func() {
		defer func() {
			if failure := recover(); failure != nil {
				if !source.complete || source.status < 200 || source.status >= 300 {
					panic(failure)
				}
				err = prompty.ErrBatchResults
			}
		}()
		err = call(context.WithValue(ctx, batchSourceKey{}, source))
	}()
	if err != nil || source.closeFailure != nil || !source.complete || source.overflow ||
		source.status < 200 || source.status >= 300 {
		return source, errors.Join(prompty.ErrBatchResults, ctx.Err())
	}
	return source, nil
}

func batchSourceJSON(source *responseSource) prompty.JSONDocument {
	if source == nil || !source.complete || source.overflow || !json.Valid(source.data.Bytes()) {
		return nil
	}
	return prompty.CloneJSONDocument(source.data.Bytes())
}

func parseBatchSource(source *responseSource, previous prompty.BatchHandle) (prompty.BatchHandle, error) {
	handle := prompty.CloneBatchHandle(previous)
	payload := batchSourceJSON(source)
	if len(payload) == 0 || validateBatchJSON(payload) != nil {
		return handle, prompty.ErrBatchResults
	}
	var root map[string]json.RawMessage
	if json.Unmarshal(payload, &root) != nil {
		return handle, prompty.ErrBatchResults
	}
	var name string
	if json.Unmarshal(root["name"], &name) != nil || !batchResourceName.MatchString(name) ||
		(handle.ProviderID != "" && name != handle.ProviderID) {
		return handle, prompty.ErrBatchScope
	}
	handle.ProviderID = name
	var metadata struct {
		State string `json:"state"`
		Model string `json:"model"`
	}
	if json.Unmarshal(root["metadata"], &metadata) != nil || len(handle.Items) == 0 ||
		(metadata.Model != "" && strings.TrimPrefix(metadata.Model, "models/") !=
			strings.TrimPrefix(handle.Items[0].Identity.Model, "models/")) {
		return handle, prompty.ErrBatchResults
	}
	status, err := geminiBatchStatus(metadata.State)
	if err != nil {
		return handle, err
	}
	handle.Status, handle.ProviderStatus = status, metadata.State
	output, outputErr := observedBatchOutput(payload)
	if outputErr != nil {
		return handle, outputErr
	}
	if output.File != "" {
		if !batchFileName.MatchString(output.File) {
			return handle, prompty.ErrBatchScope
		}
		resources := make([]prompty.BatchResourceRef, 0, len(handle.Resources)+1)
		for _, resource := range handle.Resources {
			if resource.Kind == "input" {
				resources = append(resources, resource)
			}
			if resource.Kind == "output" && resource.ProviderID != output.File {
				return handle, prompty.ErrBatchScope
			}
		}
		resources = append(resources, prompty.BatchResourceRef{Kind: "output", ProviderID: output.File})
		handle.Resources = resources
	}
	return handle, nil
}

type batchObservedOutput struct {
	File   string `json:"responsesFile"`
	Inline *struct {
		Records []json.RawMessage `json:"inlinedResponses"`
	} `json:"inlinedResponses"`
}

// REST operations expose response; the pinned SDK fixtures expose metadata.output.
// Accept only an explicitly observed source, never infer one from status or IDs.
func observedBatchOutput(payload []byte) (batchObservedOutput, error) {
	var root struct {
		Response json.RawMessage `json:"response"`
		Metadata struct {
			Output json.RawMessage `json:"output"`
		} `json:"metadata"`
	}
	var output batchObservedOutput
	if json.Unmarshal(payload, &root) != nil {
		return output, prompty.ErrBatchResults
	}
	responsePresent := len(root.Response) > 0 && !bytes.Equal(bytes.TrimSpace(root.Response), []byte("null"))
	metadataPresent := len(root.Metadata.Output) > 0 &&
		!bytes.Equal(bytes.TrimSpace(root.Metadata.Output), []byte("null"))
	if responsePresent && metadataPresent {
		return output, prompty.ErrBatchResults
	}
	raw := root.Metadata.Output
	if responsePresent {
		raw = root.Response
	}
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return output, nil
	}
	if json.Unmarshal(raw, &output) != nil || (output.File != "" && output.Inline != nil) {
		return batchObservedOutput{}, prompty.ErrBatchResults
	}
	return output, nil
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
