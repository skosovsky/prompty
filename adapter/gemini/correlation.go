package gemini

import (
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Native IDs correlate repeated names. The legacy protocol can only correlate
// a unique unresolved name; it must never invent an ID from that name.
//
//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func validateGeminiCorrelation(messages []prompty.ChatMessage) error {
	pending := make(map[string]string)
	names := make(map[string]bool)
	seenCalls := make(map[string]bool)
	seenResults := make(map[string]bool)
	bareNames := make(map[string]bool)
	hasCalls := false
	invalid := func() error { return fmt.Errorf("%w: invalid tool correlation", adapter.ErrUnsupportedContentType) }
	for _, message := range messages {
		if message.Role != prompty.RoleTool {
			clear(bareNames)
		}
		for _, content := range message.Content {
			switch part := content.(type) {
			case prompty.ToolCallPart:
				hasCalls = true
				if part.Name == "" {
					return invalid()
				}
				if part.ID == "" {
					if names[part.Name] || pendingGeminiName(pending, part.Name) {
						return invalid()
					}
					names[part.Name] = true
				} else {
					if seenCalls[part.ID] || names[part.Name] {
						return invalid()
					}
					seenCalls[part.ID] = true
					pending[part.ID] = part.Name
				}
			case prompty.ToolResultPart:
				if err := validateGeminiResult(part, hasCalls, pending, names, seenResults, bareNames); err != nil {
					return err
				}
			case *prompty.ToolResultPart:
				if part == nil {
					return invalid()
				}
				if err := validateGeminiResult(*part, hasCalls, pending, names, seenResults, bareNames); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
func validateGeminiResult(part prompty.ToolResultPart, hasCalls bool, pending map[string]string, names map[string]bool,
	seen map[string]bool, bareNames map[string]bool,
) error {
	invalid := fmt.Errorf("%w: invalid tool correlation", adapter.ErrUnsupportedContentType)
	if part.Name == "" {
		return invalid
	}
	switch {
	case part.ToolCallID != "":
		if seen[part.ToolCallID] {
			return invalid
		}
		seen[part.ToolCallID] = true
		if hasCalls {
			if pending[part.ToolCallID] != part.Name {
				return invalid
			}
			delete(pending, part.ToolCallID)
		}
	case hasCalls:
		if !names[part.Name] {
			return invalid
		}
		delete(names, part.Name)
	default:
		if bareNames[part.Name] {
			return invalid
		}
		bareNames[part.Name] = true
	}
	return nil
}

func pendingGeminiName(pending map[string]string, name string) bool {
	for _, value := range pending {
		if value == name {
			return true
		}
	}
	return false
}
