package gemini

import (
	"errors"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty/adapter"
)

func geminiThinkingLevel(value any) (genai.ThinkingLevel, error) {
	switch value {
	case "minimal":
		return genai.ThinkingLevelMinimal, nil
	case "low":
		return genai.ThinkingLevelLow, nil
	case "medium":
		return genai.ThinkingLevelMedium, nil
	case "high":
		return genai.ThinkingLevelHigh, nil
	default:
		return "", adapter.ProviderSettingError("thinking_level", errors.New("unsupported thinking level"))
	}
}
