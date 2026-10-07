package prompty

import (
	"errors"
	"fmt"
	"slices"
)

// Usage contains token statistics for the model response.
type Usage struct {
	PromptTokens              int `json:"prompt_tokens"`
	CompletionTokens          int `json:"completion_tokens"`
	TotalTokens               int `json:"total_tokens"`
	PromptTokensCached        int `json:"cache_read_tokens"`
	PromptTokensCacheCreation int `json:"cache_write_tokens"`
	CompletionTokensReasoning int `json:"reasoning_tokens"`
	// Known lists available counters, including known zero. An adapter may derive
	// total only from fully known inputs under its documented provider semantics.
	Known []UsageCounter `json:"known,omitempty"`
	// Mode distinguishes cumulative snapshots from increments.
	Mode UsageMode `json:"mode,omitempty"`
}

// UsageCounter identifies one independently known token counter.
type UsageCounter string

const (
	UsagePrompt     UsageCounter = "prompt"
	UsageCompletion UsageCounter = "completion"
	UsageTotal      UsageCounter = "total"
	UsageCacheRead  UsageCounter = "cache_read"
	UsageCacheWrite UsageCounter = "cache_write"
	UsageReasoning  UsageCounter = "reasoning"
)

// UsageMode declares how an update combines with earlier counters.
type UsageMode string

const (
	UsageCumulative UsageMode = "cumulative"
	UsageDelta      UsageMode = "delta"
)

// UsageAvailability describes presence of the primary prompt/completion/total
// counters; optional cache/reasoning breakdowns retain independent presence.
type UsageAvailability string

const (
	UsageUnknown UsageAvailability = "unknown"
	UsagePartial UsageAvailability = "partial"
	UsageKnown   UsageAvailability = "known"
)

// Availability never infers known zero from default scalar values.
func (u Usage) Availability() UsageAvailability {
	if len(u.Known) == 0 {
		return UsageUnknown
	}
	primary := 0
	primaryCounters := []UsageCounter{UsagePrompt, UsageCompletion, UsageTotal}
	for _, counter := range primaryCounters {
		if slices.Contains(u.Known, counter) {
			primary++
		}
	}
	if primary == len(primaryCounters) {
		return UsageKnown
	}
	return UsagePartial
}

// Outcome is a provider result, independent of transport completion.
type Outcome string

const (
	OutcomeCompleted  Outcome = "completed"
	OutcomeToolCalls  Outcome = "tool_calls"
	OutcomeRefusal    Outcome = "refusal"
	OutcomeIncomplete Outcome = "incomplete"
	// OutcomePaused ends this provider operation while leaving model work resumable
	// by an explicit caller request. It grants neither output success nor tool execution.
	OutcomePaused Outcome = "paused"
)

// ErrResponseOutcome indicates that the provider did not complete a successful output.
var ErrResponseOutcome = errors.New("prompty: unsuccessful provider outcome")

// OutcomeError preserves provider refusal/incomplete semantics without JSON repair classification.
type OutcomeError struct {
	Outcome      Outcome
	FinishReason string
}

func (e *OutcomeError) Error() string {
	return fmt.Sprintf("%v: %s (%s)", ErrResponseOutcome, e.Outcome, e.FinishReason)
}

func (*OutcomeError) Unwrap() error { return ErrResponseOutcome }

// Response is the canonical full model response for sync calls.
type Response struct {
	Content                 []ContentPart        `json:"content"`
	ProviderState           []PartState          `json:"-"                                  yaml:"-"`
	Annotations             []PartAnnotation     `json:"annotations,omitempty"`
	MessageAnnotations      []MessageAnnotation  `json:"message_annotations,omitempty"`
	ContinuationUnavailable bool                 `json:"continuation_unavailable,omitempty"`
	Usage                   Usage                `json:"usage"`
	FinishReason            string               `json:"finish_reason,omitempty"` // provider stop reason for telemetry
	Outcome                 Outcome              `json:"outcome"`
	Diagnostics             ExecutionDiagnostics `json:"diagnostics"`
}

// NewResponse creates a Response from content parts. Usage remains zero.
func NewResponse(parts []ContentPart) *Response {
	if parts == nil {
		parts = []ContentPart{}
	}
	return &Response{Content: parts, Outcome: OutcomeCompleted}
}

// ResponseChunk is one chunk of the stream.
// In streaming providers Usage and FinishReason are typically populated only in the final chunk.
type ResponseChunk struct {
	// Identity references the handle's local message and logical content parts.
	// Sources declare PartIDs aligned with Content and leave MessageID empty.
	// On terminal events PartIDs instead aligns with Result.Content.
	Identity      StreamIdentity
	Content       []ContentPart
	ProviderState []PartState `json:"-" yaml:"-"`
	// StateUpdates explicitly replace opaque state for the same logical tool call.
	// Indices refer to Content in this frame. Other content kinds are invalid.
	StateUpdates []PartState `json:"-" yaml:"-"`
	// Finalizations collapse already observed logical fragments without adding
	// content. Sources must preserve their exact semantic value and finalize before
	// emitting source-order metadata that contains canonical part indices.
	Finalizations           []StreamPartFinalization `json:"-" yaml:"-"`
	Annotations             []PartAnnotation
	MessageAnnotations      []MessageAnnotation
	ContinuationUnavailable bool
	Usage                   Usage
	Kind                    StreamEventKind
	Outcome                 Outcome
	Diagnostics             ExecutionDiagnostics
	FinishReason            string // provider stop reason (e.g. "stop", "length") for telemetry
	// Result is populated only on the single terminal event.
	Result *Response
}
