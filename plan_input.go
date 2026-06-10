package prompty

import (
	"fmt"
	"maps"
)

// RegistryPlanInput is an opaque token: payload already bound for template rendering.
type RegistryPlanInput struct {
	boundVars    map[string]any
	chatHistory  []ChatMessage
	capabilities map[string]any
}

// PlanInputFrom binds a typed payload for Registry.Plan (fail-fast before registry access).
func PlanInputFrom[T any](payload T) (RegistryPlanInput, error) {
	vars, history, err := bindTemplateVars(payload)
	if err != nil {
		return RegistryPlanInput{}, fmt.Errorf("plan input: %w", err)
	}
	if vars == nil {
		vars = make(map[string]any)
	}
	return RegistryPlanInput{
		boundVars:    vars,
		chatHistory:  history,
		capabilities: nil,
	}, nil
}

func (r RegistryPlanInput) isEmpty() bool {
	return len(r.boundVars) == 0 && len(r.chatHistory) == 0
}

func cloneCapabilitiesMap(caps map[string]any) map[string]any {
	if caps == nil {
		return nil
	}
	out := make(map[string]any, len(caps))
	for k, v := range caps {
		if nested, ok := v.(map[string]any); ok {
			out[k] = maps.Clone(nested)
			continue
		}
		out[k] = v
	}
	return out
}

// PlanInputWithCapabilities attaches runtime capabilities for manifest condition.match evaluation.
// Pass nil to leave capabilities unset (conservative compose: all conditional imports active).
// A non-nil map — including an empty map — enables runtime condition.match evaluation.
func PlanInputWithCapabilities(in RegistryPlanInput, caps map[string]any) RegistryPlanInput {
	if caps == nil {
		return in
	}
	in.capabilities = cloneCapabilitiesMap(caps)
	return in
}

// ComposeCapabilities returns capabilities for declarative manifest composition.
// nil means unset (conservative); a non-nil map means runtime evaluation (strict when empty).
func (r RegistryPlanInput) ComposeCapabilities() map[string]any {
	if r.capabilities == nil {
		return nil
	}
	return cloneCapabilitiesMap(r.capabilities)
}
