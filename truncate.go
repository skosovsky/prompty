package prompty

import "reflect"

const defaultMediaTokenPenalty = 256

type truncateConfig struct {
	maxTokens int
	counter   TokenCounter
}

// TruncationStrategy trims a message history to fit a token budget.
// Implementations should return messages detached from the input slice and its nested mutable content.
type TruncationStrategy interface {
	Truncate(messages []ChatMessage, maxTokens int, counter TokenCounter) ([]ChatMessage, error)
}

// DropOldestStrategy removes the oldest removable turns until the history fits the budget.
type DropOldestStrategy struct{}

// Truncated returns a new execution trimmed to fit the requested token budget.
func (e *PromptExecution) Truncated(
	maxTokens int,
	counter TokenCounter,
	strategy TruncationStrategy,
) (*PromptExecution, error) {
	if e == nil {
		return nil, nil
	}
	if maxTokens <= 0 || counter == nil {
		return e.Clone(), nil
	}
	strategy = normalizeTruncationStrategy(strategy)
	newMessages, err := strategy.Truncate(e.Messages, maxTokens, counter)
	if err != nil {
		return nil, err
	}
	if !ownsTruncationResult(strategy) {
		newMessages = cloneMessages(newMessages)
	}
	return cloneExecutionWithMessages(e, newMessages), nil
}

// Truncate trims messages to fit maxTokens while preserving protected system/tool invariants.
func (DropOldestStrategy) Truncate(messages []ChatMessage, maxTokens int, counter TokenCounter) ([]ChatMessage, error) {
	return truncateMessages(messages, &truncateConfig{
		maxTokens: maxTokens,
		counter:   counter,
	})
}

func normalizeTruncationStrategy(strategy TruncationStrategy) TruncationStrategy {
	if isNilTruncationStrategy(strategy) {
		return DropOldestStrategy{}
	}
	return strategy
}

func isNilTruncationStrategy(strategy TruncationStrategy) bool {
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

func ownsTruncationResult(strategy TruncationStrategy) bool {
	switch strategy.(type) {
	case DropOldestStrategy, *DropOldestStrategy:
		return true
	default:
		return false
	}
}

func truncateMessages(messages []ChatMessage, cfg *truncateConfig) ([]ChatMessage, error) {
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

	trimmed := make([]ChatMessage, 0, len(messages))
	for i, message := range messages {
		if removed[i] {
			continue
		}
		trimmed = append(trimmed, cloneChatMessage(message))
	}
	return trimmed, nil
}

type removableBlock struct {
	start  int
	end    int
	tokens int
}

func removableBlocks(messages []ChatMessage, cfg *truncateConfig) ([]removableBlock, error) {
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

func protectedMessages(messages []ChatMessage) []bool {
	protected := make([]bool, len(messages))
	prefixEnd := protectedPrefixEnd(messages)
	for i := range prefixEnd {
		protected[i] = true
	}
	for i := prefixEnd; i < len(messages); i++ {
		if messages[i].Role == RoleSystem {
			protected[i] = true
		}
	}
	return protected
}

func protectedPrefixEnd(messages []ChatMessage) int {
	end := 0
	for end < len(messages) && (messages[end].Role == RoleSystem || messages[end].Role == RoleDeveloper) {
		end++
	}
	return end
}

func removableBlocksInSegment(messages []ChatMessage, start, end int, cfg *truncateConfig) ([]removableBlock, error) {
	if start >= end {
		return nil, nil
	}

	var starts []int
	if messages[start].Role != RoleUser {
		starts = append(starts, start)
	}
	for i := start; i < end; i++ {
		if messages[i].Role == RoleUser {
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

func countMessagesTokens(messages []ChatMessage, cfg *truncateConfig) (int, error) {
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

func countMessageTokens(message *ChatMessage, cfg *truncateConfig) (int, error) {
	if cfg == nil || message == nil {
		return 0, nil
	}
	return cfg.counter.CountMessage(*message)
}

func toolCallArgsText(part ToolCallPart) string {
	if part.Args != "" {
		return part.Args
	}
	return part.ArgsChunk
}
