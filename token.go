package prompty

import "unicode/utf8"

type textTokenCounter interface {
	Count(text string) (int, error)
}

const defaultMediaTokenPenalty = 256

// TokenCounter estimates token count for text and canonical chat messages.
// Callers can plug in an exact tokenizer (e.g. tiktoken); default is CharFallbackCounter.
type TokenCounter interface {
	Count(text string) (int, error)
	CountMessage(msg ChatMessage) (int, error)
}

// CharFallbackCounter estimates tokens as runes/CharsPerToken.
// Zero value uses a heuristic of 4 runes per token, plus fixed media penalties.
// It is not a tokenizer or full provider request/context-window guarantee.
type CharFallbackCounter struct {
	CharsPerToken int
}

// Count returns estimated token count: ceil(rune_count / CharsPerToken).
// If CharsPerToken <= 0, uses 4.
func (c *CharFallbackCounter) Count(text string) (int, error) {
	cpt := c.CharsPerToken
	if cpt <= 0 {
		cpt = 4
	}
	n := utf8.RuneCountInString(text)
	return (n + cpt - 1) / cpt, nil
}

// CountMessage estimates tokens for a chat message, including media and nested tool content.
func (c *CharFallbackCounter) CountMessage(msg ChatMessage) (int, error) {
	return countContentPartsWithTextCounter(msg.Content, c, defaultMediaTokenPenalty)
}

func countContentPartsWithTextCounter(parts []ContentPart, counter textTokenCounter, mediaPenalty int) (int, error) {
	total := 0
	for _, part := range parts {
		n, err := countContentPartWithTextCounter(part, counter, mediaPenalty)
		if err != nil {
			return 0, err
		}
		total += n
	}
	return total, nil
}

func countContentPartWithTextCounter(part ContentPart, counter textTokenCounter, mediaPenalty int) (int, error) {
	switch x := part.(type) {
	case TextPart:
		return countTextIfNonEmpty(counter, x.Text)
	case *TextPart:
		if x == nil {
			return 0, nil
		}
		return countTextIfNonEmpty(counter, x.Text)
	case ReasoningPart:
		return countTextIfNonEmpty(counter, x.Text)
	case *ReasoningPart:
		if x == nil {
			return 0, nil
		}
		return countTextIfNonEmpty(counter, x.Text)
	case ToolCallPart:
		return countTextIfNonEmpty(counter, toolCallArgsText(x))
	case *ToolCallPart:
		if x == nil {
			return 0, nil
		}
		return countTextIfNonEmpty(counter, toolCallArgsText(*x))
	case ToolResultPart:
		return countContentPartsWithTextCounter(x.Content, counter, mediaPenalty)
	case *ToolResultPart:
		if x == nil {
			return 0, nil
		}
		return countContentPartsWithTextCounter(x.Content, counter, mediaPenalty)
	case MediaPart:
		if mediaPenalty > 0 {
			return mediaPenalty, nil
		}
	case *MediaPart:
		if x != nil && mediaPenalty > 0 {
			return mediaPenalty, nil
		}
	}

	return 0, nil
}

func countTextIfNonEmpty(counter textTokenCounter, text string) (int, error) {
	if text == "" || counter == nil {
		return 0, nil
	}
	return counter.Count(text)
}

func toolCallArgsText(part ToolCallPart) string {
	return part.Args
}
