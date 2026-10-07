package anthropic

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// StreamEvents exposes native Messages SSE without retry or buffered fallback.
// Signed reasoning is published only when its block closes with complete state.
func (a *Adapter) StreamEvents(
	ctx context.Context, req *anthropic.MessageNewParams,
) iter.Seq2[*prompty.ResponseChunk, error] {
	destination := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: a.endpoint}
	if req != nil {
		destination.Model = req.Model
	}
	return a.StreamEventsForProfile(ctx, req, destination)
}

// StreamEventsForProfile preserves the full checked destination on opaque state.
func (a *Adapter) StreamEventsForProfile(
	ctx context.Context, req *anthropic.MessageNewParams, destination prompty.ProfileIdentity,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		if a.client == nil {
			yield(nil, adapter.ErrNoClient)
			return
		}
		if req == nil {
			yield(nil, adapter.ErrNilExecution)
			return
		}
		state := messageStreamState{
			destination: destination,
			blocks:      make(map[int64]*messageStreamBlock), usage: make(map[string]json.RawMessage),
		}
		stream := a.client.Messages.NewStreaming(ctx, *req, option.WithMaxRetries(0))
		defer func() { _ = stream.Close() }()
		for stream.Next() {
			event := stream.Current()
			captureErr := adapter.CaptureProviderChunk(ctx, event.RawJSON(), prompty.CaptureRecordMetadata{})
			chunk, err := state.accept(event)
			if chunk != nil && !yield(chunk, nil) {
				return
			}
			if err = errors.Join(err, captureErr); err != nil {
				yield(nil, err)
				return
			}
		}
		if err := stream.Err(); err != nil {
			yield(nil, err)
		} else if !state.stopped {
			yield(nil, prompty.ErrStreamProtocol)
		}
	}
}

type messageStreamBlock struct {
	kind, id, name, thinking, signature, data string
	closed, arguments                         bool
	initialArguments                          bool
	source                                    map[string]json.RawMessage
	input                                     string
	text                                      strings.Builder
	privateSource                             []byte
}

type messageStreamState struct {
	destination prompty.ProfileIdentity
	blocks      map[int64]*messageStreamBlock
	usage       map[string]json.RawMessage
	started     bool
	stopped     bool
	finish      string
	published   int64
	ids         map[string]string
	completed   map[string]bool
}

func (s *messageStreamState) accept(event anthropic.MessageStreamEventUnion) (*prompty.ResponseChunk, error) {
	if err := validateNativeEventSource(event); err != nil {
		return nil, err
	}
	if s.stopped {
		return nil, prompty.ErrStreamProtocol
	}
	if event.Type == "ping" {
		return nil, nil
	}
	if event.Type == "message_start" {
		if s.started || len(event.Message.Content) != 0 || event.Message.ID == "" ||
			event.Message.Role != "assistant" || event.Message.Type != "message" {
			return nil, prompty.ErrStreamProtocol
		}
		s.started = true
		usage, err := s.mergeUsage(event.Message.Usage.RawJSON())
		return &prompty.ResponseChunk{Kind: prompty.StreamStart, Model: event.Message.Model, Usage: usage}, err
	}
	if !s.started {
		return nil, prompty.ErrStreamProtocol
	}
	switch event.Type {
	case "content_block_start":
		return s.startBlock(event)
	case "content_block_delta", "content_block_stop":
		if !event.JSON.Index.Valid() || event.Index < 0 {
			return nil, prompty.ErrStreamProtocol
		}
		block := s.blocks[event.Index]
		if block == nil || block.closed || s.finish != "" {
			return nil, prompty.ErrStreamProtocol
		}
		if event.Type == "content_block_stop" {
			chunk, err := s.closeBlock(event.Index, block)
			if err != nil {
				return nil, err
			}
			block.closed = true
			return s.publishHostedPrefix(chunk)
		}
		return s.delta(event, block)
	case "message_delta":
		for _, block := range s.blocks {
			if !block.closed {
				return nil, prompty.ErrStreamProtocol
			}
		}
		if event.Delta.StopReason != "" {
			if s.finish != "" {
				return nil, prompty.ErrStreamProtocol
			}
			s.finish = string(event.Delta.StopReason)
		}
		usage, err := s.mergeUsage(event.Usage.RawJSON())
		return &prompty.ResponseChunk{Kind: prompty.StreamUsage, Usage: usage}, err
	case "message_stop":
		if s.finish == "" {
			return nil, prompty.ErrStreamProtocol
		}
		chunk, err := s.finishHosted()
		if err != nil {
			return nil, err
		}
		s.stopped = true
		return chunk, nil
	default:
		// Unknown content/event semantics must not become successful lossy output.
		return nil, adapter.ErrUnsupportedContentType
	}
}

func (s *messageStreamState) startBlock(event anthropic.MessageStreamEventUnion) (*prompty.ResponseChunk, error) {
	if !event.JSON.Index.Valid() || event.Index != int64(len(s.blocks)) || s.finish != "" {
		return nil, prompty.ErrStreamProtocol
	}
	for _, prior := range s.blocks {
		if !prior.closed && (prior.kind == "thinking" || prior.kind == "redacted_thinking") {
			return nil, prompty.ErrStreamProtocol
		}
	}
	value := event.ContentBlock
	block := &messageStreamBlock{kind: value.Type, id: value.ID, name: value.Name,
		thinking: value.Thinking, signature: value.Signature, data: value.Data}
	if !uniqueSyntheticJSON([]byte(value.RawJSON())) || json.Unmarshal([]byte(value.RawJSON()), &block.source) != nil {
		return nil, adapter.ErrInvalidResponse
	}
	s.blocks[event.Index] = block
	switch block.kind {
	case "text":
		if err := validateHostedText([]byte(value.RawJSON())); err != nil {
			return nil, err
		}
		block.text.WriteString(value.Text)
		chunk := blockChunk(event.Index, prompty.TextPart{Text: value.Text})
		for _, citation := range value.Citations {
			chunk.Annotations = append(chunk.Annotations, prompty.PartAnnotation{
				PartIndex: 0, Kind: "anthropic.citation", Value: prompty.JSONDocument(citation.RawJSON()),
			})
		}
		return chunk, nil
	case "tool_use":
		if block.id == "" || block.name == "" {
			return nil, prompty.ErrStreamProtocol
		}
		var raw map[string]json.RawMessage
		if json.Unmarshal([]byte(value.RawJSON()), &raw) != nil {
			return nil, adapter.ErrInvalidResponse
		}
		var object map[string]json.RawMessage
		if json.Unmarshal(raw["input"], &object) != nil || object == nil {
			return nil, adapter.ErrMalformedArgs
		}
		part := prompty.ToolCallPart{ID: block.id, Name: block.name}
		if len(object) > 0 {
			part.ArgsChunk = string(raw["input"])
			block.arguments, block.initialArguments = true, true
			block.input = string(raw["input"])
		}
		return blockChunk(event.Index, part), nil
	case "server_tool_use":
		return nil, s.startHostedCall(block)
	case "web_search_tool_result", "web_fetch_tool_result":
		_, _, _, err := validateHostedSource([]byte(value.RawJSON()))
		return nil, err
	case "thinking", "redacted_thinking":
		return nil, block.validatePrivateStart()
	default:
		return nil, adapter.ErrUnsupportedContentType
	}
}

func (s *messageStreamState) delta(
	event anthropic.MessageStreamEventUnion, block *messageStreamBlock,
) (*prompty.ResponseChunk, error) {
	d := event.Delta
	switch {
	case block.kind == "text" && d.Type == "text_delta":
		block.text.WriteString(d.Text)
		return blockChunk(event.Index, prompty.TextPart{Text: d.Text}), nil
	case block.kind == "text" && d.Type == "citations_delta":
		if err := block.appendCitation([]byte(d.Citation.RawJSON())); err != nil {
			return nil, err
		}
		chunk := blockChunk(event.Index, prompty.TextPart{})
		chunk.Annotations = []prompty.PartAnnotation{{
			PartIndex: 0, Kind: "anthropic.citation", Value: prompty.JSONDocument(d.Citation.RawJSON()),
		}}
		return chunk, nil
	case block.kind == "tool_use" && d.Type == "input_json_delta":
		if block.initialArguments {
			return nil, prompty.ErrStreamProtocol
		}
		block.arguments = true
		block.input += d.PartialJSON
		return blockChunk(event.Index, prompty.ToolCallPart{
			ID: block.id, Name: block.name, ArgsChunk: d.PartialJSON,
		}), nil
	case block.kind == "server_tool_use" && d.Type == "input_json_delta":
		if block.initialArguments {
			return nil, prompty.ErrStreamProtocol
		}
		block.arguments = true
		block.input += d.PartialJSON
		return nil, nil
	case block.kind == "thinking" && d.Type == "thinking_delta":
		block.thinking += d.Thinking
		return nil, nil
	case block.kind == "thinking" && d.Type == "signature_delta":
		block.signature += d.Signature
		return nil, nil
	default:
		return nil, adapter.ErrUnsupportedContentType
	}
}

func (s *messageStreamState) closeBlock(index int64, block *messageStreamBlock) (*prompty.ResponseChunk, error) {
	switch block.kind {
	case "server_tool_use":
		return nil, block.finishInput()
	case "tool_use":
		if err := block.finishInput(); err != nil {
			return nil, err
		}
		if !block.arguments {
			return block.finalPart(
				index,
				blockChunk(index, prompty.ToolCallPart{ID: block.id, Name: block.name, ArgsChunk: "{}"}),
			)
		}
		return block.finalPart(index, nil)
	case "text":
		block.source["text"], _ = json.Marshal(block.text.String())
		return block.finalPart(index, nil)
	case "thinking", "redacted_thinking":
		value := anthropic.ContentBlockUnion{
			Type: block.kind, Thinking: block.thinking, Signature: block.signature, Data: block.data,
		}
		state, part, err := thinkingState(value, 0, s.destination)
		if err != nil {
			return nil, err
		}
		chunk := blockChunk(index, part)
		chunk.ProviderState = []prompty.PartState{state}
		block.privateSource = state.Envelope.Payload
		if json.Unmarshal(state.Envelope.Payload, &block.source) != nil {
			return nil, prompty.ErrProviderState
		}
		return chunk, nil
	}
	return nil, nil
}

func blockChunk(index int64, part prompty.ContentPart) *prompty.ResponseChunk {
	return &prompty.ResponseChunk{Kind: prompty.StreamDelta, Content: []prompty.ContentPart{part},
		Identity: prompty.StreamIdentity{PartIDs: []string{fmt.Sprintf("block:%d", index)}}}
}

func (s *messageStreamState) mergeUsage(raw string) (prompty.Usage, error) {
	if raw != "" {
		var update map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &update) != nil {
			return prompty.Usage{}, prompty.ErrUsage
		}
		maps.Copy(s.usage, update)
	}
	data, err := json.Marshal(s.usage)
	if err != nil {
		return prompty.Usage{}, prompty.ErrUsage
	}
	var usage anthropic.Usage
	if err = usage.UnmarshalJSON(data); err != nil {
		return prompty.Usage{}, prompty.ErrUsage
	}
	return usageFromAnthropic(usage)
}

// CaptureCoverage promises SDK semantic events, not HTTP/SSE wire bytes.
func (*Adapter) CaptureCoverage(mode prompty.StreamMode) prompty.CaptureCoverage {
	var coverage prompty.CaptureCoverage
	coverage.SDKBuild = adapter.DependencyBuildIdentity("github.com/anthropics/anthropic-sdk-go")
	if mode == prompty.StreamNative {
		coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
		coverage.Stages[prompty.CaptureProviderChunk] = prompty.CaptureSemantic
	}
	return coverage
}

var _ adapter.StreamerAdapter[*anthropic.MessageNewParams] = (*Adapter)(nil)
