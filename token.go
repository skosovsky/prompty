package prompty

import "unicode/utf8"

// TokenCounter estimates token count for a string.
// Callers can plug in an exact tokenizer (e.g. tiktoken); default is CharFallbackCounter.
type TokenCounter interface {
	Count(text string) (int, error)
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
