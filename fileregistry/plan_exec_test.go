package fileregistry

import (
	"context"
	"encoding/json"

	"github.com/skosovsky/prompty"
)

func executeTemplatePlan(tpl *prompty.ChatPromptTemplate, input any) (*prompty.PromptExecution, error) {
	if input == nil {
		return prompty.NewRenderPlan(tpl).Execute(context.Background())
	}
	if m, ok := input.(map[string]any); ok {
		data, err := json.Marshal(m)
		if err != nil {
			return nil, err
		}
		plan, err := prompty.NewRenderPlanFromRegistryInput(tpl, data)
		if err != nil {
			return nil, err
		}
		return plan.Execute(context.Background())
	}
	return prompty.NewRenderPlanFromStruct(tpl, input).Execute(context.Background())
}
