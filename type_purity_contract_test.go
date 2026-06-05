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

func TestTypePurity_RegistryPlanInputIsOpaqueStruct(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[RegistryPlanInput]()
	require.Equal(t, reflect.Struct, typ.Kind())
	_, hasBound := typ.FieldByName("boundVars")
	assert.True(t, hasBound)
	assert.False(t, hasBound && typ.Field(0).IsExported())
}

func TestTypePurity_PlanInputFromBindsTypedPayload(t *testing.T) {
	t.Parallel()
	input, err := PlanInputFrom(struct {
		Q string `prompt:"q"`
	}{Q: "x"})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(
		mustTemplate(t, `{{ .Input.q }}`),
		input,
	)
	require.NoError(t, err)
	exec, err := plan.Execute(t.Context())
	require.NoError(t, err)
	assert.Equal(t, "x", mustTextFromParts(t, exec.Messages[0].Content))
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
	input, err := PlanInputFrom(struct {
		Q string `prompt:"q"`
	}{Q: "x"})
	require.NoError(t, err)
	_, err = NewRenderPlanFromPlanInput(tpl, input)
	require.NoError(t, err)
	_, err = NewRenderPlanFromStruct(tpl, struct {
		Q string `prompt:"q"`
	}{Q: "x"})
	require.NoError(t, err)
}

func TestTypePurity_RenderPlanStrictMethodsOnly(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeFor[*RenderPlan]()
	_, ok := rt.MethodByName("WithLateVariables")
	assert.False(t, ok, "WithLateVariables(map) removed; use WithLateVariablesJSON")
	_, ok = rt.MethodByName("WithResponseFormat")
	assert.False(t, ok, "WithResponseFormat(any) removed; use WithResponseFormatDefinition/FromStruct")
}

func mustTemplate(t *testing.T, body string) *ChatPromptTemplate {
	t.Helper()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent(body)}})
	require.NoError(t, err)
	return tpl
}
