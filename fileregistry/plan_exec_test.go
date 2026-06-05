package fileregistry

import (
	"context"

	"github.com/skosovsky/prompty"
)

func executeTemplatePlan(tpl *prompty.ChatPromptTemplate, input any) (*prompty.PromptExecution, error) {
	if tpl == nil {
		return nil, prompty.ErrNilRenderPlan
	}
	if input == nil {
		return nil, prompty.ErrInvalidPayload
	}
	if m, ok := input.(map[string]any); ok {
		return prompty.NewRenderPlanFromMap(tpl, m).Execute(context.Background())
	}
	plan, err := prompty.NewRenderPlanFromStruct(tpl, input)
	if err != nil {
		return nil, err
	}
	return plan.Execute(context.Background())
}
