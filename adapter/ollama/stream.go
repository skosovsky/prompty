package ollama

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

var errNativeConsumerStop = errors.New("ollama: native consumer stopped")

// SupportsToolNameCorrelation reports the name-preserving tool_name mapping.
// Execution helpers still require checked profile knowledge and unique names
// across the complete returned batch before any handler is invoked.
func (*Adapter) SupportsToolNameCorrelation() bool { return true }

// PrepareNativeRequest exposes the actual stream flag in translated capture.
// It does not mutate the caller's request or dispatch.
func (*Adapter) PrepareNativeRequest(req *api.ChatRequest) (*api.ChatRequest, error) {
	if req == nil {
		return nil, adapter.ErrNilExecution
	}
	copyRequest := *req
	stream := true
	copyRequest.Stream = &stream
	return &copyRequest, nil
}

// StreamEvents uses native Chat callbacks and stops the transport on consumer
// break. Tool arguments are complete snapshots; SDK indices are not presence-aware
// and are never used to invent provider correlation or JSON deltas.
func (a *Adapter) StreamEvents(ctx context.Context, req *api.ChatRequest) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		if a.client == nil {
			yield(nil, adapter.ErrNoClient)
			return
		}
		request, err := a.PrepareNativeRequest(req)
		if err != nil {
			yield(nil, err)
			return
		}
		state := nativeStreamState{toolRefs: make(map[string]string)}
		stopped := false
		err = a.client.Chat(ctx, request, func(raw api.ChatResponse) error {
			data, marshalErr := json.Marshal(raw)
			if marshalErr != nil {
				return adapter.ErrInvalidResponse
			}
			captureErr := adapter.CaptureProviderChunk(ctx, string(data), prompty.CaptureRecordMetadata{})
			frame, parseErr := state.frame(a, raw)
			if frame != nil && !yield(frame, nil) {
				stopped = true
				return errNativeConsumerStop
			}
			return errors.Join(parseErr, captureErr)
		})
		if stopped {
			return
		}
		if err != nil {
			yield(nil, err)
		} else if !state.finished {
			yield(nil, prompty.ErrStreamProtocol)
		}
	}
}

type nativeStreamState struct {
	toolRefs map[string]string
	serial   int
	finished bool
	hasTools bool
}

func (s *nativeStreamState) frame(a *Adapter, raw api.ChatResponse) (*prompty.ResponseChunk, error) {
	if raw.Message.Role != "" && raw.Message.Role != "assistant" {
		return nil, adapter.ErrInvalidResponse
	}
	if len(raw.Message.Images) != 0 {
		return nil, adapter.ErrUnsupportedContentType
	}
	hasContent := raw.Message.Content != "" || raw.Message.Thinking != "" || len(raw.Message.ToolCalls) != 0
	if s.finished && (hasContent || raw.Done || raw.DoneReason != "") {
		return nil, prompty.ErrStreamProtocol
	}
	parts, err := a.ParseStreamChunk(&raw)
	if err != nil {
		return nil, err
	}
	usage, usageErr := usageFromOllama(&raw)
	frame := &prompty.ResponseChunk{Kind: prompty.StreamDelta, Content: parts, Usage: usage}
	if !hasContent {
		frame.Kind = prompty.StreamUsage
	}
	for _, part := range parts {
		var ref string
		switch value := part.(type) {
		case prompty.TextPart:
			ref = "text:0"
		case prompty.ReasoningPart:
			ref = "thinking:0"
		case prompty.ToolCallPart:
			s.hasTools = true
			if value.ID != "" {
				ref = s.toolRefs[value.ID]
			}
			if ref == "" {
				ref = fmt.Sprintf("tool:%d", s.serial)
				s.serial++
				if value.ID != "" {
					s.toolRefs[value.ID] = ref
				}
			}
		default:
			return nil, adapter.ErrUnsupportedContentType
		}
		frame.Identity.PartIDs = append(frame.Identity.PartIDs, ref)
	}
	if raw.Done {
		s.finished = true
		frame.Kind, frame.FinishReason, frame.Outcome = prompty.StreamFinish, raw.DoneReason, prompty.OutcomeIncomplete
		if raw.DoneReason == "stop" {
			frame.Outcome = prompty.OutcomeCompleted
			if s.hasTools {
				frame.Outcome = prompty.OutcomeToolCalls
			}
		}
	}
	return frame, usageErr
}

// CaptureCoverage declares SDK semantic callbacks, not original NDJSON bytes,
// presence of zero counters, or physical-attempt evidence.
func (*Adapter) CaptureCoverage(mode prompty.StreamMode) prompty.CaptureCoverage {
	var coverage prompty.CaptureCoverage
	coverage.SDKBuild = adapter.DependencyBuildIdentity("github.com/ollama/ollama")
	if mode == prompty.StreamNative {
		coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
		coverage.Stages[prompty.CaptureProviderChunk] = prompty.CaptureSemantic
	}
	return coverage
}

var _ adapter.StreamerAdapter[*api.ChatRequest] = (*Adapter)(nil)
