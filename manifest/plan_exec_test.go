package manifest

import (
	"context"

	"github.com/skosovsky/prompty"
)

func executeTemplatePlan(tpl *prompty.ChatPromptTemplate, input any) (*prompty.PromptExecution, error) {
	return prompty.NewRenderPlan(tpl, input).Execute(context.Background())
}
