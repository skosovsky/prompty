package prompty

import "fmt"

// RegistryPlanInput is an opaque token: payload already bound for template rendering.
type RegistryPlanInput struct {
	boundVars   map[string]any
	chatHistory []ChatMessage
}

// PlanInputFrom binds a typed payload for Registry.Plan (fail-fast before registry access).
func PlanInputFrom[T any](payload T) (RegistryPlanInput, error) {
	vars, history, err := BindTemplateVars(payload)
	if err != nil {
		return RegistryPlanInput{}, fmt.Errorf("plan input: %w", err)
	}
	if vars == nil {
		vars = make(map[string]any)
	}
	return RegistryPlanInput{
		boundVars:   vars,
		chatHistory: history,
	}, nil
}

func (r RegistryPlanInput) isEmpty() bool {
	return len(r.boundVars) == 0 && len(r.chatHistory) == 0
}
