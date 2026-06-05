package remoteregistry

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
	plan, err := reg.Plan(ctx, id, prompty.RegistryPlanInput{})
	if err != nil {
		return nil, err
	}
	return plan.Template(), nil
}
