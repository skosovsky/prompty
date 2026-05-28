package adapter

import "github.com/skosovsky/prompty"

// PrepareTranslateExecution clones and normalizes execution for provider translation
// without mutating the caller-owned PromptExecution.
func PrepareTranslateExecution(exec *prompty.PromptExecution) *prompty.PromptExecution {
	if exec == nil {
		return nil
	}
	return exec.Clone().Normalize()
}
