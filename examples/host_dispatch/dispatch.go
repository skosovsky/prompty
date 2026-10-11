// Package hostdispatch is an optional caller-owned integration recipe.
package hostdispatch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"mime"
	"time"
	"unicode/utf8"

	"github.com/skosovsky/toolsy"

	"github.com/skosovsky/prompty"
)

// ErrProjection rejects an outcome that this host profile cannot send to a model.
var ErrProjection = errors.New("host dispatch: unsupported model projection")

// Batch is a single-owner in-memory cursor; it is not a durable agent checkpoint.
// Calls/execution are private so a consumer cannot mutate admitted arguments.
// Outcomes and pending errors are host evidence, never automatically model text.
type Batch struct {
	response        *prompty.Response
	validator       prompty.ToolValidator
	destination     prompty.ProfileIdentity
	approvalPending bool
	execution       *prompty.PromptExecution
	calls           []prompty.ToolCallPart
	next            int
	outcomes        []toolsy.ToolOutcome
	pending         error
}

// Admit snapshots a complete provider response after a successful checked call.
// dispatchErr rejects all effects even when response contains partial evidence.
func Admit(ctx context.Context, exec *prompty.PromptExecution, response *prompty.Response,
	dispatchErr error, validator prompty.ToolValidator, destination prompty.ProfileIdentity,
) (*Batch, error) {
	if dispatchErr != nil {
		return nil, dispatchErr
	}
	if exec == nil || response == nil {
		return nil, errors.New("host dispatch: execution and response required")
	}
	work := exec.Clone()
	message, err := response.AssistantMessage()
	if err != nil {
		return nil, fmt.Errorf("host dispatch assistant: %w", err)
	}
	if err = prompty.ValidateContinuation(append(work.Messages, message), destination, time.Now()); err != nil {
		return nil, fmt.Errorf("host dispatch continuation: %w", err)
	}
	calls, err := prompty.AdmitToolCalls(ctx, work, response, validator)
	if err != nil {
		return nil, fmt.Errorf("host dispatch admission: %w", err)
	}
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	if err != nil {
		return nil, fmt.Errorf("host response snapshot: %w", err)
	}
	snapshot, err := prompty.UnmarshalResponse(wire)
	if err != nil {
		return nil, fmt.Errorf("host response snapshot: %w", err)
	}
	return &Batch{
		execution:   work.AddMessage(message),
		calls:       calls,
		response:    snapshot,
		validator:   validator,
		destination: destination,
	}, nil
}

// Pending returns the call awaiting an explicit host decision or reconciliation.
func (b *Batch) Pending() (prompty.ToolCallPart, error) {
	if b.next >= len(b.calls) {
		return prompty.ToolCallPart{}, b.pending
	}
	return b.calls[b.next], b.pending
}

// Outcomes returns host-owned evidence. Use after Run on the same owner goroutine.
func (b *Batch) Outcomes() []toolsy.ToolOutcome {
	return append([]toolsy.ToolOutcome(nil), b.outcomes...)
}

// Execution returns an isolated history snapshot. Only a completed batch may be
// submitted to the provider; an incomplete tool exchange is not a continuation.
func (b *Batch) Execution() (*prompty.PromptExecution, error) {
	if b.next != len(b.calls) || b.pending != nil {
		return nil, errors.New("host dispatch: batch has pending calls")
	}
	return b.execution.Clone(), nil
}

// Run executes the remaining admitted calls. Resolve pre-effect pending approval
// through ResumeApproval; repeated Run never clears a pending error. Unknown/infrastructure outcomes
// require reconciliation outside this example; blind retry is forbidden.
func (b *Batch) Run(ctx context.Context, session *toolsy.Session) error {
	if b.pending != nil {
		return b.pending
	}
	if err := prompty.ValidateContinuation(b.execution.Messages, b.destination, time.Now()); err != nil {
		b.pending = err
		return err
	}
	if _, err := prompty.AdmitToolCalls(ctx, b.execution, b.response, b.validator); err != nil {
		b.pending = err
		return err
	}
	for b.next < len(b.calls) {
		if err := ctx.Err(); err != nil {
			b.pending = err
			return err
		}
		call := b.calls[b.next]
		outcome, err := session.RunCall(ctx, toolsy.ToolCall{
			ToolName: call.Name, Input: toolsy.ToolInput{CallID: call.ID, ArgsJSON: []byte(call.Args)},
		})
		b.outcomes = append(b.outcomes, outcome)
		if err != nil {
			b.pending = err
			var approval *toolsy.PendingApprovalError
			b.approvalPending = errors.As(err, &approval) && len(outcome.Effects) == 0 && len(outcome.Result) == 0 &&
				outcome.ExecutionError == nil
			return err
		}
		if len(outcome.Controls) != 0 {
			b.pending = toolsy.ControlErrorFromSignal(outcome.Controls[0])
			if b.pending == nil {
				b.pending = ErrProjection
			}
			return b.pending
		}
		result, err := Project(call, outcome)
		if err != nil {
			b.pending = err
			return err
		}
		b.execution = b.execution.AddMessage(
			prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{result}},
		)
		b.next++
		if err := ctx.Err(); err != nil {
			b.pending = err
			return err
		}
	}
	return nil
}

// Project is this application's explicit model audience policy. Errors use a
// constant safe message, never an arbitrary error string or diagnostic metadata.
// Effects/progress remain in Batch.Outcomes; controls must be resolved by host.
func Project(call prompty.ToolCallPart, outcome toolsy.ToolOutcome) (prompty.ToolResultPart, error) {
	result := prompty.ToolResultPart{ToolCallID: call.ID, Name: call.Name}
	if len(outcome.Controls) != 0 {
		return result, ErrProjection
	}
	if outcome.Envelope.Audience != toolsy.AudienceModel {
		return result, ErrProjection
	}
	if outcome.ExecutionError != nil {
		result.IsError = true
		result.Content = []prompty.ContentPart{prompty.TextPart{Text: "Tool rejected the request."}}
		return result, nil //nolint:nilerr // Business failure is explicitly encoded in IsError, not a Go transport error.
	}
	if outcome.Status != toolsy.OutcomeSuccess {
		return result, ErrProjection
	}
	mediaType, _, err := mime.ParseMediaType(outcome.ResultMimeType)
	if err != nil {
		return result, ErrProjection
	}
	if (mediaType != "application/json" && mediaType != "text/plain") ||
		(outcome.Envelope.DeliveryClass != toolsy.DeliveryClassStructured && outcome.Envelope.DeliveryClass != toolsy.DeliveryClassText) {
		return result, ErrProjection
	}
	if !utf8.Valid(outcome.Result) || (mediaType == "application/json" && !json.Valid(outcome.Result)) {
		return result, ErrProjection
	}
	result.Content = []prompty.ContentPart{prompty.TextPart{Text: string(outcome.Result)}}
	return result, nil
}

// ResumeApproval resumes only a pre-effect pending approval after an explicit host
// authorization decision. The session gate must enforce that decision itself.
// Other controls or partial effects require reconciliation outside this recipe.
func (b *Batch) ResumeApproval(ctx context.Context, session *toolsy.Session) error {
	if !b.approvalPending {
		return errors.New("host dispatch: no resumable pre-effect approval")
	}
	b.pending = nil
	b.approvalPending = false
	return b.Run(ctx, session)
}
