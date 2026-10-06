package gemini

import (
	"encoding/json"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

type partialFunction struct {
	id, name, reference string
	arguments           partialArguments
	closed              bool
	parts               []json.RawMessage
	partBytes           int
	signed              bool
	ordinals            []int
}

type normalizedPartialCalls struct {
	references  map[int]string
	provisional map[int]bool
	annotations []prompty.PartAnnotation
	updates     []prompty.PartState
}

func (s *generateStreamState) normalizePartialCalls(
	raw *genai.GenerateContentResponse,
	payload []byte,
) (normalizedPartialCalls, error) {
	normalized := normalizedPartialCalls{references: make(map[int]string), provisional: make(map[int]bool)}
	if len(raw.Candidates) != 1 || raw.Candidates[0].Content == nil {
		return normalized, nil
	}
	var original struct {
		Candidates []struct {
			Content struct {
				Parts []struct {
					Call json.RawMessage `json:"functionCall"`
				} `json:"parts"`
			} `json:"content"`
		} `json:"candidates"`
	}
	if json.Unmarshal(payload, &original) != nil {
		return normalized, prompty.ErrStreamProtocol
	}
	for index, part := range raw.Candidates[0].Content.Parts {
		s.sourcePartSerial++
		if part == nil || part.FunctionCall == nil {
			continue
		}
		callSource := original.Candidates[0].Content.Parts[index].Call
		call, fragments, incremental, err := s.partialCall(part, callSource)
		if err != nil {
			return normalized, err
		}
		if !incremental {
			continue
		}
		var sourceParts struct {
			Candidates []struct {
				Content struct {
					Parts []json.RawMessage `json:"parts"`
				} `json:"content"`
			} `json:"candidates"`
		}
		if json.Unmarshal(payload, &sourceParts) != nil {
			return normalized, prompty.ErrProviderState
		}
		sourcePart := sourceParts.Candidates[0].Content.Parts[index]
		if len(sourcePart) > defaultResponsePayloadLimit-call.partBytes {
			return normalized, prompty.ErrProviderState
		}
		if err = call.arguments.accept(fragments); err != nil {
			return normalized, err
		}
		continues := part.FunctionCall.WillContinue != nil && *part.FunctionCall.WillContinue
		args := map[string]any(nil)
		if !continues {
			complete, completeErr := call.arguments.complete()
			if completeErr != nil {
				return normalized, completeErr
			}
			args, err = prompty.SchemaDocumentAsMap(prompty.JSONDocument(complete))
			if err != nil {
				return normalized, prompty.ErrStreamProtocol
			}
			call.closed = true
		}
		call.parts = append(call.parts, append(json.RawMessage(nil), sourcePart...))
		call.partBytes += len(sourcePart)
		call.ordinals = append(call.ordinals, s.sourcePartSerial)
		call.signed = call.signed || len(part.ThoughtSignature) > 0
		if call.signed {
			statePayload, stateErr := json.Marshal(partialPartSequence{Parts: call.parts, Ordinals: call.ordinals})
			if stateErr != nil || len(statePayload) > defaultResponsePayloadLimit {
				return normalized, prompty.ErrProviderState
			}
			normalized.updates = append(normalized.updates, prompty.PartState{PartIndex: index,
				Envelope: prompty.ProviderEnvelope{Format: prompty.ProviderStateFormat, Scope: s.destination,
					Codec: geminiPartialPartsCodec, Payload: statePayload, Required: true}})
		}
		part.ThoughtSignature = nil // The canonical projection is not a signed SDK part.
		part.FunctionCall = &genai.FunctionCall{ID: call.id, Name: call.name, Args: args}
		normalized.references[index], normalized.provisional[index] = call.reference, continues
		normalized.annotations = append(normalized.annotations, prompty.PartAnnotation{
			PartIndex: index,
			Kind:      "gemini.partial_function_call",
			Value:     append(prompty.JSONDocument(nil), callSource...),
		})
	}
	return normalized, nil
}

func (s *generateStreamState) partialCall(
	part *genai.Part,
	source []byte,
) (*partialFunction, []json.RawMessage, bool, error) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(source, &fields) != nil || fields == nil {
		return nil, nil, false, prompty.ErrStreamProtocol
	}
	for key := range fields {
		switch key {
		case "id", "name", "args", "partialArgs", "willContinue":
		default:
			return nil, nil, false, prompty.ErrStreamProtocol
		}
	}
	fc := part.FunctionCall
	call := s.partialFunctions[fc.ID]
	if fc.ID == "" {
		call = s.openIDless
		if call != nil && !call.closed && fc.Name != "" {
			return nil, nil, false, prompty.ErrStreamProtocol // Ambiguous new ID-less start.
		}
	}
	_, partialPresent := fields["partialArgs"]
	_, continuationPresent := fields["willContinue"]
	incremental := partialPresent || continuationPresent || call != nil && !call.closed
	if !incremental {
		if fc.Name == "" {
			return nil, nil, false, prompty.ErrStreamProtocol
		}
		return nil, nil, false, nil
	}
	if _, snapshot := fields["args"]; snapshot {
		return nil, nil, false, prompty.ErrStreamProtocol
	}
	if call == nil || call.closed && fc.ID == "" {
		if fc.Name == "" {
			return nil, nil, false, prompty.ErrStreamProtocol
		}
		if fc.ID != "" && s.toolRefs[fc.ID] != "" {
			return nil, nil, false, prompty.ErrStreamProtocol
		}
		call = &partialFunction{id: fc.ID, name: fc.Name, reference: s.nextRef()}
		if fc.ID == "" {
			s.openIDless = call
		} else {
			if s.partialFunctions == nil {
				s.partialFunctions = make(map[string]*partialFunction)
			}
			s.partialFunctions[fc.ID] = call
			if s.toolRefs == nil {
				s.toolRefs = make(map[string]string)
			}
			s.toolRefs[fc.ID] = call.reference
		}
	}
	if call.closed || fc.Name != "" && fc.Name != call.name {
		return nil, nil, false, prompty.ErrStreamProtocol
	}
	var fragments []json.RawMessage
	if partialPresent {
		if json.Unmarshal(fields["partialArgs"], &fragments) != nil || fragments == nil {
			return nil, nil, false, prompty.ErrStreamProtocol
		}
	}
	if continuationPresent {
		var continuing bool
		if string(fields["willContinue"]) != "true" && string(fields["willContinue"]) != "false" ||
			json.Unmarshal(fields["willContinue"], &continuing) != nil {
			return nil, nil, false, prompty.ErrStreamProtocol
		}
	}
	return call, fragments, true, nil
}
