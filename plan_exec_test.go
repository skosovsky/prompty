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
		plan, err := newRenderPlanFromMap(tpl, m)
		if err != nil {
			return nil, err
		}
		return plan.Execute(context.Background())
	}
	return NewRenderPlanFromStruct(tpl, input).Execute(context.Background())
}
