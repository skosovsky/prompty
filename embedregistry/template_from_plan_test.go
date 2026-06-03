package embedregistry

import (
	"context"

	"github.com/skosovsky/prompty"
)

func templateFromPlan(
	ctx context.Context,
	reg interface {
		Plan(ctx context.Context, id string, input prompty.RegistryPlanInput) (*prompty.RenderPlan, error)
	},
	id string,
) (*prompty.ChatPromptTemplate, error) {
	plan, err := reg.Plan(ctx, id, nil)
	if err != nil {
		return nil, err
	}
	return plan.Template(), nil
}
