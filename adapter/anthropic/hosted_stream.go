package anthropic

import (
	"encoding/json"
	"fmt"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Server arguments are withheld until close. Empty input is a placeholder.
func (s *messageStreamState) startHostedCall(block *messageStreamBlock) error {
	fields := block.source
	if _, present := fields["input"]; !present {
		fields["input"] = json.RawMessage(`{}`)
	}
	raw, _ := json.Marshal(fields)
	if _, _, _, err := validateHostedSource(raw); err != nil {
		return err
	}
	var input map[string]json.RawMessage
	_ = json.Unmarshal(fields["input"], &input)
	if len(input) > 0 {
		block.initialArguments, block.arguments = true, true
		block.input = string(fields["input"])
	}
	return nil
}

func (b *messageStreamBlock) finishInput() error {
	input := b.input
	if !b.arguments {
		input = "{}"
	}
	var object map[string]json.RawMessage
	if !uniqueSyntheticJSON([]byte(input)) || json.Unmarshal([]byte(input), &object) != nil || object == nil {
		return adapter.ErrMalformedArgs
	}
	b.source["input"] = json.RawMessage(input)
	return nil
}

func (b *messageStreamBlock) appendCitation(raw []byte) error {
	var citations []json.RawMessage
	if source, present := b.source["citations"]; present {
		if json.Unmarshal(source, &citations) != nil {
			return ErrHostedTool
		}
	}
	citations = append(citations, json.RawMessage(raw))
	b.source["citations"], _ = json.Marshal(citations)
	source, _ := json.Marshal(b.source)
	return validateHostedText(source)
}

// A later close cannot reorder records or authorize a result before its call.
//
//nolint:gocognit,nestif // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (s *messageStreamState) publishHostedPrefix(
	chunk *prompty.ResponseChunk,
) (*prompty.ResponseChunk, error) {
	if s.ids == nil {
		s.ids = make(map[string]string)
		s.completed = make(map[string]bool)
	}
	response := prompty.NewResponse(nil)
	for s.published < int64(len(s.blocks)) {
		block := s.blocks[s.published]
		if !block.closed {
			break
		}
		source, err := json.Marshal(block.source)
		if err != nil {
			return hostedPrefixChunk(chunk, response), err
		}
		if hostedType(block.kind) {
			kind, id, name, err := validateHostedSource(source)
			if err != nil {
				return hostedPrefixChunk(chunk, response), err
			}
			if kind == wireServerToolUse {
				if s.ids[id] != "" {
					return hostedPrefixChunk(chunk, response), ErrHostedTool
				}
				s.ids[id] = name
			} else {
				if s.ids[id] != name || s.completed[id] {
					return hostedPrefixChunk(chunk, response), ErrHostedTool
				}
				s.completed[id] = true
			}
			if err := appendHostedItem(response, int(s.published), source, s.destination); err != nil {
				return hostedPrefixChunk(chunk, response), err
			}
		} else if block.kind == wireToolUse {
			if s.ids[block.id] != "" {
				return hostedPrefixChunk(chunk, response), ErrHostedTool
			}
			s.ids[block.id] = wireLocal
		}
		s.published++
	}
	return hostedPrefixChunk(chunk, response), nil
}

func (s *messageStreamState) finishHosted() (*prompty.ResponseChunk, error) {
	//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
	msg := &anthropic.Message{
		StopReason: anthropic.StopReason(s.finish),
	}
	for index := range int64(len(s.blocks)) {
		block := s.blocks[index]
		if !block.closed {
			return nil, prompty.ErrStreamProtocol
		}
		raw, _ := json.Marshal(block.source)
		if block.privateSource != nil {
			raw = block.privateSource
		}
		var value anthropic.ContentBlockUnion
		if json.Unmarshal(raw, &value) != nil {
			return nil, adapter.ErrInvalidResponse
		}
		msg.Content = append(msg.Content, value)
	}
	parsed, err := (&Adapter{defaultModel: "", client: nil, endpoint: "", profiles: nil}).ParseResponseForProfile(
		msg,
		s.destination,
	)
	if err != nil {
		return nil, err
	}
	chunk := &prompty.ResponseChunk{
		Model:                   "",
		Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
		Content:                 nil,
		ProviderState:           nil,
		StateUpdates:            nil,
		Finalizations:           nil,
		Annotations:             nil,
		MessageAnnotations:      nil,
		ContinuationUnavailable: false,
		Usage: prompty.Usage{
			PromptTokens:              0,
			CompletionTokens:          0,
			TotalTokens:               0,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 0,
			CompletionTokensReasoning: 0,
			Known:                     nil,
			Mode:                      "",
		},
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
		Result:       nil,
		Kind:         prompty.StreamFinish,
		FinishReason: s.finish,
		Outcome:      parsed.Outcome,
	}
	for _, annotation := range parsed.MessageAnnotations {
		if annotation.Kind == hostedHistoryAnnotation {
			chunk.MessageAnnotations = append(chunk.MessageAnnotations, annotation)
		}
	}
	return chunk, nil
}

func hostedPrefixChunk(chunk *prompty.ResponseChunk, response *prompty.Response) *prompty.ResponseChunk {
	if len(response.MessageAnnotations) > 0 {
		if chunk == nil {
			chunk = &prompty.ResponseChunk{
				Model:                   "",
				Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
				Content:                 nil,
				ProviderState:           nil,
				StateUpdates:            nil,
				Finalizations:           nil,
				Annotations:             nil,
				MessageAnnotations:      nil,
				ContinuationUnavailable: false,
				Usage: prompty.Usage{
					PromptTokens:              0,
					CompletionTokens:          0,
					TotalTokens:               0,
					PromptTokensCached:        0,
					PromptTokensCacheCreation: 0,
					CompletionTokensReasoning: 0,
					Known:                     nil,
					Mode:                      "",
				},
				Outcome: "",
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
				FinishReason: "",
				Result:       nil,
				Kind:         prompty.StreamDelta,
			}
		}
		chunk.MessageAnnotations = response.MessageAnnotations
	}
	return chunk
}

func (b *messageStreamBlock) finalPart(index int64, chunk *prompty.ResponseChunk) (*prompty.ResponseChunk, error) {
	raw, _ := json.Marshal(b.source)
	var block anthropic.ContentBlockUnion
	if json.Unmarshal(raw, &block) != nil {
		return nil, adapter.ErrInvalidResponse
	}
	response, err := (&Adapter{defaultModel: "", client: nil, endpoint: "", profiles: nil}).ParseResponse(
		//nolint:exhaustruct_v5 // SDK DTO: optional fields and unselected union arms stay zero.
		&anthropic.Message{Content: []anthropic.ContentBlockUnion{block}},
	)
	if err != nil {
		return nil, err
	}
	if len(response.Content) != 1 {
		return nil, adapter.ErrInvalidResponse
	}
	if call, ok := response.Content[0].(prompty.ToolCallPart); ok {
		call.Args = b.input
		if !b.arguments {
			call.Args = "{}"
		}
		response.Content[0] = call
	}
	if chunk == nil {
		chunk = &prompty.ResponseChunk{
			Model:                   "",
			Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
			Content:                 nil,
			ProviderState:           nil,
			StateUpdates:            nil,
			Finalizations:           nil,
			Annotations:             nil,
			MessageAnnotations:      nil,
			ContinuationUnavailable: false,
			Usage: prompty.Usage{
				PromptTokens:              0,
				CompletionTokens:          0,
				TotalTokens:               0,
				PromptTokensCached:        0,
				PromptTokensCacheCreation: 0,
				CompletionTokensReasoning: 0,
				Known:                     nil,
				Mode:                      "",
			},
			Outcome: "",
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
			FinishReason: "",
			Result:       nil,
			Kind:         prompty.StreamDelta,
		}
	}
	chunk.Finalizations = []prompty.StreamPartFinalization{
		{ProviderState: nil, Annotations: nil, PartID: fmt.Sprintf("block:%d", index), Content: response.Content[0]},
	}
	return chunk, nil
}

func (b *messageStreamBlock) validatePrivateStart() error {
	allowed := []string{wireType, wireThinking, wireSignature}
	if b.kind == wireRedactedThinking {
		allowed = []string{wireType, "data"}
	}
	raw, _ := json.Marshal(b.source)
	fields, err := hostedObject(raw, allowed...)
	if err != nil {
		return err
	}
	for _, key := range allowed[1:] {
		if _, present := fields[key]; present {
			if _, ok := sourceString(fields, key, false); !ok {
				return adapter.ErrInvalidResponse
			}
		}
	}
	return nil
}
