package gemini

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"math"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

var _ prompty.BatchManager = (*Batches)(nil)

//nolint:cyclop,funlen,gocognit,gocyclo,nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (b *Batches) Results(
	ctx context.Context,
	handle prompty.BatchHandle,
	options prompty.BatchResultsOptions,
) (*prompty.BatchResult, error) {
	handle = prompty.CloneBatchHandle(handle)
	if err := prompty.ValidateBatchHandle(handle); err != nil {
		return nil, err
	}
	if handle.Provider != wireGemini || handle.Endpoint != b.base.RuntimeDescriptor().Endpoint ||
		handle.AccountBinding != b.account || !batchResourceName.MatchString(handle.ProviderID) {
		return nil, prompty.ErrBatchScope
	}
	if err := validateBatchResources(handle.Resources); err != nil {
		return nil, err
	}
	if b.base.responseClient == nil {
		return nil, adapter.ErrNoClient
	}
	if b.base.responseClient.ClientConfig().Backend != genai.BackendGeminiAPI {
		return nil, adapter.ErrUnsupportedCapability
	}
	if options.MaxBytes < 0 || options.MaxBytes > math.MaxInt || options.MaxLineBytes < 0 {
		return nil, prompty.ErrBatch
	}
	if handle.Status != prompty.BatchEnded && handle.Status != prompty.BatchFailed &&
		handle.Status != prompty.BatchExpired && handle.Status != prompty.BatchCanceled {
		return nil, prompty.ErrBatchUnavailable
	}
	if len(options.Bindings) != len(handle.Items) {
		return nil, prompty.ErrBatchScope
	}
	inputs := make(map[string]*prompty.PromptExecution)
	for _, binding := range options.Bindings {
		if binding.Execution == nil || inputs[binding.ID] != nil {
			return nil, prompty.ErrBatchScope
		}
		inputs[binding.ID] = binding.Execution
	}
	prepared := make(map[string]*adapter.PreparedBatchResultItem)
	bindings := make(map[string]prompty.BatchItemBinding)
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
	for _, binding := range handle.Items {
		if inputs[binding.ID] == nil {
			return result, prompty.ErrBatchScope
		}
		original := inputs[binding.ID].Clone()
		if original.ModelOptions == nil {
			original.ModelOptions = &prompty.ModelOptions{
				Model:            "",
				Temperature:      nil,
				MaxTokens:        nil,
				TopP:             nil,
				Stop:             nil,
				ProviderSettings: nil,
			}
		}
		if original.ModelOptions.Model == "" {
			original.ModelOptions.Model = b.base.defaultModel
		}
		digest, err := prompty.BatchExecutionDigest(original)
		if err != nil || digest != binding.ExecutionDigest ||
			binding.Identity.Model != handle.Items[0].Identity.Model {
			return result, prompty.ErrBatchScope
		}
		item, err := adapter.PrepareBatchResultItem(b.base, original)
		if err != nil {
			return result, err
		}
		if item.Diagnostics.Profile != binding.Identity {
			return result, prompty.ErrBatchScope
		}
		prepared[binding.ID], bindings[binding.ID] = item, binding
		result.Diagnostics = append(result.Diagnostics, item.Diagnostics)
	}
	if len(handle.Resources) > 0 {
		return b.fileResults(ctx, result, options, bindings, prepared)
	}
	limit := b.base.responseLimit
	if options.MaxBytes > 0 {
		limit = int(options.MaxBytes)
	}
	if limit <= 0 {
		return result, prompty.ErrBatch
	}
	lineLimit := options.MaxLineBytes
	if lineLimit == 0 {
		lineLimit = defaultResponsePayloadLimit
	}
	capture, err := adapter.NewBatchCapture(
		"gemini.batch.results",
		"google.golang.org/genai",
		handle.Items[0].Identity,
		options.Options,
	)
	if err != nil {
		return result, err
	}
	if err = capture.Before(ctx, handle, map[string]any{wireName: handle.ProviderID}); err != nil {
		return capture.Finish(ctx, result, err)
	}
	if err = ctx.Err(); err != nil {
		return capture.Finish(ctx, result, err)
	}
	capture.MarkDispatched()
	source, operationErr := observeBatchOperation(ctx, limit, func(callCtx context.Context) error {
		_, callErr := b.base.responseClient.Batches.Get(callCtx, handle.ProviderID, nil)
		return callErr
	})
	downloaded := false
	// A converter failure on one malformed item must not discard other source-
	// proven items. Reparse only a complete, bounded, successful native response;
	// transport/cancel/close failures do not become successful retrieval.
	if operationErr == nil || (ctx.Err() == nil && source.complete && !source.overflow &&
		source.closeFailure == nil && source.status >= 200 && source.status < 300 && batchSourceJSON(source) != nil) {
		var observed prompty.BatchHandle
		observed, operationErr = parseBatchSource(source, handle)
		if operationErr == nil {
			handle = observed
			var records []json.RawMessage
			records, operationErr = inlineBatchRecords(batchSourceJSON(source))
			if handle.Status != prompty.BatchEnded && handle.Status != prompty.BatchFailed &&
				handle.Status != prompty.BatchExpired && handle.Status != prompty.BatchCanceled {
				operationErr = prompty.ErrBatchUnavailable
			}
			if operationErr == nil {
				downloaded = true
				for _, record := range records {
					if len(record) > lineLimit {
						operationErr, downloaded = prompty.ErrBatchResults, false
						break
					}
					item, parseErr := b.parseInlineItem(record, bindings, prepared)
					if item.ID != "" {
						result.Items = append(result.Items, item)
					}
					if parseErr != nil {
						operationErr, downloaded = parseErr, false
						break
					}
				}
			}
		}
	}
	result.Completeness = prompty.ReconcileBatchResults(handle, result.Items, downloaded)
	providerErr := capture.Provider(ctx, batchSourceJSON(source), source.status, source.complete && !source.overflow)
	return capture.Finish(ctx, result, errors.Join(operationErr, providerErr))
}

func inlineBatchRecords(payload prompty.JSONDocument) ([]json.RawMessage, error) {
	output, err := observedBatchOutput(payload)
	if err != nil {
		return nil, err
	}
	if output.File != "" || output.Inline == nil || output.Inline.Records == nil {
		return nil, prompty.ErrBatchUnavailable
	}
	return output.Inline.Records, nil
}

func (b *Batches) parseInlineItem(
	payload json.RawMessage,
	bindings map[string]prompty.BatchItemBinding,
	prepared map[string]*adapter.PreparedBatchResultItem,
) (prompty.BatchItemResult, error) {
	item := prompty.BatchItemResult{
		ID:         "",
		Binding:    nil,
		Response:   nil,
		ErrorKind:  "",
		SourceJSON: prompty.CloneJSONDocument(payload),
		Outcome:    prompty.BatchItemUnknown,
	}
	var record struct {
		Metadata struct {
			Key string `json:"key"`
		} `json:"metadata"`
		Response json.RawMessage `json:"response"`
		Error    json.RawMessage `json:"error"`
	}
	if json.Unmarshal(payload, &record) != nil || record.Metadata.Key == "" {
		return item, prompty.ErrBatchResults
	}
	item.ID = record.Metadata.Key
	binding, known := bindings[item.ID]
	if !known {
		return item, nil
	}
	item.Binding = &binding
	hasResponse := len(record.Response) > 0 && string(record.Response) != wireNull
	hasError := len(record.Error) > 0 && string(record.Error) != wireNull
	if hasResponse == hasError {
		return item, prompty.ErrBatchResults
	}
	if hasError {
		var providerError map[string]json.RawMessage
		if json.Unmarshal(record.Error, &providerError) != nil || providerError == nil {
			return item, prompty.ErrBatchResults
		}
		item.Outcome, item.ErrorKind = prompty.BatchItemError, "provider_error"
		return item, nil
	}
	var raw genai.GenerateContentResponse
	decoder := json.NewDecoder(bytes.NewReader(record.Response))
	decoder.UseNumber()
	if decoder.Decode(&raw) != nil {
		return item, prompty.ErrBatchResults
	}
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	raw.SDKHTTPResponse = &genai.HTTPResponse{
		Body: string(record.Response),
	}
	response, parseErr := adapter.ParseBatchResultItem(b.base, prepared[item.ID], &raw)
	item.Response = response
	if response == nil {
		item.Outcome, item.ErrorKind = prompty.BatchItemIncomplete, "invalid_response"
		return item, nil
	}
	switch response.Outcome {
	case prompty.OutcomeRefusal:
		item.Outcome = prompty.BatchItemRefusal
	case prompty.OutcomeCompleted, prompty.OutcomeToolCalls:
		item.Outcome = prompty.BatchItemSuccess
	case prompty.OutcomeIncomplete:
		item.Outcome = prompty.BatchItemIncomplete
	case prompty.OutcomePaused:
		fallthrough
	default:
		item.Outcome = prompty.BatchItemUnknown
	}
	if parseErr != nil {
		item.ErrorKind = "contract_validation"
		if item.Outcome != prompty.BatchItemRefusal {
			item.Outcome = prompty.BatchItemIncomplete
		}
	}
	return item, nil
}
