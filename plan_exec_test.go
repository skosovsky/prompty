package prompty

import (
	"context"
)

func executeTemplatePlan(tpl *ChatPromptTemplate, input any) (*PromptExecution, error) {
	if tpl == nil {
		return nil, ErrNilRenderPlan
	}
	if input == nil {
		return nil, ErrInvalidPayload
	}
	if m, ok := input.(map[string]any); ok {
		if len(m) > 0 {
			return nil, ErrInvalidPayload
		}
		return NewRenderPlan(tpl).Execute(context.Background())
	}
	plan, err := NewRenderPlanFromStruct(tpl, input)
	if err != nil {
		return nil, err
	}
	return plan.Execute(context.Background())
}
