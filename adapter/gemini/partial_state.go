package gemini

import (
	"bytes"
	"encoding/json"
	"reflect"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

const geminiPartialPartsCodec = "gemini.partial_parts.json"

type partialPartSequence struct {
	Parts    []json.RawMessage `json:"parts"`
	Ordinals []int             `json:"ordinals"`
}

func decodePartialState(state prompty.PartState) ([]*genai.Part, prompty.ToolCallPart, bool, error) {
	var sequence partialPartSequence
	var canonical prompty.ToolCallPart
	payload := state.Envelope.Payload
	if len(payload) > defaultResponsePayloadLimit || !json.Valid(payload) ||
		uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil {
		return nil, canonical, false, prompty.ErrProviderState
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&sequence) != nil || len(sequence.Parts) == 0 {
		return nil, canonical, false, prompty.ErrProviderState
	}
	if len(sequence.Ordinals) != len(sequence.Parts) {
		return nil, canonical, false, prompty.ErrProviderState
	}
	for index, ordinal := range sequence.Ordinals {
		if ordinal < 1 || index > 0 && ordinal <= sequence.Ordinals[index-1] {
			return nil, canonical, false, prompty.ErrProviderState
		}
	}
	reencoded, encodeErr := json.Marshal(sequence)
	if encodeErr != nil || !reflect.DeepEqual(geminiJSONValue(payload), geminiJSONValue(reencoded)) {
		return nil, canonical, false, prompty.ErrProviderState
	}
	return projectPartialSequence(sequence.Parts, true)
}

func projectPartialSequence(
	sources []json.RawMessage,
	requireSignature bool,
) ([]*genai.Part, prompty.ToolCallPart, bool, error) {
	var canonical prompty.ToolCallPart
	stream := generateStreamState{toolRefs: make(map[string]string)}
	var parts []*genai.Part
	var expected *partialFunction
	signed := false
	for _, source := range sources {
		var part genai.Part
		decoder := json.NewDecoder(bytes.NewReader(source))
		decoder.DisallowUnknownFields()
		decoder.UseNumber()
		if decoder.Decode(&part) != nil || part.FunctionCall == nil || part.Text != "" || part.Thought ||
			part.InlineData != nil || part.FileData != nil || part.FunctionResponse != nil ||
			part.CodeExecutionResult != nil || part.ExecutableCode != nil || part.MediaResolution != nil || part.VideoMetadata != nil {
			return nil, canonical, false, prompty.ErrProviderState
		}
		encoded, err := json.Marshal(&part)
		if err != nil || !reflect.DeepEqual(geminiJSONValue(source), geminiJSONValue(encoded)) {
			return nil, canonical, false, prompty.ErrProviderState
		}
		var fields struct {
			Call json.RawMessage `json:"functionCall"`
		}
		if json.Unmarshal(source, &fields) != nil {
			return nil, canonical, false, prompty.ErrProviderState
		}
		call, fragments, incremental, err := stream.partialCall(&part, fields.Call)
		if err != nil || !incremental || expected != nil && call != expected {
			return nil, canonical, false, prompty.ErrProviderState
		}
		expected = call
		if call.arguments.accept(fragments) != nil {
			return nil, canonical, false, prompty.ErrProviderState
		}
		canonical.ID, canonical.Name = call.id, call.name
		if part.FunctionCall.WillContinue == nil || !*part.FunctionCall.WillContinue {
			args, completeErr := call.arguments.complete()
			if completeErr != nil {
				return nil, canonical, false, prompty.ErrProviderState
			}
			canonical.Args, call.closed = args, true
		}
		signed = signed || len(part.ThoughtSignature) > 0
		parts = append(parts, &part)
	}
	if len(parts) == 0 || requireSignature && !signed {
		return nil, canonical, false, prompty.ErrProviderState
	}
	return parts, canonical, expected.closed, nil
}
