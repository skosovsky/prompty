package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestComposeValues_UnsetReturnsZeroValue(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)
	assert.False(t, base.ComposeValues().IsSet())
}

func TestComposeValues_EmptyMapIsRuntimeStrict(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)
	withEmpty := PlanInputWithComposeContext(base, testComposeValuesContext{values: NewComposeValuesFromPairs()})
	assert.True(t, withEmpty.ComposeValues().IsSet())
	_, ok := withEmpty.ComposeValues().Lookup("capabilities.workspace_enabled")
	assert.False(t, ok)
}

func TestPlanInputWithComposeContext_ReturnsClonedValues(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)

	withCaps := PlanInputWithComposeContext(
		base,
		testComposeValuesContext{values: NewComposeValuesFromPairs(
			ComposeBool("capabilities.workspace_enabled", true),
		)},
	)
	got, ok := withCaps.ComposeValues().Lookup("capabilities.workspace_enabled")
	require.True(t, ok)
	assert.True(t, got.(bool))

	mutated := withCaps.ComposeValues().mapValue()
	mutated["capabilities"].(map[string]any)["workspace_enabled"] = false
	got, ok = withCaps.ComposeValues().Lookup("capabilities.workspace_enabled")
	require.True(t, ok)
	assert.True(t, got.(bool))
}

func TestPlanInputWithComposeContext_UsesTypedPairs(t *testing.T) {
	t.Parallel()
	base, err := PlanInputFrom(struct {
		Query string `prompt:"query"`
	}{Query: "hi"})
	require.NoError(t, err)

	ctx := testComposeContext{workspaceEnabled: true}
	withCtx := PlanInputWithComposeContext(base, ctx)

	got, ok := withCtx.ComposeValues().Lookup("capabilities.workspace_enabled")
	require.True(t, ok)
	assert.True(t, got.(bool))
}

func TestComposeValues_UnknownConditionKey(t *testing.T) {
	t.Parallel()
	values := NewComposeValuesFromPairs(ComposeBool("capabilities.workspace_enabled", true))

	_, ok := values.Lookup("capabilities.unknown")

	assert.False(t, ok)
}

type testComposeContext struct {
	workspaceEnabled bool
}

func (c testComposeContext) ComposeValues() ComposeValues {
	return NewComposeValuesFromPairs(ComposeBool("capabilities.workspace_enabled", c.workspaceEnabled))
}

type testComposeValuesContext struct {
	values ComposeValues
}

func (c testComposeValuesContext) ComposeValues() ComposeValues {
	return c.values
}
