package truncate

import (
	"reflect"

	"github.com/skosovsky/prompty"
)

type truncateConfig struct {
	maxTokens int
	counter   prompty.TokenCounter
}

// Strategy trims message history to fit token budget.
type Strategy interface {
	Truncate(
		messages []prompty.ChatMessage,
		maxTokens int,
		counter prompty.TokenCounter,
	) ([]prompty.ChatMessage, error)
}

// DropOldestStrategy removes the oldest removable turns until history fits budget.
type DropOldestStrategy struct{}

// DropOldest is a convenience function using DropOldestStrategy.
func DropOldest(
	messages []prompty.ChatMessage,
	maxTokens int,
	counter prompty.TokenCounter,
) ([]prompty.ChatMessage, error) {
	return DropOldestStrategy{}.Truncate(messages, maxTokens, counter)
}

// Truncate trims messages to fit maxTokens while preserving protected system/tool invariants.
func (DropOldestStrategy) Truncate(
	messages []prompty.ChatMessage,
	maxTokens int,
	counter prompty.TokenCounter,
) ([]prompty.ChatMessage, error) {
	return truncateMessages(messages, &truncateConfig{
		maxTokens: maxTokens,
		counter:   counter,
	})
}

// TruncateWithStrategy applies strategy; nil strategy defaults to DropOldestStrategy.
//
//nolint:revive // stutter kept for explicit public API naming in ext/truncate.
func TruncateWithStrategy(
	messages []prompty.ChatMessage,
	maxTokens int,
	counter prompty.TokenCounter,
	strategy Strategy,
) ([]prompty.ChatMessage, error) {
	if maxTokens <= 0 || counter == nil {
		return cloneMessages(messages), nil
	}
	strategy = normalizeStrategy(strategy)
	out, err := strategy.Truncate(messages, maxTokens, counter)
	if err != nil {
		return nil, err
	}
	if !ownsResult(strategy) {
		out = cloneMessages(out)
	}
	return out, nil
}

func normalizeStrategy(strategy Strategy) Strategy {
	if isNilStrategy(strategy) {
		return DropOldestStrategy{}
	}
	return strategy
}

func isNilStrategy(strategy Strategy) bool {
	if strategy == nil {
		return true
	}
	value := reflect.ValueOf(strategy)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	default:
		return false
	}
}

func ownsResult(strategy Strategy) bool {
	switch strategy.(type) {
	case DropOldestStrategy, *DropOldestStrategy:
		return true
	default:
		return false
	}
}

func truncateMessages(
	messages []prompty.ChatMessage,
	cfg *truncateConfig,
) ([]prompty.ChatMessage, error) {
	if cfg == nil || cfg.maxTokens <= 0 || cfg.counter == nil {
		return cloneMessages(messages), nil
	}

	total, err := countMessagesTokens(messages, cfg)
	if err != nil {
		return nil, err
	}
	if total <= cfg.maxTokens {
		return cloneMessages(messages), nil
	}

	blocks, err := removableBlocks(messages, cfg)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return cloneMessages(messages), nil
	}

	removed := make([]bool, len(messages))
	removedAny := false
	for _, block := range blocks {
		if total <= cfg.maxTokens {
			break
		}
		for i := block.start; i < block.end; i++ {
			removed[i] = true
		}
		total -= block.tokens
		removedAny = true
	}

	if !removedAny {
		return cloneMessages(messages), nil
	}

	trimmed := make([]prompty.ChatMessage, 0, len(messages))
	for i, message := range messages {
		if removed[i] {
			continue
		}
		trimmed = append(trimmed, message)
	}
	return cloneMessages(trimmed), nil
}

type removableBlock struct {
	start  int
	end    int
	tokens int
}

func removableBlocks(
	messages []prompty.ChatMessage,
	cfg *truncateConfig,
) ([]removableBlock, error) {
	protected := protectedMessages(messages)
	var blocks []removableBlock

	for start := 0; start < len(messages); {
		if protected[start] {
			start++
			continue
		}
		segmentEnd := start
		for segmentEnd < len(messages) && !protected[segmentEnd] {
			segmentEnd++
		}
		segmentBlocks, err := removableBlocksInSegment(messages, start, segmentEnd, cfg)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, segmentBlocks...)
		start = segmentEnd
	}
	return blocks, nil
}

func protectedMessages(messages []prompty.ChatMessage) []bool {
	protected := make([]bool, len(messages))
	prefixEnd := protectedPrefixEnd(messages)
	for i := range prefixEnd {
		protected[i] = true
	}
	for i := prefixEnd; i < len(messages); i++ {
		if messages[i].Role == prompty.RoleSystem || messages[i].Role == prompty.RoleDeveloper {
			protected[i] = true
		}
	}
	return protected
}

func protectedPrefixEnd(messages []prompty.ChatMessage) int {
	end := 0
	for end < len(messages) && (messages[end].Role == prompty.RoleSystem || messages[end].Role == prompty.RoleDeveloper) {
		end++
	}
	return end
}

func removableBlocksInSegment(
	messages []prompty.ChatMessage,
	start, end int,
	cfg *truncateConfig,
) ([]removableBlock, error) {
	if start >= end {
		return nil, nil
	}

	var starts []int
	if messages[start].Role != prompty.RoleUser {
		starts = append(starts, start)
	}
	for i := start; i < end; i++ {
		if messages[i].Role == prompty.RoleUser {
			starts = append(starts, i)
		}
	}

	blocks := make([]removableBlock, 0, len(starts))
	for i, blockStart := range starts {
		blockEnd := end
		if i+1 < len(starts) {
			blockEnd = starts[i+1]
		}
		blockTokens, err := countMessagesTokens(messages[blockStart:blockEnd], cfg)
		if err != nil {
			return nil, err
		}
		blocks = append(blocks, removableBlock{
			start:  blockStart,
			end:    blockEnd,
			tokens: blockTokens,
		})
	}
	return blocks, nil
}

func countMessagesTokens(messages []prompty.ChatMessage, cfg *truncateConfig) (int, error) {
	total := 0
	for i := range messages {
		n, err := countMessageTokens(&messages[i], cfg)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func countMessageTokens(message *prompty.ChatMessage, cfg *truncateConfig) (int, error) {
	if cfg == nil || message == nil {
		return 0, nil
	}
	return cfg.counter.CountMessage(*message)
}

func cloneMessages(messages []prompty.ChatMessage) []prompty.ChatMessage {
	if messages == nil {
		return nil
	}
	return prompty.NewExecution(messages).Messages
}
