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

func TestTypePurity_ChatMessageUsesTypedProvenanceAndCachePolicy(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[ChatMessage]()
	_, hasProvenance := typ.FieldByName("Provenance")
	assert.True(t, hasProvenance)
	provField, _ := typ.FieldByName("Provenance")
	assert.Equal(t, reflect.Pointer, provField.Type.Kind())
	elem := provField.Type.Elem()
	assert.Equal(t, "MessageProvenance", elem.Name())

	_, hasCachePolicy := typ.FieldByName("CachePolicy")
	assert.True(t, hasCachePolicy)

	for _, removed := range []string{"LayerID", "ManifestID", "LayerRef", "CacheControl"} {
		_, ok := typ.FieldByName(removed)
		assert.False(t, ok, "ChatMessage must not expose %s after Task33", removed)
	}
}

func TestTypePurity_BindTemplateVarsNotExported(t *testing.T) {
	t.Parallel()
	_, ok := reflect.TypeFor[struct{}]().MethodByName("BindTemplateVars")
	assert.False(t, ok)
	rt := reflect.TypeFor[func(v any) (map[string]any, []ChatMessage, error)]()
	assert.Equal(t, reflect.Func, rt.Kind())
	assert.Empty(t, rt.Name(), "bindTemplateVars must remain unexported")
}

func TestTypePurity_NoCompiledPromptInRootPackage(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "github.com/skosovsky/prompty", reflect.TypeFor[Registry]().PkgPath())
	for _, rt := range []reflect.Type{
		reflect.TypeFor[Registry](),
		reflect.TypeFor[ManifestDescriptor](),
		reflect.TypeFor[*RenderPlan](),
		reflect.TypeFor[PromptExecution](),
	} {
		assert.NotEqual(t, "CompiledPrompt", rt.Name())
	}
	_, hasCompiledPromptType := reflect.TypeFor[Registry]().MethodByName("CompiledPrompt")
	assert.False(t, hasCompiledPromptType)
}

func TestTypePurity_RenderPlanStrictMethodsOnly(t *testing.T) {
	t.Parallel()
	rt := reflect.TypeFor[*RenderPlan]()
	_, ok := rt.MethodByName("WithLateVariables")
	assert.False(t, ok, "WithLateVariables(map) removed; use WithLateInput")
	_, ok = rt.MethodByName("WithLateVariablesJSON")
	assert.False(t, ok, "WithLateVariablesJSON removed; use WithLateInput")
	_, ok = rt.MethodByName("WithLateInput")
	assert.True(t, ok, "WithLateInput must be available for typed late binding")
	_, ok = rt.MethodByName("AppendToLayer")
	assert.False(t, ok, "AppendToLayer removed; use manifest layers/imports")
	_, ok = rt.MethodByName("ReplaceLayer")
	assert.False(t, ok, "ReplaceLayer removed; use manifest layers/imports")
	_, ok = rt.MethodByName("Compile")
	assert.False(t, ok, "Compile removed from public API; use RenderPlan.Execute + ManifestDescriptor")
	_, ok = rt.MethodByName("WithResponseFormat")
	assert.False(t, ok, "WithResponseFormat(any) removed; use WithResponseFormatDefinition/FromStruct")
}

func TestTypePurity_ManifestCheckpointRegistryEmbedsRegistryAndBytesReader(t *testing.T) {
	t.Parallel()
	typ := reflect.TypeFor[ManifestCheckpointRegistry]()
	require.Equal(t, reflect.Interface, typ.Kind())

	methods := map[string]bool{
		"Plan":                        false,
		"ReadManifestBytes":           false,
		"RecommendManifestDescriptor": false,
		"VerifyManifestDescriptor":    false,
	}
	for m := range typ.Methods() {
		methods[m.Name] = true
	}
	for name, found := range methods {
		assert.True(t, found, "ManifestCheckpointRegistry must declare %s", name)
	}
	assert.Equal(t, 4, typ.NumMethod())
}

func TestTypePurity_HistoryProvenanceContractSmoke(t *testing.T) {
	t.Parallel()
	prov := &MessageProvenance{
		ManifestID: "prior-session",
		LayerID:    "user_turn",
	}
	require.NotNil(t, prov)
	assert.Equal(t, "prior-session", prov.ManifestID)
	assert.Equal(t, "user_turn", prov.LayerID)
}

func mustTemplate(t *testing.T, body string) *ChatPromptTemplate {
	t.Helper()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent(body)}})
	require.NoError(t, err)
	return tpl
}
