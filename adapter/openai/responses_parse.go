package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func parseRemoteResponse(
	raw *responses.Response,
	handle prompty.RemoteResponseHandle,
	result *prompty.RemoteResponseResult,
	schema *prompty.CompiledProviderSchema,
	tools responseToolContract,
) (*prompty.RemoteResponseResult, error) {
	if raw == nil {
		return result, prompty.ErrRemoteResponseUnknown
	}
	// Safe identity can remain reconcilable after malformed accepted output.
	if responseResourceID.MatchString(raw.ID) && (handle.ProviderID == "" || handle.ProviderID == raw.ID) {
		if handle.ProviderID == "" {
			handle.Status = prompty.RemoteResponseUnknown
		}
		handle.ProviderID = raw.ID
		result.Handle = &handle
	}
	if !uniqueResponseJSON([]byte(raw.RawJSON())) || !raw.JSON.ID.Valid() || !raw.JSON.Model.Valid() ||
		!raw.JSON.Status.Valid() || handle.ProviderID == "" || raw.ID != handle.ProviderID || raw.Model != handle.Identity.Model {
		return result, prompty.ErrRemoteResponseUnknown
	}
	status := prompty.RemoteResponseStatus(raw.Status)
	if raw.Status == responses.ResponseStatusCancelled {
		status = prompty.RemoteResponseCanceled
	}
	switch status {
	case prompty.RemoteResponseQueued, prompty.RemoteResponseRunning, prompty.RemoteResponseCompleted,
		prompty.RemoteResponseFailed, prompty.RemoteResponseIncomplete, prompty.RemoteResponseCanceled:
		handle.Status = status
	default:
		return result, prompty.ErrRemoteResponseUnknown
	}
	result.Handle = &handle
	response := prompty.NewResponse(nil)
	response.Diagnostics = prompty.CloneExecutionDiagnostics(result.Diagnostics)
	response.FinishReason = string(raw.Status)
	if handle.Status != prompty.RemoteResponseCompleted {
		response.Outcome = prompty.OutcomeIncomplete
	}
	result.Response = response
	usage, usageErr := responseUsage(raw.Usage)
	response.Usage = usage
	parseErr := parseRemoteItems(raw.Output, response, handle.HostedTools, handle.Identity, tools)
	if parseErr != nil && response.Outcome != prompty.OutcomeRefusal {
		response.Outcome = prompty.OutcomeIncomplete
	}
	metadataErr := prompty.ValidateResponseMetadata(response)
	if (handle.ProviderStateRequired || responseRequiresState(result.Diagnostics)) && response.ContinuationUnavailable {
		metadataErr = errors.Join(metadataErr, prompty.ErrStateUnavailable)
	}
	if handle.Status == prompty.RemoteResponseQueued || handle.Status == prompty.RemoteResponseRunning {
		return result, errors.Join(usageErr, parseErr, metadataErr)
	}
	if response.Outcome != prompty.OutcomeCompleted && response.Outcome != prompty.OutcomeToolCalls {
		return result, errors.Join(usageErr, parseErr, metadataErr,
			&prompty.OutcomeError{Outcome: response.Outcome, FinishReason: response.FinishReason})
	}
	if len(response.Content) == 0 {
		return result, errors.Join(usageErr, parseErr, metadataErr, adapter.ErrInvalidResponse)
	}
	selectionErr := validateResponseToolSelection(response, tools)
	if parseErr == nil && selectionErr == nil && handle.Status == prompty.RemoteResponseCompleted {
		handle.ToolCalls = nil
		if response.Outcome == prompty.OutcomeToolCalls {
			handle.ToolCalls = make(map[string]string)
			for _, part := range response.Content {
				if call, ok := part.(prompty.ToolCallPart); ok {
					handle.ToolCalls[call.ID] = call.Name
				}
			}
		}
	}
	if response.Outcome == prompty.OutcomeToolCalls {
		return result, errors.Join(usageErr, parseErr, metadataErr, selectionErr)
	}
	return result, errors.Join(usageErr, parseErr, metadataErr, selectionErr, validateRemoteOutput(schema, response))
}

func parseRemoteItems(
	items []responses.ResponseOutputItemUnion,
	response *prompty.Response,
	hosted []string,
	profile prompty.ProfileIdentity,
	tools responseToolContract,
) error {
	seen := make(map[string]bool)
	callIDs := make(map[string]bool)
	for index, item := range items {
		if (strings.TrimSpace(item.ID) == "" && item.Type != "function_call") ||
			(item.ID != "" && seen[item.ID]) {
			return prompty.ErrRemoteResponse
		}
		if item.ID != "" {
			seen[item.ID] = true
		}
		if item.Type == "function_call" {
			if err := parseResponseFunction([]byte(item.RawJSON()), response, profile, tools, callIDs); err != nil {
				return err
			}
			continue
		}
		if item.Type == "reasoning" {
			if err := preserveResponseReasoning([]byte(item.RawJSON()), response, profile); err != nil {
				return err
			}
			continue
		}
		if item.Type == "web_search_call" || item.Type == "file_search_call" {
			if err := preserveHostedItem(index, item.RawJSON(), hosted, response); err != nil {
				return err
			}
			continue
		}
		if item.Type != "message" || item.Role != "assistant" {
			return prompty.ErrRemoteResponse
		}
		if item.Status != "completed" && item.Status != "in_progress" && item.Status != "incomplete" {
			return prompty.ErrRemoteResponse
		}
		if item.Status != "completed" &&
			(response.Outcome == prompty.OutcomeCompleted || response.Outcome == prompty.OutcomeToolCalls) {
			response.Outcome = prompty.OutcomeIncomplete
		}
		for _, content := range item.Content {
			index := len(response.Content)
			switch content.Type {
			case "output_text":
				response.Content = append(response.Content, prompty.TextPart{Text: content.Text})
				for _, annotation := range content.Annotations {
					response.Annotations = append(response.Annotations, prompty.PartAnnotation{
						PartIndex: index, Kind: "openai.responses.annotation", Value: []byte(annotation.RawJSON()),
					})
				}
			case "refusal":
				response.Outcome = prompty.OutcomeRefusal
				response.Content = append(response.Content, prompty.TextPart{Text: content.Refusal})
			default:
				return prompty.ErrRemoteResponse
			}
		}
	}
	return preserveHostedHistory(items, response, profile)
}

func responseUsage(usage responses.ResponseUsage) (prompty.Usage, error) {
	var out prompty.Usage
	out.Mode = prompty.UsageCumulative
	var failure error
	for _, field := range []struct {
		counter     prompty.UsageCounter
		present     bool
		value       int64
		destination *int
	}{
		{prompty.UsagePrompt, usage.JSON.InputTokens.Valid(), usage.InputTokens, &out.PromptTokens},
		{prompty.UsageCompletion, usage.JSON.OutputTokens.Valid(), usage.OutputTokens, &out.CompletionTokens},
		{prompty.UsageTotal, usage.JSON.TotalTokens.Valid(), usage.TotalTokens, &out.TotalTokens},
		{prompty.UsageCacheRead, usage.InputTokensDetails.JSON.CachedTokens.Valid(), usage.InputTokensDetails.CachedTokens, &out.PromptTokensCached},
		{prompty.UsageReasoning, usage.OutputTokensDetails.JSON.ReasoningTokens.Valid(), usage.OutputTokensDetails.ReasoningTokens, &out.CompletionTokensReasoning},
	} {
		if !field.present {
			continue
		}
		value, err := adapter.SumUsageCounters(field.value)
		if err != nil {
			failure = errors.Join(failure, err)
			continue
		}
		*field.destination = value
		out.Known = append(out.Known, field.counter)
	}
	return out, failure
}

func uniqueResponseJSON(document []byte) bool {
	if !json.Valid(document) || len(document) > 16*1024*1024 {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	return uniqueResponseValue(decoder, 0)
}

const remoteResponseMaxDepth = 256

func uniqueResponseValue(decoder *json.Decoder, depth int) bool {
	if depth > remoteResponseMaxDepth {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	if token != json.Delim('{') && token != json.Delim('[') {
		return true
	}
	keys := make(map[string]bool)
	for decoder.More() {
		if token == json.Delim('{') {
			key, keyErr := decoder.Token()
			name, ok := key.(string)
			if keyErr != nil || !ok || keys[name] {
				return false
			}
			keys[name] = true
		}
		if !uniqueResponseValue(decoder, depth+1) {
			return false
		}
	}
	_, err = decoder.Token()
	return err == nil
}
