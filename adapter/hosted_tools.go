package adapter

import "github.com/skosovsky/prompty"

// RejectUnsupportedHostedTools prevents direct translation ports without a
// hosted mapper from silently dropping canonical execution intent.
func RejectUnsupportedHostedTools(exec *prompty.PromptExecution) error {
	if exec == nil {
		return ErrNilExecution
	}
	if len(exec.HostedTools) != 0 {
		return ErrUnsupportedCapability
	}
	return nil
}
