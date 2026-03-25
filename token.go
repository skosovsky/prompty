package prompty

import "unicode/utf8"

type textTokenCounter interface {
	Count(text string) (int, error)
}

// TokenCounter estimates token count for text and canonical chat messages.
// Callers can plug in an exact tokenizer (e.g. tiktoken); default is CharFallbackCounter.
type TokenCounter interface {
	Count(text string) (int, error)
	CountMessage(msg ChatMessage) (int, error)
}

// CharFallbackCounter estimates tokens as runes/CharsPerToken.
// Zero value uses 4 chars per token (English average).
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
		switch x := part.(type) {
		case TextPart:
			n, err := countTextIfNonEmpty(counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case *TextPart:
			if x == nil {
				continue
			}
			n, err := countTextIfNonEmpty(counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case ReasoningPart:
			n, err := countTextIfNonEmpty(counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case *ReasoningPart:
			if x == nil {
				continue
			}
			n, err := countTextIfNonEmpty(counter, x.Text)
			if err != nil {
				return 0, err
			}
			total += n
		case ToolCallPart:
			n, err := countTextIfNonEmpty(counter, toolCallArgsText(x))
			if err != nil {
				return 0, err
			}
			total += n
		case *ToolCallPart:
			if x == nil {
				continue
			}
			n, err := countTextIfNonEmpty(counter, toolCallArgsText(*x))
			if err != nil {
				return 0, err
			}
			total += n
		case ToolResultPart:
			n, err := countContentPartsWithTextCounter(x.Content, counter, mediaPenalty)
			if err != nil {
				return 0, err
			}
			total += n
		case *ToolResultPart:
			if x == nil {
				continue
			}
			n, err := countContentPartsWithTextCounter(x.Content, counter, mediaPenalty)
			if err != nil {
				return 0, err
			}
			total += n
		case MediaPart:
			if mediaPenalty > 0 {
				total += mediaPenalty
			}
		case *MediaPart:
			if x != nil && mediaPenalty > 0 {
				total += mediaPenalty
			}
		}
	}
	return total, nil
}

func countTextIfNonEmpty(counter textTokenCounter, text string) (int, error) {
	if text == "" || counter == nil {
		return 0, nil
	}
	return counter.Count(text)
}
