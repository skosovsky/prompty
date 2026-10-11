package gemini

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const geminiPartCodec = "gemini.part.json"

// ParseResponseForProfile preserves ordered provider parts and request-scoped signatures.
func (a *Adapter) ParseResponseForProfile(
	raw *genai.GenerateContentResponse,
	destination prompty.ProfileIdentity,
) (*prompty.Response, error) {
	if raw != nil {
		encoded, err := json.Marshal(raw)
		if err != nil {
			// Preserve safe domain validation categories without retaining marshal causes.
			_, parseErr := parseGeminiResponse(raw, destination, 0)
			return sdkUsageFailure(raw), errors.Join(prompty.ErrProviderState, parseErr)
		}
		if a.responseLimit <= 0 || len(encoded) > a.responseLimit {
			return sdkUsageFailure(raw), prompty.ErrProviderState
		}
	}
	response, err := parseGeminiResponse(raw, destination, 0)
	if err != nil && response == nil {
		response = sdkUsageFailure(raw)
	}
	return response, err
}

//nolint:funlen,gocognit,nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func parseGeminiResponse(
	raw *genai.GenerateContentResponse, destination prompty.ProfileIdentity, previousGroundingChunks int,
) (*prompty.Response, error) {
	if raw == nil {
		return nil, adapter.ErrInvalidResponse
	}
	if len(raw.Candidates) == 0 {
		if raw.PromptFeedback != nil && raw.PromptFeedback.BlockReason != "" {
			return &prompty.Response{
				Content:                 nil,
				ProviderState:           nil,
				Annotations:             nil,
				MessageAnnotations:      nil,
				ContinuationUnavailable: false,
				Diagnostics: prompty.ExecutionDiagnostics{
					Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
					Snapshot:   "",
					Provenance: "",
					Override:   false,
					Checks:     nil,
					Schemas:    nil,
					Capture:    nil,
					Cache:      nil,
				},
				Model:        raw.ModelVersion,
				Outcome:      prompty.OutcomeRefusal,
				FinishReason: string(raw.PromptFeedback.BlockReason),
				Usage:        geminiUsage(raw),
			}, nil
		}
		return nil, adapter.ErrEmptyResponse
	}
	if len(raw.Candidates) != 1 || raw.Candidates[0] == nil {
		return nil, adapter.ErrInvalidResponse
	}
	candidate := raw.Candidates[0]
	if err := validateGeminiContentRole(candidate.Content); err != nil {
		return nil, err
	}
	if _, err := validateCandidateMetadata(candidate, previousGroundingChunks); err != nil {
		return nil, err
	}
	response := &prompty.Response{
		Content:                 nil,
		ProviderState:           nil,
		Annotations:             nil,
		MessageAnnotations:      nil,
		ContinuationUnavailable: false,
		Diagnostics: prompty.ExecutionDiagnostics{
			Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
			Snapshot:   "",
			Provenance: "",
			Override:   false,
			Checks:     nil,
			Schemas:    nil,
			Capture:    nil,
			Cache:      nil,
		},
		Model:        raw.ModelVersion,
		Outcome:      geminiOutcome(candidate.FinishReason),
		FinishReason: string(candidate.FinishReason),
		Usage:        geminiUsage(raw),
	}
	if candidate.Content != nil {
		for sourceIndex, sdkPart := range candidate.Content.Parts {
			part, sourceErr := originalGeminiPart(raw, sourceIndex, sdkPart)
			if sourceErr != nil {
				return partialGeminiFailure(response, sourceErr)
			}
			canonical, err := canonicalGeminiPart(part)
			if err != nil {
				return partialGeminiFailure(response, err)
			}
			index := len(response.Content)
			response.Content = append(response.Content, canonical)
			if len(part.ThoughtSignature) != 0 {
				payload, err := encodeExactGeminiPart(part)
				if err != nil {
					return nil, fmt.Errorf("%w: part encoding", prompty.ErrProviderState)
				}
				response.ProviderState = append(
					response.ProviderState,
					prompty.PartState{PartIndex: index, Envelope: prompty.ProviderEnvelope{ExpiresAt: nil,
						Format:   prompty.ProviderStateFormat,
						Scope:    destination,
						Codec:    geminiPartCodec,
						Payload:  payload,
						Required: true,
					}},
				)
			}
		}
	}
	history := []prompty.ChatMessage{
		{
			ProviderState:           nil,
			Annotations:             nil,
			MessageAnnotations:      nil,
			ContinuationUnavailable: false,
			CachePolicy:             nil,
			Provenance:              nil,
			Metadata:                nil,
			LayerKind:               "",
			Role:                    prompty.RoleAssistant,
			Content:                 response.Content,
		},
	}
	if err := validateGeminiCorrelation(history); err != nil {
		return partialGeminiFailure(response, err)
	}
	if len(response.Content) == 0 && response.Outcome != prompty.OutcomeRefusal && response.FinishReason == "" &&
		candidate.CitationMetadata == nil && candidate.GroundingMetadata == nil {
		return nil, adapter.ErrEmptyResponse
	}
	if response.Outcome == prompty.OutcomeCompleted {
		for _, part := range response.Content {
			if _, ok := part.(prompty.ToolCallPart); ok {
				response.Outcome = prompty.OutcomeToolCalls
				break
			}
		}
	}
	for _, annotation := range []struct {
		kind     string
		metadata any
	}{
		{"candidate_citation_metadata", candidate.CitationMetadata}, {"candidate_grounding_metadata", candidate.GroundingMetadata},
	} {
		if reflect.ValueOf(annotation.metadata).IsNil() {
			continue
		}
		value, err := json.Marshal(annotation.metadata)
		if err != nil {
			return nil, adapter.ErrInvalidResponse
		}
		response.MessageAnnotations = append(
			response.MessageAnnotations,
			prompty.MessageAnnotation{Scope: nil, Required: false, Kind: annotation.kind, Value: value},
		)
	}

	return response, nil
}

func canonicalGeminiPart(part *genai.Part) (prompty.ContentPart, error) {
	if part == nil {
		return nil, adapter.ErrInvalidResponse
	}
	if err := validateGeminiPartShape(part); err != nil {
		return nil, err
	}
	switch {
	case part.FunctionCall != nil:
		args := "{}"
		if part.FunctionCall.Args != nil {
			value, err := json.Marshal(part.FunctionCall.Args)
			if err != nil {
				return nil, fmt.Errorf("%w: %w", adapter.ErrMalformedArgs, err)
			}
			args = string(value)
		}
		return prompty.ToolCallPart{
			ArgsChunk:   "",
			CachePolicy: nil,
			ID:          part.FunctionCall.ID,
			Name:        part.FunctionCall.Name,
			Args:        args,
		}, nil
	case part.InlineData != nil:
		return prompty.MediaPart{URL: "", CachePolicy: nil,
			MediaType: geminiMediaType(part.InlineData.MIMEType),
			MIMEType:  part.InlineData.MIMEType,
			Data:      bytes.Clone(part.InlineData.Data),
		}, nil
	case part.FileData != nil:
		return prompty.MediaPart{Data: nil, CachePolicy: nil,
			MediaType: geminiMediaType(part.FileData.MIMEType),
			MIMEType:  part.FileData.MIMEType,
			URL:       part.FileData.FileURI,
		}, nil
	case part.Thought:
		return prompty.ReasoningPart{CachePolicy: nil, Text: part.Text}, nil
	case part.Text != "":
		return prompty.TextPart{CachePolicy: nil, Text: part.Text}, nil
	case len(part.ThoughtSignature) != 0:
		return prompty.ReasoningPart{}, nil
	default:
		if reflect.DeepEqual(
			part,
			//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
			&genai.Part{},
		) {
			return nil, adapter.ErrEmptyResponse
		}
		return nil, adapter.ErrUnsupportedContentType
	}
}

func validateGeminiPartShape(part *genai.Part) error {
	variants := 0
	for _, present := range []bool{part.FunctionCall != nil, part.InlineData != nil, part.FileData != nil,
		part.Text != "" || part.Thought} {
		if present {
			variants++
		}
	}
	if variants > 1 {
		return adapter.ErrInvalidResponse
	}
	if part.CodeExecutionResult != nil || part.ExecutableCode != nil || part.FunctionResponse != nil ||
		part.MediaResolution != nil || part.VideoMetadata != nil {
		return adapter.ErrUnsupportedContentType
	}
	if part.FunctionCall != nil && (len(part.FunctionCall.PartialArgs) != 0 || part.FunctionCall.WillContinue != nil) {
		return adapter.ErrUnsupportedContentType
	}
	if (part.InlineData != nil && part.InlineData.DisplayName != "") ||
		(part.FileData != nil && part.FileData.DisplayName != "") {
		return adapter.ErrUnsupportedContentType
	}
	return nil
}

func geminiMediaType(mime string) string {
	mediaType, _, _ := strings.Cut(mime, "/")
	switch mediaType {
	case "image", "audio", "video":
		return mediaType
	default:
		return "document"
	}
}

func geminiOutcome(reason genai.FinishReason) prompty.Outcome {
	switch reason {
	case genai.FinishReasonStop:
		return prompty.OutcomeCompleted
	case genai.FinishReasonSafety, genai.FinishReasonRecitation, genai.FinishReasonBlocklist,
		genai.FinishReasonProhibitedContent, genai.FinishReasonSPII, genai.FinishReasonImageSafety,
		genai.FinishReasonImageProhibitedContent:
		return prompty.OutcomeRefusal
	case genai.FinishReasonUnspecified,
		genai.FinishReasonMaxTokens,
		genai.FinishReasonLanguage,
		genai.FinishReasonOther,
		genai.FinishReasonMalformedFunctionCall,
		genai.FinishReasonUnexpectedToolCall,
		genai.FinishReasonTooManyToolCalls,
		genai.FinishReasonNoImage,
		genai.FinishReasonImageRecitation,
		genai.FinishReasonImageOther,
		genai.FinishReasonContinuation:
		fallthrough
	default:
		return prompty.OutcomeIncomplete
	}
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func validateGeminiPayloads(messages []prompty.ChatMessage) error {
	for _, message := range messages {
		if hasPartialState(message) {
			if _, err := orderedPartialMessage(message); err != nil {
				return err
			}
		}
		for _, state := range message.ProviderState {
			if state.Envelope.Codec == geminiPartialPartsCodec {
				parts, canonical, complete, err := decodePartialState(state)
				if err != nil || !complete || message.Role != prompty.RoleAssistant || !state.Envelope.Required ||
					!reflect.DeepEqual(canonical, message.Content[state.PartIndex]) {
					return prompty.ErrProviderState
				}
				for _, part := range parts {
					if validateGeminiSDKStateEncoding(part) != nil {
						return prompty.ErrProviderState
					}
				}
				continue
			}
			if message.Role != prompty.RoleAssistant || state.Envelope.Codec != geminiPartCodec ||
				!state.Envelope.Required {
				return prompty.ErrProviderState
			}
			part, err := decodeGeminiState(state)
			if err != nil {
				return err
			}
			if encodingErr := validateGeminiSDKStateEncoding(part); encodingErr != nil {
				return encodingErr
			}
			canonical, err := canonicalGeminiPart(part)
			if err != nil {
				return err
			}
			if !reflect.DeepEqual(canonical, message.Content[state.PartIndex]) {
				return fmt.Errorf("%w: signed content changed", prompty.ErrProviderState)
			}
		}
	}
	return nil
}

func decodeGeminiState(state prompty.PartState) (*genai.Part, error) {
	payload := state.Envelope.Payload
	if !json.Valid(payload) || uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil {
		return nil, prompty.ErrProviderState
	}
	var part genai.Part
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(&part); err != nil || len(part.ThoughtSignature) == 0 {
		return nil, prompty.ErrProviderState
	}
	reencoded, err := encodeExactGeminiPart(&part)
	if err != nil || !reflect.DeepEqual(geminiJSONValue(payload), geminiJSONValue(reencoded)) {
		return nil, prompty.ErrProviderState
	}
	return &part, nil
}

func geminiJSONValue(payload []byte) any {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var value any
	// Callers supply either json.Valid input or successfully marshaled SDK data.
	if err := decoder.Decode(&value); err != nil {
		return nil
	}
	return value
}

func uniqueGeminiJSONValue(decoder *json.Decoder) error {
	decoder.UseNumber()
	token, tokenErr := decoder.Token()
	if tokenErr != nil {
		return prompty.ErrProviderState
	}
	delimiter, container := token.(json.Delim)
	if !container {
		return nil
	}
	if delimiter == '{' {
		return uniqueGeminiJSONObject(decoder)
	}
	if delimiter != '[' {
		return prompty.ErrProviderState
	}
	for decoder.More() {
		if err := uniqueGeminiJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim(']') {
		return prompty.ErrProviderState
	}
	return nil
}

func uniqueGeminiJSONObject(decoder *json.Decoder) error {
	keys := make(map[string]bool)
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return prompty.ErrProviderState
		}
		key, ok := token.(string)
		if !ok || keys[key] {
			return prompty.ErrProviderState
		}
		keys[key] = true
		if err := uniqueGeminiJSONValue(decoder); err != nil {
			return err
		}
	}
	closing, err := decoder.Token()
	if err != nil || closing != json.Delim('}') {
		return prompty.ErrProviderState
	}
	return nil
}

func (a *Adapter) destinationIdentity(exec *prompty.PromptExecution) prompty.ProfileIdentity {
	desc := a.RuntimeDescriptor()
	identity := prompty.ProfileIdentity{
		ModelFamily: "",
		Provider:    desc.Name,
		Endpoint:    desc.Endpoint,
		Model:       desc.DefaultModel,
	}
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		identity.Model = exec.ModelOptions.Model
	}
	for _, profile := range desc.Profiles {
		if profile.Identity.Provider == identity.Provider && profile.Identity.Endpoint == identity.Endpoint &&
			profile.Identity.Model == identity.Model {
			identity.ModelFamily = profile.Identity.ModelFamily
			break
		}
	}
	if override := exec.Requirements.Override; override != nil && override.Identity.Provider == identity.Provider &&
		override.Identity.Endpoint == identity.Endpoint && override.Identity.Model == identity.Model {
		identity.ModelFamily = override.Identity.ModelFamily
	}
	return identity
}

func (a *Adapter) assistantMessageContent(message prompty.ChatMessage) (*genai.Content, error) {
	if hasPartialState(message) {
		parts, err := orderedPartialMessage(message)
		if err != nil {
			return nil, err
		}
		return &genai.Content{Role: genai.RoleModel, Parts: parts}, nil
	}
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	content := &genai.Content{
		Role: genai.RoleModel,
	}
	states := make(map[int]prompty.PartState, len(message.ProviderState))
	for _, state := range message.ProviderState {
		states[state.PartIndex] = state
	}
	for index, part := range message.Content {
		if state, exists := states[index]; exists {
			if state.Envelope.Codec == geminiPartialPartsCodec {
				parts, _, complete, err := decodePartialState(state)
				if err != nil || !complete {
					return nil, prompty.ErrProviderState
				}
				content.Parts = append(content.Parts, parts...)
				continue
			}
			original, err := decodeGeminiState(state)
			if err != nil {
				return nil, err
			}
			content.Parts = append(content.Parts, original)
			continue
		}
		converted, err := a.assistantContent([]prompty.ContentPart{part})
		if err != nil {
			return nil, err
		}
		content.Parts = append(content.Parts, converted.Parts...)
	}
	return content, nil
}

// SDK scalar fields lack presence; nonzero values are known, zero values remain
// unknown unless a valid retained HTTP body reports the matching numeric value.
// SDK-GENAI-SOURCE in docs/official-sdk-migration.md: scalar zero has no
// SDK presence evidence; caller Body cannot restore original-provider fidelity.
func geminiUsage(raw *genai.GenerateContentResponse) prompty.Usage {
	var usage prompty.Usage
	if raw.UsageMetadata == nil {
		return usage
	}
	metadata := raw.UsageMetadata
	usage.Mode = prompty.UsageCumulative
	for _, field := range []struct {
		key         string
		counter     prompty.UsageCounter
		value       int32
		destination *int
	}{
		{"promptTokenCount", prompty.UsagePrompt, metadata.PromptTokenCount, &usage.PromptTokens},
		{"candidatesTokenCount", prompty.UsageCompletion, metadata.CandidatesTokenCount, &usage.CompletionTokens},
		{"totalTokenCount", prompty.UsageTotal, metadata.TotalTokenCount, &usage.TotalTokens},
		{"cachedContentTokenCount", prompty.UsageCacheRead, metadata.CachedContentTokenCount, &usage.PromptTokensCached},
		{"thoughtsTokenCount", prompty.UsageReasoning, metadata.ThoughtsTokenCount, &usage.CompletionTokensReasoning},
	} {
		if field.value != 0 {
			*field.destination = int(field.value)
			usage.Known = append(usage.Known, field.counter)
		}
	}
	return usage
}

func partialGeminiFailure(response *prompty.Response, err error) (*prompty.Response, error) {
	if len(response.Content) == 0 && len(response.Usage.Known) == 0 {
		return nil, err
	}
	response.Outcome = "" // Unknown: failed decoding cannot confirm completion.
	response.FinishReason = ""
	response.ContinuationUnavailable = true
	return response, err
}

func validateGeminiContentRole(content *genai.Content) error {
	if content != nil && content.Role != "" && content.Role != wireModel {
		return adapter.ErrInvalidResponse
	}
	return nil
}
