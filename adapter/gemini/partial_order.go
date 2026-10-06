package gemini

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

const geminiSourcePartAnnotation = "gemini.source_part"

type sourcePartRecord struct {
	Ordinal int             `json:"ordinal"`
	Part    json.RawMessage `json:"part,omitempty"`
}

func sourcePartAnnotations(payload []byte, previous int) ([]prompty.PartAnnotation, error) {
	var root struct {
		Candidates []struct {
			Content struct {
				Parts []json.RawMessage `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(payload, &root) != nil {
		return nil, prompty.ErrProviderState
	}
	if len(root.Candidates) == 0 {
		return nil, nil
	}
	var annotations []prompty.PartAnnotation
	for index, source := range root.Candidates[0].Content.Parts {
		var part genai.Part
		if json.Unmarshal(source, &part) != nil {
			return nil, prompty.ErrProviderState
		}
		record := sourcePartRecord{Ordinal: previous + index + 1}
		if len(part.ThoughtSignature) == 0 {
			record.Part = source
		}
		value, err := json.Marshal(record)
		if err != nil {
			return nil, prompty.ErrProviderState
		}
		annotations = append(
			annotations,
			prompty.PartAnnotation{PartIndex: index, Kind: geminiSourcePartAnnotation, Value: value},
		)
	}
	return annotations, nil
}

func hasPartialState(message prompty.ChatMessage) bool {
	for _, state := range message.ProviderState {
		if state.Envelope.Codec == geminiPartialPartsCodec {
			return true
		}
	}
	return false
}

func decodeOrderedSource(source []byte) (*genai.Part, error) {
	if !json.Valid(source) || uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(source))) != nil {
		return nil, prompty.ErrProviderState
	}
	var part genai.Part
	decoder := json.NewDecoder(bytes.NewReader(source))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if decoder.Decode(&part) != nil {
		return nil, prompty.ErrProviderState
	}
	encoded, err := json.Marshal(&part)
	originalValue := geminiJSONValue(source)
	if len(part.ThoughtSignature) == 0 && part.FunctionCall != nil && len(part.FunctionCall.Args) == 0 {
		// SDK omitempty normalizes unsigned empty args to absence; both project {}.
		if object, ok := originalValue.(map[string]any); ok {
			if call, exists := object["functionCall"].(map[string]any); exists {
				if args, empty := call["args"].(map[string]any); empty && len(args) == 0 {
					delete(call, "args")
				}
			}
		}
	}
	if err != nil || !reflect.DeepEqual(originalValue, geminiJSONValue(encoded)) ||
		validateGeminiSDKStateEncoding(&part) != nil {
		return nil, prompty.ErrProviderState
	}
	return &part, nil
}

func orderedPartialMessage(message prompty.ChatMessage) ([]*genai.Part, error) {
	byPart := make(map[int][]sourcePartRecord)
	ordered := make(map[int]*genai.Part)
	seen := make(map[int]bool)
	for _, annotation := range message.Annotations {
		if annotation.Kind != geminiSourcePartAnnotation {
			continue
		}
		var record sourcePartRecord
		value := annotation.Value
		if annotation.PartIndex < 0 || annotation.PartIndex >= len(message.Content) ||
			len(value) > defaultResponsePayloadLimit ||
			!json.Valid(value) ||
			uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(value))) != nil {
			return nil, prompty.ErrProviderState
		}
		decoder := json.NewDecoder(bytes.NewReader(value))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&record) != nil || record.Ordinal < 1 || seen[record.Ordinal] {
			return nil, prompty.ErrProviderState
		}
		encoded, err := json.Marshal(record)
		if err != nil || !reflect.DeepEqual(geminiJSONValue(value), geminiJSONValue(encoded)) {
			return nil, prompty.ErrProviderState
		}
		seen[record.Ordinal] = true
		byPart[annotation.PartIndex] = append(byPart[annotation.PartIndex], record)
	}
	states := make(map[int]prompty.PartState)
	for _, state := range message.ProviderState {
		if state.PartIndex < 0 || state.PartIndex >= len(message.Content) {
			return nil, prompty.ErrProviderState
		}
		if _, exists := states[state.PartIndex]; exists {
			return nil, prompty.ErrProviderState
		}
		states[state.PartIndex] = state
	}
	var total int
	for index, canonical := range message.Content {
		records := byPart[index]
		if len(records) == 0 {
			return nil, prompty.ErrProviderState
		}
		slices.SortFunc(records, func(a, b sourcePartRecord) int {
			if a.Ordinal < b.Ordinal {
				return -1
			}
			if a.Ordinal > b.Ordinal {
				return 1
			}
			return 0
		})
		var sources []json.RawMessage
		if state, exists := states[index]; exists {
			if !state.Envelope.Required || message.Role != prompty.RoleAssistant {
				return nil, prompty.ErrProviderState
			}
			switch {
			case state.Envelope.Codec == geminiPartialPartsCodec:
				_, projection, complete, err := decodePartialState(state)
				if err != nil || !complete || !reflect.DeepEqual(projection, canonical) {
					return nil, prompty.ErrProviderState
				}
				var sequence partialPartSequence
				if json.Unmarshal(state.Envelope.Payload, &sequence) != nil || len(records) != len(sequence.Parts) {
					return nil, prompty.ErrProviderState
				}
				for offset := range records {
					if records[offset].Ordinal != sequence.Ordinals[offset] {
						return nil, prompty.ErrProviderState
					}
				}
				sources = sequence.Parts
			case state.Envelope.Codec == geminiPartCodec && len(records) == 1:
				sources = []json.RawMessage{state.Envelope.Payload}
			default:
				return nil, prompty.ErrProviderState
			}
		} else {
			for _, record := range records {
				if len(record.Part) == 0 {
					return nil, prompty.ErrProviderState
				}
				sources = append(sources, record.Part)
			}
		}
		parts := make([]*genai.Part, 0, len(sources))
		incremental := false
		for offset, source := range sources {
			part, err := decodeOrderedSource(source)
			if err != nil {
				return nil, err
			}
			// Signed original data never belongs in public source annotations.
			if len(part.ThoughtSignature) > 0 && len(records[offset].Part) != 0 ||
				len(part.ThoughtSignature) == 0 &&
					!reflect.DeepEqual(geminiJSONValue(source), geminiJSONValue(records[offset].Part)) {
				return nil, prompty.ErrProviderState
			}
			if part.FunctionCall != nil &&
				(part.FunctionCall.WillContinue != nil || len(part.FunctionCall.PartialArgs) > 0) {
				incremental = true
			}
			parts = append(parts, part)
			total += len(source)
			if total > defaultResponsePayloadLimit {
				return nil, prompty.ErrProviderState
			}
			ordered[records[offset].Ordinal] = part
		}
		if incremental {
			_, projection, complete, err := projectPartialSequence(sources, false)
			if err != nil || !complete || !reflect.DeepEqual(projection, canonical) {
				return nil, prompty.ErrProviderState
			}
		} else {
			for _, part := range parts {
				projection, err := canonicalGeminiPart(part)
				if err != nil || !reflect.DeepEqual(projection, canonical) {
					return nil, prompty.ErrProviderState
				}
			}
		}
	}
	if len(ordered) != len(seen) {
		return nil, prompty.ErrProviderState
	}
	parts := make([]*genai.Part, 0, len(ordered))
	for ordinal := 1; ordinal <= len(ordered); ordinal++ {
		if ordered[ordinal] == nil {
			return nil, prompty.ErrProviderState
		}
		parts = append(parts, ordered[ordinal])
	}
	return parts, nil
}
