//go:build e2e

package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestE2EPlanInputFrom_RegistryPathE2E(t *testing.T) {
	t.Parallel()
	reg := compileStubRegistry{}
	input, err := PlanInputFrom(struct {
		Q string `prompt:"q"`
	}{Q: "ok"})
	require.NoError(t, err)
	plan, err := reg.Plan(context.Background(), "stub", input)
	require.NoError(t, err)
	exec, err := plan.Execute(context.Background())
	require.NoError(t, err)
	assert.Equal(t, "ok", mustTextFromParts(t, exec.Messages[0].Content))
}

type compileStubRegistry struct{}

func (compileStubRegistry) Plan(_ context.Context, _ string, input RegistryPlanInput) (*RenderPlan, error) {
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "stub"}))
	if err != nil {
		return nil, err
	}
	return NewRenderPlanFromPlanInput(tpl, input)
}

func (compileStubRegistry) ReadManifestBytes(context.Context, string) ([]byte, error) {
	return nil, ErrManifestBytesUnavailable
}
