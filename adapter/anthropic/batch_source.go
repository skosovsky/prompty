package anthropic

import (
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

// Metadata only; provider body decoding belongs to the SDK.
// SDK-MUTATION-ID in docs/official-sdk-migration.md: no malformed-body ID recovery.
func (s *batchSource) observe(request *http.Request, next option.MiddlewareNext) (*http.Response, error) {
	response, err := next(request)
	if response != nil {
		s.status = response.StatusCode
	}
	return response, err
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
