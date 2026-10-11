package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"slices"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// StreamEvents performs native GenerateContent streaming, without sync fallback.
func (a *Adapter) StreamEvents(ctx context.Context, req *Request) iter.Seq2[*prompty.ResponseChunk, error] {
	destination := prompty.ProfileIdentity{Model: "", ModelFamily: "", Provider: wireGemini, Endpoint: a.endpoint}
	if req != nil {
		destination.Model = req.Model
	}
	return a.StreamEventsForProfile(ctx, req, destination)
}

// StreamEventsForProfile binds signed SDK part representations to the checked destination.
// SDK-GENAI-LOG/STREAM in docs/official-sdk-migration.md: the SDK logs scanner
// failures and malformed converters can panic; neither is hidden by this adapter.
//
//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (a *Adapter) StreamEventsForProfile(
	ctx context.Context, req *Request, destination prompty.ProfileIdentity,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		if a.client == nil {
			yield(nil, adapter.ErrNoClient)
			return
		}
		if a.responseLimit <= 0 {
			yield(nil, prompty.ErrProviderState)
			return
		}
		if req == nil {
			yield(nil, adapter.ErrNilExecution)
			return
		}
		request := copyNativeRequest(req)
		state := generateStreamState{
			serial:           0,
			finished:         false,
			hasTools:         false,
			partialFunctions: nil,
			openIDless:       nil,
			groundingChunks:  0,
			sourcePartSerial: 0,
			destination:      destination,
			toolRefs:         make(map[string]string),
		}
		for raw, sdkErr := range a.client.Models.GenerateContentStream(ctx, request.Model, request.Contents, request.Config) {
			if sdkErr != nil {
				yield(nil, safeObservedResponseError(ctx, sdkErr))
				return
			}
			payload, marshalErr := json.Marshal(raw)
			if marshalErr != nil || len(payload) > a.responseLimit {
				if frame := sdkUsageFailureChunk(raw); frame != nil && !yield(frame, nil) {
					return
				}
				yield(nil, prompty.ErrProviderState)
				return
			}
			frame, err := state.frameSDK(raw, payload)
			if err != nil && frame == nil {
				frame = sdkUsageFailureChunk(raw)
			}
			captureErr := adapter.CaptureProviderChunk(
				ctx,
				string(payload),
				prompty.CaptureRecordMetadata{
					ProviderResponseID: "",
					RequestID:          "",
					HTTPStatus:         nil,
					Usage:              nil,
					Outcome:            "",
					SchemaDigests:      nil,
				},
			)
			if frame != nil && !yield(frame, nil) {
				return
			}
			if err = errors.Join(err, captureErr); err != nil {
				yield(nil, err)
				return
			}
		}
		// SDK-GENAI-STREAM: framing and scanner errors belong to the SDK.
		// A clean iterator EOF still requires a terminal DTO.
		if err := ctx.Err(); err != nil {
			yield(nil, err)
		} else if !state.finished {
			yield(nil, prompty.ErrStreamProtocol)
		}
	}
}

func copyNativeRequest(req *Request) *Request {
	copyRequest := *req
	if req.Config != nil {
		config := *req.Config
		copyRequest.Config = &config
		if config.SystemInstruction != nil {
			instruction := *config.SystemInstruction
			config.SystemInstruction = &instruction
		}
		if config.HTTPOptions != nil {
			options := *config.HTTPOptions
			options.Headers = options.Headers.Clone()
			config.HTTPOptions = &options
		}
	}
	if copyRequest.Config == nil {
		copyRequest.Config = &genai.GenerateContentConfig{}
	}
	copyRequest.Config.HTTPOptions = noRetryHTTPOptions(copyRequest.Config.HTTPOptions)
	return &copyRequest
}

type generateStreamState struct {
	destination      prompty.ProfileIdentity
	toolRefs         map[string]string
	serial           int
	finished         bool
	hasTools         bool
	partialFunctions map[string]*partialFunction
	openIDless       *partialFunction
	groundingChunks  int
	sourcePartSerial int
}

//nolint:cyclop,funlen,gocognit,gocyclo,nestif // Ordered state, metadata and tool publication share one protocol transition.
func (s *generateStreamState) frameSDK(
	raw *genai.GenerateContentResponse,
	payload []byte,
) (*prompty.ResponseChunk, error) {
	if raw == nil {
		return nil, adapter.ErrInvalidResponse
	}
	var err error
	if len(raw.Candidates) > 1 || (len(raw.Candidates) == 1 && raw.Candidates[0] == nil) {
		return nil, adapter.ErrInvalidResponse
	}
	frame := &prompty.ResponseChunk{
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
		Kind:         prompty.StreamUsage,
		Model:        raw.ModelVersion,
	}
	var frameErr error
	groundingChunks := s.groundingChunks
	if len(raw.Candidates) == 1 {
		groundingChunks, err = validateCandidateMetadata(raw.Candidates[0], groundingChunks)
		if err != nil {
			return nil, err
		}
	}
	provenance, err := sourcePartAnnotations(payload, s.sourcePartSerial)
	if err != nil {
		return nil, err
	}
	normalized, err := s.normalizePartialCalls(raw, payload)
	if err != nil {
		return sdkUsageFailureChunk(raw), err
	}
	if len(raw.Candidates) == 1 || raw.PromptFeedback != nil && raw.PromptFeedback.BlockReason != "" {
		if len(raw.Candidates) == 1 {
			candidate := raw.Candidates[0]
			if candidate.Index != 0 {
				return nil, prompty.ErrStreamProtocol
			}
			if roleErr := validateGeminiContentRole(candidate.Content); roleErr != nil {
				return nil, errors.Join(prompty.ErrStreamProtocol, roleErr)
			}
		}
		response, parseErr := parseGeminiResponse(raw, s.destination, s.groundingChunks)
		if parseErr != nil && !errors.Is(parseErr, adapter.ErrEmptyResponse) {
			if response == nil {
				return nil, parseErr
			}
			frameErr = parseErr
			frame.ContinuationUnavailable = true
		}
		if response != nil {
			if s.finished && (len(response.Content) != 0 || response.FinishReason != "") {
				return nil, prompty.ErrStreamProtocol
			}
			frame.Content, frame.ProviderState, frame.Annotations = response.Content, response.ProviderState, response.Annotations
			if len(raw.Candidates) == 1 {
				frame.MessageAnnotations, err = sourceCandidateAnnotations(payload)
				if err != nil {
					return nil, err
				}
			}
			frame.Annotations = append(frame.Annotations, normalized.annotations...)
			frame.Annotations = append(frame.Annotations, provenance...)
			frame.StateUpdates = normalized.updates
			if frameErr != nil {
				trimFailedFrameAssociations(frame)
			}
			if len(frame.Content) != 0 {
				frame.Kind = prompty.StreamDelta
			}
			for index, part := range frame.Content {
				ref := normalized.references[index]
				if tool, ok := part.(prompty.ToolCallPart); ok {
					if normalized.provisional[index] {
						tool.Args = ""
						frame.Content[index] = tool
					}
					s.hasTools = true
					if ref == "" && tool.ID != "" {
						ref = s.toolRefs[tool.ID]
					}
					if ref == "" {
						ref = s.nextRef()
						if tool.ID != "" {
							s.toolRefs[tool.ID] = ref
						}
					}
				} else {
					ref = s.nextRef()
				}
				frame.Identity.PartIDs = append(frame.Identity.PartIDs, ref)
			}
			if response.FinishReason != "" {
				s.finished = true
				frame.Kind, frame.FinishReason, frame.Outcome = prompty.StreamFinish, response.FinishReason, response.Outcome
				if s.hasTools && frame.Outcome == prompty.OutcomeCompleted {
					frame.Outcome = prompty.OutcomeToolCalls
				}
			}
		}
	}
	frame.Usage = geminiUsage(raw)
	s.groundingChunks = groundingChunks
	return frame, frameErr
}

func (s *generateStreamState) nextRef() string {
	ref := fmt.Sprintf("part:%d", s.serial)
	s.serial++
	return ref
}

// CaptureCoverage reports SDK DTO events, not original JSON/SSE or physical attempts.
func (*Adapter) CaptureCoverage(mode prompty.StreamMode) prompty.CaptureCoverage {
	coverage := prompty.CaptureCoverage{
		Stages:           nil,
		PhysicalAttempts: false,
		SDKBuild:         adapter.DependencyBuildIdentity("google.golang.org/genai"),
	}
	if mode == prompty.StreamNative {
		coverage.Stages = make(map[prompty.CaptureStage]prompty.CaptureAccuracy)
		coverage.Stages[prompty.CaptureProviderChunk] = prompty.CaptureSemantic
	}
	return coverage
}

var _ adapter.StreamerAdapter[*Request] = (*Adapter)(nil)
var _ adapter.ProfileStreamerAdapter[*Request] = (*Adapter)(nil)

// Associations for rejected later native parts cannot be attached to a prefix.
func trimFailedFrameAssociations(frame *prompty.ResponseChunk) {
	count := len(frame.Content)
	frame.Annotations = slices.DeleteFunc(frame.Annotations, func(annotation prompty.PartAnnotation) bool {
		return annotation.PartIndex >= count
	})
	frame.StateUpdates = slices.DeleteFunc(frame.StateUpdates, func(state prompty.PartState) bool {
		return state.PartIndex >= count
	})
}
