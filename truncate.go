package prompty

const defaultMediaTokenPenalty = 256

type mediaPenaltyProvider interface {
	MediaTokenPenalty() int
}

type truncateConfig struct {
	maxTokens    int
	counter      TokenCounter
	mediaPenalty int
}

// Truncate trims the execution history in place until it fits the requested token budget.
func (e *PromptExecution) Truncate(maxTokens int, counter TokenCounter) error {
	if e == nil || maxTokens <= 0 || counter == nil {
		return nil
	}

	trimmed, err := truncateMessages(e.Messages, &truncateConfig{
		maxTokens:    maxTokens,
		counter:      counter,
		mediaPenalty: mediaPenaltyFromCounter(counter),
	})
	if err != nil {
		return err
	}
	e.Messages = trimmed
	return nil
}

func mediaPenaltyFromCounter(counter TokenCounter) int {
	if counter == nil {
		return defaultMediaTokenPenalty
	}
	if provider, ok := counter.(mediaPenaltyProvider); ok {
		return provider.MediaTokenPenalty()
	}
	return defaultMediaTokenPenalty
}

func truncateMessages(messages []ChatMessage, cfg *truncateConfig) ([]ChatMessage, error) {
	if cfg == nil || cfg.maxTokens <= 0 || cfg.counter == nil {
		return messages, nil
	}

	total, err := countMessagesTokens(messages, cfg)
	if err != nil {
		return nil, err
	}
	if total <= cfg.maxTokens {
		return messages, nil
	}

	blocks, err := removableBlocks(messages, cfg)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return messages, nil
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
		return messages, nil
	}

	trimmed := make([]ChatMessage, 0, len(messages))
	for i, message := range messages {
		if removed[i] {
			continue
		}
		trimmed = append(trimmed, message)
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
	return countContentPartsTokens(message.Content, cfg)
}

func countContentPartsTokens(parts []ContentPart, cfg *truncateConfig) (int, error) {
	total := 0
	for _, part := range parts {
		switch x := part.(type) {
		case TextPart:
			n, err := countIfNonEmpty(cfg.counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case *TextPart:
			if x == nil {
				continue
			}
			n, err := countIfNonEmpty(cfg.counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case ReasoningPart:
			n, err := countIfNonEmpty(cfg.counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case *ReasoningPart:
			if x == nil {
				continue
			}
			n, err := countIfNonEmpty(cfg.counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case ToolCallPart:
			n, err := countIfNonEmpty(cfg.counter, toolCallArgsText(x))
			if err != nil {
				return 0, err
			}
			total += n
		case *ToolCallPart:
			if x == nil {
				continue
			}
			n, err := countIfNonEmpty(cfg.counter, toolCallArgsText(*x))
			if err != nil {
				return 0, err
			}
			total += n
		case ToolResultPart:
			n, err := countContentPartsTokens(x.Content, cfg)
			if err != nil {
				return 0, err
			}
			total += n
		case *ToolResultPart:
			if x == nil {
				continue
			}
			n, err := countContentPartsTokens(x.Content, cfg)
			if err != nil {
				return 0, err
			}
			total += n
		case MediaPart:
			if cfg.mediaPenalty > 0 {
				total += cfg.mediaPenalty
			}
		case *MediaPart:
			if x != nil && cfg.mediaPenalty > 0 {
				total += cfg.mediaPenalty
			}
		}
	}
	return total, nil
}

func toolCallArgsText(part ToolCallPart) string {
	if part.Args != "" {
		return part.Args
	}
	return part.ArgsChunk
}

func countIfNonEmpty(counter TokenCounter, text string) (int, error) {
	if text == "" || counter == nil {
		return 0, nil
	}
	return counter.Count(text)
}
