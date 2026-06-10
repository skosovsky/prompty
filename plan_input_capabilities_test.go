package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeCapabilities_UnsetReturnsNil(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)
	assert.Nil(t, base.ComposeCapabilities())
}

func TestComposeCapabilities_EmptyMapIsRuntimeStrict(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)
	withEmpty := PlanInputWithCapabilities(base, map[string]any{})
	require.NotNil(t, withEmpty.ComposeCapabilities())
	assert.Empty(t, withEmpty.ComposeCapabilities())
}

func TestPlanInputWithCapabilities_ClonesMap(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)

	caps := map[string]any{
		"capabilities": map[string]any{"workspace_enabled": true},
	}
	withCaps := PlanInputWithCapabilities(base, caps)
	got := withCaps.ComposeCapabilities()
	require.NotNil(t, got["capabilities"])
	assert.True(t, got["capabilities"].(map[string]any)["workspace_enabled"].(bool))

	caps["capabilities"].(map[string]any)["workspace_enabled"] = false
	assert.True(t, withCaps.ComposeCapabilities()["capabilities"].(map[string]any)["workspace_enabled"].(bool))
}
