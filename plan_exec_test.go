package prompty

import (
	"context"
)

func executeTemplatePlan(tpl *ChatPromptTemplate, input any) (*PromptExecution, error) {
	if tpl == nil {
		return nil, ErrNilRenderPlan
	}
	if _, ok := input.(map[string]any); !ok {
		if _, _, err := getPayloadFields(input); err != nil {
			return nil, err
		}
	}
	return NewRenderPlan(tpl, input).Execute(context.Background())
}
