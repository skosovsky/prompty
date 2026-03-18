// Package contextx provides context window management middleware for prompty.
package contextx

import (
	"context"
	"iter"

	"github.com/skosovsky/prompty"
)

// trimConfig holds token budget and media penalty for counting.
type trimConfig struct {
	maxTokens    int
	counter      prompty.TokenCounter
	mediaPenalty int // tokens per MediaPart (LLMs charge ~250-1000 per image)
}

// trimToTokenBudget trims exec.Messages in place (on the given slice) until total tokens fit.
// A turn is User + Assistant (+ Tool results if any); never removes system/developer.
// Turn-aware: removes entire turns to avoid orphan tool messages (providers return 400 otherwise).
func trimToTokenBudget(msgs []prompty.ChatMessage, cfg *trimConfig) ([]prompty.ChatMessage, error) {
	if cfg == nil || cfg.maxTokens <= 0 || cfg.counter == nil {
		return msgs, nil
	}
	total, err := countMessagesTokens(msgs, cfg)
	if err != nil {
		return nil, err
	}
	if total <= cfg.maxTokens {
		return msgs, nil
	}
	// Find system/developer block end (messages we must keep)
	keepEnd := 0
	for keepEnd < len(msgs) && (msgs[keepEnd].Role == prompty.RoleSystem || msgs[keepEnd].Role == prompty.RoleDeveloper) {
		keepEnd++
	}
	// Build turn boundaries: each turn starts at User and runs until (exclusive) next User
	turnStarts := findTurnStarts(msgs, keepEnd)
	if len(turnStarts) == 0 {
		return msgs, nil
	}
	// Remove turns from oldest until we fit. When total-turnTokens <= maxTokens, include this
	// turn in removal (set trimEnd = end before break) so budget is actually met.
	trimEnd := keepEnd
	for _, start := range turnStarts {
		if start < keepEnd {
			continue
		}
		end := turnEnd(msgs, start)
		turnTokens, err := countMessagesTokens(msgs[start:end], cfg)
		if err != nil {
			return nil, err
		}
		if total-turnTokens <= cfg.maxTokens {
			trimEnd = end // include this turn so we fit
			break
		}
		total -= turnTokens
		trimEnd = end
	}
	if trimEnd <= keepEnd {
		return msgs, nil
	}
	newMsgs := make([]prompty.ChatMessage, 0, len(msgs)-(trimEnd-keepEnd))
	newMsgs = append(newMsgs, msgs[:keepEnd]...)
	newMsgs = append(newMsgs, msgs[trimEnd:]...)
	return newMsgs, nil
}

// findTurnStarts returns indices where turns start. A turn starts at User, or at the first
// non-system message (Assistant/Tool) if it precedes any User (orphan block).
func findTurnStarts(msgs []prompty.ChatMessage, keepEnd int) []int {
	var starts []int
	if keepEnd >= len(msgs) {
		return starts
	}
	if msgs[keepEnd].Role != prompty.RoleUser {
		starts = append(starts, keepEnd)
	}
	for i := keepEnd; i < len(msgs); i++ {
		if msgs[i].Role == prompty.RoleUser {
			starts = append(starts, i)
		}
	}
	return starts
}

// turnEnd returns the index after the turn that starts at start.
// Turn: User, then Assistant(s) and Tool(s) until next User or end.
func turnEnd(msgs []prompty.ChatMessage, start int) int {
	for i := start + 1; i < len(msgs); i++ {
		if msgs[i].Role == prompty.RoleUser {
			return i
		}
	}
	return len(msgs)
}

func countMessagesTokens(msgs []prompty.ChatMessage, cfg *trimConfig) (int, error) {
	var total int
	for i := range msgs {
		n, err := countMessageTokens(&msgs[i], cfg)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

// countMessageTokens counts tokens for all relevant content parts: TextPart, ReasoningPart,
// ToolCallPart.Args (or ArgsChunk), text from ToolResultPart.Content, and MediaPart (via penalty).
func countMessageTokens(m *prompty.ChatMessage, cfg *trimConfig) (int, error) {
	if cfg == nil {
		return 0, nil
	}
	var total int
	for _, p := range m.Content {
		switch part := p.(type) {
		case prompty.TextPart:
			if part.Text != "" {
				n, err := cfg.counter.Count(part.Text)
				if err != nil {
					return 0, err
				}
				total += n
			}
		case prompty.ReasoningPart:
			if part.Text != "" {
				n, err := cfg.counter.Count(part.Text)
				if err != nil {
					return 0, err
				}
				total += n
			}
		case prompty.ToolCallPart:
			text := part.Args
			if text == "" {
				text = part.ArgsChunk
			}
			if text != "" {
				n, err := cfg.counter.Count(text)
				if err != nil {
					return 0, err
				}
				total += n
			}
		case prompty.ToolResultPart:
			text := prompty.TextFromParts(part.Content)
			if text != "" {
				n, err := cfg.counter.Count(text)
				if err != nil {
					return 0, err
				}
				total += n
			}
		case prompty.MediaPart:
			if cfg.mediaPenalty > 0 {
				total += cfg.mediaPenalty
			}
		}
	}
	return total, nil
}

// TokenBudgetOption configures WithTokenBudget.
type TokenBudgetOption func(*trimConfig)

// WithMediaTokenPenalty sets token penalty per MediaPart (image, audio, etc).
// LLM providers charge ~250-1000 tokens per image. Default is 256.
func WithMediaTokenPenalty(n int) TokenBudgetOption {
	return func(c *trimConfig) { c.mediaPenalty = n }
}

// WithTokenBudget returns a Middleware that trims exec.Messages to fit within maxTokens
// before invoking next. Original exec is never mutated; a deep-copied trimmed exec is passed to next.
// System/developer messages are never removed.
func WithTokenBudget(maxTokens int, counter prompty.TokenCounter, opts ...TokenBudgetOption) prompty.Middleware {
	cfg := &trimConfig{maxTokens: maxTokens, counter: counter, mediaPenalty: 256}
	for _, opt := range opts {
		opt(cfg)
	}
	return func(next prompty.Invoker) prompty.Invoker {
		return &tokenBudgetInvoker{next: next, cfg: cfg}
	}
}

type tokenBudgetInvoker struct {
	next prompty.Invoker
	cfg  *trimConfig
}

func (t *tokenBudgetInvoker) Generate(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	if exec == nil {
		return t.next.Generate(ctx, exec)
	}
	// Deep copy Messages to avoid aliasing; trim on copy, pass copy to next.
	execCopy := *exec
	execCopy.Messages = make([]prompty.ChatMessage, len(exec.Messages))
	copy(execCopy.Messages, exec.Messages)
	trimmed, err := trimToTokenBudget(execCopy.Messages, t.cfg)
	if err != nil {
		return nil, err
	}
	execCopy.Messages = trimmed
	return t.next.Generate(ctx, &execCopy)
}

func (t *tokenBudgetInvoker) GenerateStream(ctx context.Context, exec *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
	if exec == nil {
		return t.next.GenerateStream(ctx, exec)
	}
	execCopy := *exec
	execCopy.Messages = make([]prompty.ChatMessage, len(exec.Messages))
	copy(execCopy.Messages, exec.Messages)
	trimmed, err := trimToTokenBudget(execCopy.Messages, t.cfg)
	if err != nil {
		return func(yield func(*prompty.ResponseChunk, error) bool) { yield(nil, err) }
	}
	execCopy.Messages = trimmed
	return t.next.GenerateStream(ctx, &execCopy)
}
