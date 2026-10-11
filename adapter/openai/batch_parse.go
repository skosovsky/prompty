package openai

import (
	"bytes"
	"encoding/json"
	"net/http"

	sdk "github.com/openai/openai-go/v3"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func parseOpenAIBatch(raw *sdk.Batch, handle *prompty.BatchHandle) error {
	if raw == nil || !uniqueResponseJSON([]byte(raw.RawJSON())) || !raw.JSON.ID.Valid() ||
		!batchPathID.MatchString(raw.ID) ||
		(handle.ProviderID != "" && handle.ProviderID != raw.ID) {
		return prompty.ErrBatchUnknown
	}
	handle.ProviderID = raw.ID
	if !raw.JSON.Endpoint.Valid() || raw.Endpoint != batchChatEndpoint || !raw.JSON.InputFileID.Valid() ||
		!batchPathID.MatchString(raw.InputFileID) {
		return prompty.ErrBatchScope
	}
	for _, resource := range handle.Resources {
		if resource.Kind == wireInput && resource.ProviderID != raw.InputFileID {
			return prompty.ErrBatchScope
		}
	}
	if !raw.JSON.Object.Valid() || raw.Object != wireBatch || !raw.JSON.Status.Valid() {
		return prompty.ErrBatchUnknown
	}
	var status prompty.BatchStatus
	switch raw.Status {
	case sdk.BatchStatusValidating:
		status = prompty.BatchQueued
	case sdk.BatchStatusInProgress, sdk.BatchStatusFinalizing:
		status = prompty.BatchRunning
	case sdk.BatchStatusCompleted:
		status = prompty.BatchEnded
	case sdk.BatchStatusFailed:
		status = prompty.BatchFailed
	case sdk.BatchStatusExpired:
		status = prompty.BatchExpired
	case sdk.BatchStatusCancelling:
		status = prompty.BatchCanceling
	case sdk.BatchStatusCancelled:
		status = prompty.BatchCanceled
	default:
		return prompty.ErrBatchUnknown
	}
	resources := []prompty.BatchResourceRef{{Kind: wireInput, ProviderID: raw.InputFileID}}
	for _, file := range []struct{ kind, id string }{{wireError, raw.ErrorFileID}, {wireOutput, raw.OutputFileID}} {
		if file.id == "" {
			continue
		}
		if !batchPathID.MatchString(file.id) {
			return prompty.ErrBatchScope
		}
		resources = append(resources, prompty.BatchResourceRef{Kind: file.kind, ProviderID: file.id})
	}
	handle.Status, handle.ProviderStatus, handle.Resources = status, string(raw.Status), resources
	return nil
}

func batchUniqueJSON(document []byte) bool {
	if !json.Valid(document) {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	return uniqueResponseValue(decoder, 0)
}

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *Batches) parseItem(
	raw []byte,
	bindings map[string]prompty.BatchItemBinding,
	prepared map[string]*adapter.PreparedBatchResultItem,
) prompty.BatchItemResult {
	item := prompty.BatchItemResult{
		ID:         "",
		Binding:    nil,
		Response:   nil,
		ErrorKind:  "",
		Outcome:    prompty.BatchItemUnknown,
		SourceJSON: prompty.CloneJSONDocument(raw),
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || json.Unmarshal(fields["custom_id"], &item.ID) != nil || item.ID == "" {
		item.ErrorKind = "invalid_correlation"
		return item
	}
	if binding, exists := bindings[item.ID]; exists {
		item.Binding = &binding
	} else {
		item.ErrorKind = "unexpected_correlation"
		return item
	}
	hasError := len(fields[wireError]) != 0 && !bytes.Equal(bytes.TrimSpace(fields[wireError]), []byte("null"))
	hasResponse := len(fields["response"]) != 0 && !bytes.Equal(bytes.TrimSpace(fields["response"]), []byte("null"))
	if hasError == hasResponse {
		item.ErrorKind = "ambiguous_result"
		return item
	}
	if hasError {
		var failure map[string]json.RawMessage
		if json.Unmarshal(fields[wireError], &failure) != nil || failure == nil {
			item.ErrorKind = "invalid_provider_error"
			return item
		}
		var code string
		_ = json.Unmarshal(failure["code"], &code)
		item.Outcome, item.ErrorKind = prompty.BatchItemError, "provider_error"
		if code == "batch_expired" {
			item.Outcome = prompty.BatchItemExpired
		}
		return item
	}
	var responseFields map[string]json.RawMessage
	var status int
	if json.Unmarshal(fields["response"], &responseFields) != nil ||
		json.Unmarshal(responseFields["status_code"], &status) != nil ||
		status < http.StatusContinue ||
		status > 599 {
		item.ErrorKind = "invalid_response_status"
		return item
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		item.Outcome, item.ErrorKind = prompty.BatchItemError, "provider_http_error"
		return item
	}
	var completion sdk.ChatCompletion
	if json.Unmarshal(responseFields["body"], &completion) != nil ||
		!completion.JSON.ID.Valid() || !completion.JSON.Model.Valid() ||
		!completion.JSON.Object.Valid() ||
		completion.Object != "chat.completion" ||
		completion.ID == "" ||
		completion.Model != item.Binding.Identity.Model {
		item.ErrorKind = "invalid_completion"
		return item
	}
	response, err := adapter.ParseBatchResultItem(b.base, prepared[item.ID], &completion)
	item.Response = response
	if response == nil {
		item.ErrorKind = "invalid_completion"
		return item
	}
	switch response.Outcome {
	case prompty.OutcomeRefusal:
		item.Outcome = prompty.BatchItemRefusal
	case prompty.OutcomeIncomplete:
		item.Outcome = prompty.BatchItemIncomplete
	case prompty.OutcomeCompleted, prompty.OutcomeToolCalls, prompty.OutcomePaused:
		fallthrough
	default:
		item.Outcome = prompty.BatchItemSuccess
	}
	if err != nil {
		item.ErrorKind = "validation_failed"
		if item.Outcome != prompty.BatchItemRefusal {
			item.Outcome = prompty.BatchItemIncomplete
		}
	}
	return item
}
