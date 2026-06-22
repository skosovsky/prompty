package prompty

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNonTextResponse indicates the model returned non-text content (tool calls, media, etc.)
// when a strict textual output was required.
var ErrNonTextResponse = errors.New("prompty: response is not plain text")

// ErrEmptyTextResponse indicates strict text extraction found no text content.
var ErrEmptyTextResponse = errors.New("prompty: response contains no text")

// ErrAmbiguousTextExecution indicates strict render-only extraction found multiple messages.
var ErrAmbiguousTextExecution = errors.New("prompty: execution text is ambiguous")

// NonTextResponseError describes which content types blocked strict text extraction.
type NonTextResponseError struct {
	PartKinds []string
}

func (e *NonTextResponseError) Error() string {
	if e == nil || len(e.PartKinds) == 0 {
		return ErrNonTextResponse.Error()
	}
	return fmt.Sprintf("%v: found %v", ErrNonTextResponse, e.PartKinds)
}

func (e *NonTextResponseError) Unwrap() error { return ErrNonTextResponse }

// contentPartKind returns a stable label for fail-closed diagnostics.
func contentPartKind(p ContentPart) string {
	switch p.(type) {
	case TextPart, *TextPart:
		return "text"
	case MediaPart, *MediaPart:
		return "media"
	case ReasoningPart, *ReasoningPart:
		return "reasoning"
	case ToolCallPart, *ToolCallPart:
		return "tool_call"
	case ToolResultPart, *ToolResultPart:
		return "tool_result"
	default:
		return fmt.Sprintf("%T", p)
	}
}

// StrictTextFromParts concatenates text parts and returns an error if any non-text part is present.
func StrictTextFromParts(parts []ContentPart) (string, error) {
	if len(parts) == 0 {
		return "", ErrEmptyTextResponse
	}
	var b strings.Builder
	var nonText []string
	for _, p := range parts {
		switch x := p.(type) {
		case TextPart:
			b.WriteString(x.Text)
		case *TextPart:
			if x != nil {
				b.WriteString(x.Text)
			}
		default:
			nonText = append(nonText, contentPartKind(p))
		}
	}
	if len(nonText) > 0 {
		return "", &NonTextResponseError{PartKinds: nonText}
	}
	if b.Len() == 0 {
		return "", ErrEmptyTextResponse
	}
	return b.String(), nil
}

// StrictText returns concatenated text or fail-closed error when response contains non-text parts.
func (r *Response) StrictText() (string, error) {
	if r == nil {
		return "", errors.New("prompty: nil response")
	}
	return StrictTextFromParts(r.Content)
}

// Text returns rendered execution text without invoking a model.
func (e *PromptExecution) Text(strict bool) (string, error) {
	if e == nil {
		return "", errors.New("prompty: nil execution")
	}
	if strict {
		if len(e.Messages) != 1 {
			return "", fmt.Errorf(
				"%w: expected exactly one message, got %d",
				ErrAmbiguousTextExecution,
				len(e.Messages),
			)
		}
		return StrictTextFromParts(e.Messages[0].Content)
	}
	var b strings.Builder
	for _, msg := range e.Messages {
		text, err := JoinAdapterTextParts(msg.Content)
		if err != nil {
			return "", err
		}
		if text == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(text)
	}
	if b.Len() == 0 {
		return "", ErrEmptyTextResponse
	}
	return b.String(), nil
}

// RenderText materializes the plan and returns joined rendered text without invoking a model.
func (p *RenderPlan) RenderText(ctx context.Context) (string, error) {
	exec, err := p.Execute(ctx)
	if err != nil {
		return "", err
	}
	return exec.Text(false)
}

// ExecuteAsText renders the plan, invokes the model, and returns plain text (fail-closed).
func (p *RenderPlan) ExecuteAsText(ctx context.Context, invoker Invoker) (string, error) {
	if invoker == nil {
		return "", errors.New("execute as text: invoker is nil")
	}
	exec, err := p.Execute(ctx)
	if err != nil {
		return "", err
	}
	if vErr := validateTextExecution(exec); vErr != nil {
		return "", vErr
	}
	resp, err := invoker.Execute(ctx, exec)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("execute as text: nil response")
	}
	return resp.StrictText()
}

// ExecuteAsText runs a prepared execution through the invoker and returns strict plain text.
func ExecuteAsText(ctx context.Context, invoker Invoker, exec *PromptExecution) (string, error) {
	if invoker == nil {
		return "", errors.New("execute as text: invoker is nil")
	}
	if exec == nil {
		return "", errors.New("execute as text: execution is nil")
	}
	if vErr := validateTextExecution(exec); vErr != nil {
		return "", vErr
	}
	resp, err := invoker.Execute(ctx, exec)
	if err != nil {
		return "", err
	}
	if resp == nil {
		return "", errors.New("execute as text: nil response")
	}
	return resp.StrictText()
}

// JoinAdapterTextParts concatenates TextPart values for provider translation.
// MediaPart and parts handled elsewhere (tool_call, reasoning) are skipped.
func JoinAdapterTextParts(parts []ContentPart) (string, error) {
	if len(parts) == 0 {
		return "", nil
	}
	var b strings.Builder
	for _, p := range parts {
		switch x := p.(type) {
		case TextPart:
			b.WriteString(x.Text)
		case *TextPart:
			if x != nil {
				b.WriteString(x.Text)
			}
		case MediaPart, *MediaPart, ToolCallPart, *ToolCallPart, ReasoningPart, *ReasoningPart:
			continue
		case ToolResultPart, *ToolResultPart:
			return "", fmt.Errorf("%w: unexpected tool_result in adapter text extraction", ErrNonTextResponse)
		default:
			return "", fmt.Errorf("%w: unsupported part %T in adapter text extraction", ErrNonTextResponse, p)
		}
	}
	return b.String(), nil
}

func validateTextExecution(exec *PromptExecution) error {
	if exec.ResponseFormat != nil {
		return fmt.Errorf("%w: response_format requires structured output API", ErrNonTextResponse)
	}
	if len(exec.Tools) > 0 {
		return fmt.Errorf("%w: tools declared on execution", ErrNonTextResponse)
	}
	if len(exec.RequiredTools) > 0 {
		return fmt.Errorf("%w: required_tools declared on execution", ErrNonTextResponse)
	}
	if exec.ForcedTool != "" {
		return fmt.Errorf("%w: forced_tool declared on execution", ErrNonTextResponse)
	}
	return nil
}
