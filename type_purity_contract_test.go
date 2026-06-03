package prompty

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTypePurity_ToolInvokerReturnsJSONRawMessage(t *testing.T) {
	t.Parallel()
	method, ok := reflect.TypeFor[ToolInvoker]().MethodByName("InvokeTool")
	require.True(t, ok)
	require.Equal(t, reflect.TypeFor[json.RawMessage](), method.Type.Out(0))
}

func TestTypePurity_RegistryPlanUsesRegistryPlanInput(t *testing.T) {
	t.Parallel()
	method, ok := reflect.TypeFor[Registry]().MethodByName("Plan")
	require.True(t, ok)
	require.Equal(t, reflect.TypeFor[RegistryPlanInput](), method.Type.In(2))
}

func TestTypePurity_RegistryPlanInputAliasExists(t *testing.T) {
	t.Parallel()
	var input = RegistryPlanInput(`{"q":"x"}`)
	assert.JSONEq(t, `{"q":"x"}`, string(input))
}

func TestTypePurity_RegistryPlanInputTypedFactories(t *testing.T) {
	t.Parallel()
	fromStruct, err := RegistryPlanInputFrom(struct {
		Q string `json:"q"`
	}{Q: "x"})
	require.NoError(t, err)
	assert.JSONEq(t, `{"q":"x"}`, string(fromStruct))

	fromJSON, err := RegistryPlanInputFromJSON([]byte(`{"q":"x"}`))
	require.NoError(t, err)
	assert.JSONEq(t, `{"q":"x"}`, string(fromJSON))
}

func TestTypePurity_PublicJSONDocumentBoundaries(t *testing.T) {
	t.Parallel()
	jsonDoc := reflect.TypeFor[JSONDocument]()
	fields := []struct {
		typ   reflect.Type
		field string
	}{
		{reflect.TypeFor[ChatMessage](), "Metadata"},
		{reflect.TypeFor[ToolDefinition](), "Parameters"},
		{reflect.TypeFor[SchemaDefinition](), "Schema"},
		{reflect.TypeFor[PromptMetadata](), "Extras"},
		{reflect.TypeFor[ModelOptions](), "ProviderSettings"},
		{reflect.TypeFor[MessageTemplate](), "Metadata"},
	}
	for _, tc := range fields {
		f, ok := tc.typ.FieldByName(tc.field)
		require.True(t, ok, "field %s on %s", tc.field, tc.typ.Name())
		assert.Equal(t, jsonDoc, f.Type, "%s.%s must be JSONDocument", tc.typ.Name(), tc.field)
	}
}

func TestTypePurity_RenderPlanTypedConstructorsCompile(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent("{{ .Input.q }}")}})
	require.NoError(t, err)
	_, err = NewRenderPlanFromRegistryInput(tpl, RegistryPlanInput(`{"q":"x"}`))
	require.NoError(t, err)
	_ = NewRenderPlanFromStruct(tpl, struct {
		Q string `prompt:"q"`
	}{Q: "x"})
}

func TestTypePurity_RenderPlanStrictMethodsOnly(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeFor[*RenderPlan]()
	_, ok := rt.MethodByName("WithLateVariables")
	assert.False(t, ok, "WithLateVariables(map) removed; use WithLateVariablesJSON")
	_, ok = rt.MethodByName("WithResponseFormat")
	assert.False(t, ok, "WithResponseFormat(any) removed; use WithResponseFormatDefinition/FromStruct")
}
