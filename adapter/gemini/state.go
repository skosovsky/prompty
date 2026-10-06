package gemini

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const geminiPartCodec = "gemini.part.json"

// ParseResponseForProfile preserves ordered provider parts and request-scoped signatures.
func (*Adapter) ParseResponseForProfile(
	raw *genai.GenerateContentResponse,
	destination prompty.ProfileIdentity,
) (*prompty.Response, error) {
	return parseGeminiResponse(raw, destination, 0)
}

func parseGeminiResponse(
	raw *genai.GenerateContentResponse, destination prompty.ProfileIdentity, previousGroundingChunks int,
) (*prompty.Response, error) {
	if raw == nil {
		return nil, adapter.ErrInvalidResponse
	}
	if len(raw.Candidates) == 0 {
		if raw.PromptFeedback != nil && raw.PromptFeedback.BlockReason != "" {
			return &prompty.Response{
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
	if _, err := validateCandidateMetadata(candidate, previousGroundingChunks); err != nil {
		return nil, err
	}
	response := &prompty.Response{
		Outcome:      geminiOutcome(candidate.FinishReason),
		FinishReason: string(candidate.FinishReason),
		Usage:        geminiUsage(raw),
	}
	if candidate.Content != nil {
		for sourceIndex, sdkPart := range candidate.Content.Parts {
			part, sourceErr := originalSignedGeminiPart(raw, sourceIndex, sdkPart)
			if sourceErr != nil {
				return nil, sourceErr
			}
			canonical, err := canonicalGeminiPart(part)
			if err != nil {
				return nil, err
			}
			index := len(response.Content)
			response.Content = append(response.Content, canonical)
			if len(part.ThoughtSignature) != 0 {
				payload, err := json.Marshal(part)
				if err != nil {
					return nil, fmt.Errorf("%w: part encoding", prompty.ErrProviderState)
				}
				response.ProviderState = append(
					response.ProviderState,
					prompty.PartState{PartIndex: index, Envelope: prompty.ProviderEnvelope{
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
			prompty.MessageAnnotation{Kind: annotation.kind, Value: value},
		)
	}
	if raw.SDKHTTPResponse != nil && raw.SDKHTTPResponse.Body != "" &&
		(candidate.CitationMetadata != nil || candidate.GroundingMetadata != nil) {
		annotations, err := sourceCandidateAnnotations([]byte(raw.SDKHTTPResponse.Body))
		if err != nil {
			return nil, err
		}
		response.MessageAnnotations = annotations
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
		return prompty.ToolCallPart{ID: part.FunctionCall.ID, Name: part.FunctionCall.Name, Args: args}, nil
	case part.InlineData != nil:
		return prompty.MediaPart{
			MediaType: geminiMediaType(part.InlineData.MIMEType),
			MIMEType:  part.InlineData.MIMEType,
			Data:      bytes.Clone(part.InlineData.Data),
		}, nil
	case part.FileData != nil:
		return prompty.MediaPart{
			MediaType: geminiMediaType(part.FileData.MIMEType),
			MIMEType:  part.FileData.MIMEType,
			URL:       part.FileData.FileURI,
		}, nil
	case part.Thought:
		return prompty.ReasoningPart{Text: part.Text}, nil
	case part.Text != "":
		return prompty.TextPart{Text: part.Text}, nil
	case len(part.ThoughtSignature) != 0:
		return prompty.ReasoningPart{}, nil
	default:
		if reflect.DeepEqual(part, &genai.Part{}) {
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
	default:
		return prompty.OutcomeIncomplete
	}
}

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
	reencoded, err := json.Marshal(&part)
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
	identity := prompty.ProfileIdentity{Provider: desc.Name, Endpoint: desc.Endpoint, Model: desc.DefaultModel}
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
	content := &genai.Content{Role: genai.RoleModel}
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
func geminiUsage(raw *genai.GenerateContentResponse) prompty.Usage {
	var usage prompty.Usage
	if raw.UsageMetadata == nil {
		return usage
	}
	metadata := raw.UsageMetadata
	var body struct {
		Usage map[string]json.RawMessage `json:"usageMetadata"`
	}
	if raw.SDKHTTPResponse != nil {
		if err := json.Unmarshal([]byte(raw.SDKHTTPResponse.Body), &body); err != nil {
			body.Usage = nil
		}
	}
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
		if geminiUsageKnown(field.value, body.Usage[field.key]) {
			*field.destination = int(field.value)
			usage.Known = append(usage.Known, field.counter)
		}
	}
	return usage
}

func geminiUsageKnown(value int32, raw json.RawMessage) bool {
	if value != 0 {
		return true
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var valueShape any
	if err := decoder.Decode(&valueShape); err != nil {
		return false
	}
	number, numeric := valueShape.(json.Number)
	if !numeric {
		return false
	}
	parsed, err := number.Int64()
	return err == nil && parsed == int64(value)
}
