package anthropic

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
)

const batchMetadataMaxBytes = 4 * 1024 * 1024

type batchSource struct {
	data     []byte
	status   int
	complete bool
}

func (s *batchSource) observe(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	response, err := next(request)
	if err != nil || response == nil {
		return response, err
	}
	s.status = response.StatusCode
	if response.Body == nil {
		return response, prompty.ErrBatchUnknown
	}
	data, readErr := io.ReadAll(io.LimitReader(response.Body, batchMetadataMaxBytes+1))
	closeErr := response.Body.Close()
	s.data, s.complete = data, readErr == nil && len(data) <= batchMetadataMaxBytes
	if !s.complete || closeErr != nil {
		return nil, prompty.ErrBatchUnknown
	}
	response.Body = io.NopCloser(bytes.NewReader(data))
	return response, nil
}

func (s *batchSource) acceptedIdentity() string {
	if !s.complete || s.status < http.StatusOK || s.status >= http.StatusMultipleChoices || !json.Valid(s.data) {
		return ""
	}
	decoder := json.NewDecoder(bytes.NewReader(s.data))
	token, err := decoder.Token()
	if err != nil || token != json.Delim('{') {
		return ""
	}
	count, identity := 0, ""
	for decoder.More() {
		key, keyErr := decoder.Token()
		var value json.RawMessage
		if keyErr != nil || decoder.Decode(&value) != nil {
			return ""
		}
		if key == "id" {
			count++
			if json.Unmarshal(value, &identity) != nil {
				return ""
			}
		}
	}
	if count != 1 || !batchProviderID.MatchString(identity) {
		return ""
	}
	return identity
}

func parseBatchHandle(raw *sdk.MessageBatch, handle *prompty.BatchHandle) error {
	if raw == nil || !uniqueSyntheticJSON([]byte(raw.RawJSON())) || !raw.JSON.ID.Valid() ||
		!batchProviderID.MatchString(raw.ID) ||
		(handle.ProviderID != "" && handle.ProviderID != raw.ID) {
		return prompty.ErrBatchUnknown
	}
	handle.ProviderID = raw.ID
	handle.ProviderStatus = string(raw.ProcessingStatus)
	if !raw.JSON.ProcessingStatus.Valid() || !raw.JSON.Type.Valid() || raw.Type != "message_batch" {
		return prompty.ErrBatchUnknown
	}
	switch raw.ProcessingStatus {
	case sdk.MessageBatchProcessingStatusInProgress:
		handle.Status = prompty.BatchRunning
	case sdk.MessageBatchProcessingStatusCanceling:
		handle.Status = prompty.BatchCanceling
	case sdk.MessageBatchProcessingStatusEnded:
		handle.Status = prompty.BatchEnded
	default:
		return prompty.ErrBatchUnknown
	}
	return nil
}
