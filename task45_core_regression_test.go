package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/stretchr/testify/require"
)

func TestPartialDefaultsOwnership(t *testing.T) {
	t.Parallel()
	// Arrange: a reused option and caller-owned nested JSON values.
	defaults := map[string]any{
		"user": map[string]any{"name": "original"},
		"list": []any{map[string]any{"n": json.Number("9007199254740993")}},
	}
	opt, err := WithPartialVariablesJSON(MustJSONDocumentFromMap(defaults))
	require.NoError(t, err)
	message := []MessageTemplate{{Role: RoleUser, Content: TextContent("{{.Input.user.name}}")}}
	first, err := NewChatPromptTemplate(message, opt)
	require.NoError(t, err)
	second, err := NewChatPromptTemplate(message, opt)
	require.NoError(t, err)
	custom, err := NewChatPromptTemplate(message, func(c *ChatPromptTemplate) { c.PartialVariables = defaults })
	require.NoError(t, err)
	sibling := CloneTemplate(first)
	// Act: mutate the caller, source template and cloned template independently.
	defaults["user"].(map[string]any)["name"] = "caller"
	first.PartialVariables["user"].(map[string]any)["name"] = "first"
	sibling.PartialVariables["list"].([]any)[0].(map[string]any)["n"] = json.Number("0")
	// Assert: every ownership boundary isolates nested values and numeric types.
	require.Equal(t, "original", custom.PartialVariables["user"].(map[string]any)["name"])
	require.Equal(t, "original", second.PartialVariables["user"].(map[string]any)["name"])
	require.Equal(t, "original", sibling.PartialVariables["user"].(map[string]any)["name"])
	require.Equal(t, json.Number("9007199254740993"), first.PartialVariables["list"].([]any)[0].(map[string]any)["n"])
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for range 100 {
			first.PartialVariables["user"].(map[string]any)["name"] = "changed"
		}
	}()
	go func() {
		defer wg.Done()
		for range 100 {
			_ = CloneTemplate(second)
		}
	}()
	wg.Wait()
}

func TestTemplateDependenciesRespectDotScopes(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, source string
		want         []string
	}{
		{"range", `{{range .Input.items}}{{.name}}{{end}}`, []string{"items"}},
		{"with", `{{with .Input.user}}{{.name}}{{else}}{{.Input.fallback}}{{end}}`, []string{"user", "fallback"}},
		{"root alias", `{{$root := .}}{{range .Input.items}}{{.name}}{{$root.Input.title}}{{$.Input.footer}}{{end}}`, []string{"items", "title", "footer"}},
		{"input alias", `{{$input := .Input}}{{$input.title}}{{with $input.user}}{{.name}}{{end}}`, []string{"title", "user"}},
		{"range aliases", `{{range $index, $item := .Input.items}}{{$index}}{{$item.name}}{{with $item.profile}}{{.name}}{{end}}{{else}}{{.Input.empty}}{{end}}`, []string{"items", "empty"}},
		{"partial root", `{{define "p"}}{{.Input.title}}{{end}}{{template "p" .}}`, []string{"title"}},
		{"partial local", `{{define "p"}}{{.name}}{{end}}{{range .Input.items}}{{template "p" .}}{{end}}`, []string{"items"}},
		{"partial input", `{{define "p"}}{{.title}}{{end}}{{template "p" .Input}}`, []string{"title"}},
		{"nested partial", `{{define "q"}}{{.Input.title}}{{end}}{{define "p"}}{{template "q" .}}{{end}}{{template "p" .}}`, []string{"title"}},
		{"branch scope", `{{if .Input.enabled}}{{with .Input.user}}{{.name}}{{end}}{{else}}{{.Input.fallback}}{{end}}`, []string{"enabled", "user", "fallback"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			tmpl, err := template.New("root").Parse(test.source)
			require.NoError(t, err)
			// Act.
			got := extractVarsFromTemplate(tmpl)
			// Assert.
			require.ElementsMatch(t, test.want, got)
		})
	}
}

var errEmptyRecipeSchema = errors.New("empty input schema rejected")

type rejectedEmptyRecipeInput struct{}

func (rejectedEmptyRecipeInput) ValidateSchema() error { return errEmptyRecipeSchema }

type rejectedHistoryRecipeInput struct{ History []ChatMessage }

func (rejectedHistoryRecipeInput) ValidateSchema() error { return errEmptyRecipeSchema }

func TestRecipePreservesAdmissionErrorsBeforeRegistry(t *testing.T) {
	t.Parallel()
	// Arrange: empty and history-only values whose schema validation fails.
	for _, input := range []any{rejectedEmptyRecipeInput{}, rejectedHistoryRecipeInput{}} {
		registry := newRecipeTestRegistry(t)
		late, err := NewPromptRecipe[any, recipeLateInput](registry.desc, input)
		require.NoError(t, err)
		noLate, err := NewPromptRecipeNoLate[any](registry.desc, input)
		require.NoError(t, err)
		// Act / Assert: both recipe variants and both entrypoints retain the sentinel.
		_, err = late.Plan(context.Background(), registry)
		require.ErrorIs(t, err, errEmptyRecipeSchema)
		_, err = late.Execute(context.Background(), registry)
		require.ErrorIs(t, err, errEmptyRecipeSchema)
		_, err = noLate.Plan(context.Background(), registry)
		require.ErrorIs(t, err, errEmptyRecipeSchema)
		_, err = noLate.Execute(context.Background(), registry)
		require.ErrorIs(t, err, errEmptyRecipeSchema)
		require.False(t, registry.planCalled)
	}
	// Act: a genuinely empty payload is a valid binding, not error recovery.
	input, err := PlanInputFrom(struct{}{})
	// Assert.
	require.NoError(t, err)
	require.Empty(t, input.boundVars)
	_, err = registryPlanInputFromRecipe(struct{ Value string }{}, PromptRuntimeOptions{})
	require.ErrorIs(t, err, ErrInvalidPayload)
}

type shadowedSchemaUnsupported struct {
	Value chan int `json:"value"`
}
type ambiguousSchemaUnsupported struct {
	Value func() `json:"value"`
}

func TestSchemaInfersOnlyDominantFields(t *testing.T) {
	t.Parallel()
	// Arrange: encoding/json selects the shallow supported field.
	value := struct {
		shadowedSchemaUnsupported

		Value string `json:"value"`
	}{Value: "ok"}
	// Act.
	schema, err := ExtractSchema(value)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, map[string]any{"type": "string"}, schema["properties"].(map[string]any)["value"])
	document, err := json.Marshal(value)
	require.NoError(t, err)
	compiled, err := NewJSONSchemaValidator(nil).CompileSchema(MustJSONDocumentFromMap(schema), SchemaDialect2020)
	require.NoError(t, err)
	require.NoError(t, compiled.ValidateJSON(document))
	// Ambiguous same-depth fields are both excluded before type inference.
	schema, err = ExtractSchema(struct {
		shadowedSchemaUnsupported
		ambiguousSchemaUnsupported //nolint:govet // deliberate JSON field ambiguity tests encoding/json dominance.
	}{})
	require.NoError(t, err)
	require.Empty(t, schema["properties"])
	// A selected unsupported field still fails.
	_, err = ExtractSchema(shadowedSchemaUnsupported{})
	require.Error(t, err)
}

func TestStrictSchemaMappingUsesExactNumericEquality(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source, mapped string
		equal          bool
	}{
		{`{"minimum":1}`, `{"minimum":1.0}`, true},
		{`{"const":{"n":[1,9007199254740993]}}`, `{"const":{"n":[1e0,9007199254740993.0]}}`, true},
		{`{"enum":[0,1.25]}`, `{"enum":[-0.0,125e-2]}`, true},
		{`{"const":9007199254740993}`, `{"const":9007199254740992}`, false},
		{`{"minimum":0.10000000000000001}`, `{"minimum":0.1}`, false},
		{`{"minimum":1}`, `{"minimum":1} {}`, false},
	} {
		t.Run(test.mapped, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			request := compileFixtureRequest(test.source)
			mapper := schemaMapperFunc(func(SchemaMappingRequest) (SchemaMapping, error) {
				return SchemaMapping{Schema: JSONDocument(test.mapped)}, nil
			})
			// Act.
			compiled, err := CompileProviderSchema(request, mapper)
			// Assert.
			if !test.equal {
				require.Error(t, err)
				require.Nil(t, compiled)
				return
			}
			require.NoError(t, err)
			require.NotEqual(t, compiled.Report().SourceDigest, compiled.Report().TranslatedDigest)
		})
	}
}

type rewritingSchemaCompiler struct {
	call       int
	mutateCall int
	from, to   string
}

func (c *rewritingSchemaCompiler) CompileSchema(doc JSONDocument, dialect string) (ValueValidator, error) {
	c.call++
	if c.call == c.mutateCall {
		copy(doc, strings.ReplaceAll(string(doc), c.from, c.to))
	}
	return NewJSONSchemaValidator(nil).CompileSchema(doc, dialect)
}
func TestProviderSchemaCompilerMutationGuard(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, from, to string
		call           int
		accepted       bool
	}{
		{"original contract", "object", "string", 1, false},
		{"translated contract", "object", "string", 2, false},
		{"original numeric normalization", "1.0", "1e0", 1, true},
		{"translated numeric normalization", "1.0", "1e0", 2, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: an honest compiler compiles the bytes after its mutation.
			request := compileFixtureRequest(`{"type":"object","properties":{"n":{"minimum":1.0}}}`)
			request.Validator = &rewritingSchemaCompiler{mutateCall: test.call, from: test.from, to: test.to}
			calls := 0
			mapper := schemaMapperFunc(func(r SchemaMappingRequest) (SchemaMapping, error) {
				calls++
				return SchemaMapping{Schema: r.Source}, nil
			})
			// Act.
			compiled, err := CompileProviderSchema(request, mapper)
			// Assert.
			if test.accepted {
				require.NoError(t, err)
				require.NotNil(t, compiled)
			} else {
				require.ErrorIs(t, err, ErrSchemaUnsupported)
				require.Nil(t, compiled)
			}
			if test.call == 1 && !test.accepted {
				require.Zero(t, calls)
			}
			require.Contains(t, string(request.Source), "object")
			require.Contains(t, string(request.Source), "1.0")
		})
	}
}

func TestReturnedPlanTemplateOwnsDefaults(t *testing.T) {
	t.Parallel()
	// Arrange.
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{Role: RoleUser, Content: TextContent("{{.Input.user.name}}:{{range .Input.list}}{{.n}}{{end}}")},
		},
		MustWithPartialVariablesJSON(JSONDocument(`{"user":{"name":"original"},"list":[{"n":9007199254740993}]}`)),
	)
	require.NoError(t, err)
	plan := NewRenderPlan(tpl)
	sibling := NewRenderPlan(tpl)
	exported := plan.Template()
	// Act: mutate separate public objects; the owned plan remains unchanged.
	tpl.PartialVariables["user"].(map[string]any)["name"] = "source"
	exported.PartialVariables["user"].(map[string]any)["name"] = "exported"
	exported.PartialVariables["list"].([]any)[0].(map[string]any)["n"] = json.Number("1")
	execution, err := plan.Execute(context.Background())
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "original:9007199254740993", mustTextFromParts(t, execution.Messages[0].Content))
	other, err := sibling.Execute(context.Background())
	require.NoError(t, err)
	require.Equal(t, execution.Messages, other.Messages)
}

func TestScopedDependenciesRenderAndPreserveMissingInput(t *testing.T) {
	t.Parallel()
	// Arrange: typed public bindings and an optional message with local dot members.
	type item struct {
		Name string `prompt:"name"`
	}
	type input struct {
		Items []item `prompt:"items"`
		User  item   `prompt:"user"`
		Title string `prompt:"title"`
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: TextContent(
				`{{$root := .}}{{range .Input.items}}{{template "member" .}}{{end}}{{with .Input.user}}{{.name}}{{end}}:{{$root.Input.title}}`,
			),
		},
		{Role: RoleUser, Optional: true, Content: TextContent(`{{range .Input.items}}{{.name}}{{end}}`)},
	}, WithPartialSources(map[string]string{"members.tmpl": `{{define "member"}}{{.name}}{{end}}`}))
	require.NoError(t, err)
	plan, err := NewRenderPlanFromStruct(tpl, input{Items: []item{{Name: "A"}}, User: item{Name: "B"}, Title: "C"})
	require.NoError(t, err)
	// Act.
	execution, err := plan.Execute(context.Background())
	// Assert: local name is never required at the root and optional inclusion uses items.
	require.NoError(t, err)
	require.Len(t, execution.Messages, 2)
	require.Equal(t, "AB:C", mustTextFromParts(t, execution.Messages[0].Content))
	require.Equal(t, "A", mustTextFromParts(t, execution.Messages[1].Content))
	missing, err := NewRenderPlanFromStruct(tpl, struct {
		User item `prompt:"user"`
	}{User: item{Name: "B"}})
	require.NoError(t, err)
	_, err = missing.Execute(context.Background())
	require.ErrorIs(t, err, ErrMissingVariable)
	optional, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{Role: RoleUser, Content: TextContent("base")},
			{Role: RoleUser, Optional: true, Content: TextContent(`{{range .Input.items}}{{.name}}{{end}}`)},
		},
	)
	require.NoError(t, err)
	empty, err := NewRenderPlanFromStruct(optional, input{})
	require.NoError(t, err)
	result, err := empty.Execute(context.Background())
	require.NoError(t, err)
	require.Len(t, result.Messages, 1)
}

func TestRecipeBindingErrorsAndValidEmptyRender(t *testing.T) {
	t.Parallel()
	// Arrange: malformed history and an untagged anonymous prompt struct.
	type Embedded struct {
		Value string `prompt:"value"`
	}
	for _, input := range []any{struct{ History []*ChatMessage }{History: []*ChatMessage{nil}}, struct{ Embedded }{}} {
		// Act.
		_, err := registryPlanInputFromRecipe(input, PromptRuntimeOptions{})
		// Assert: errors are not converted to empty bindings.
		require.Error(t, err)
	}
	// Arrange / Act: a valid empty input reaches rendering.
	bound, err := registryPlanInputFromRecipe(struct{}{}, PromptRuntimeOptions{})
	require.NoError(t, err)
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: RoleUser, Content: TextContent("empty accepted")}})
	require.NoError(t, err)
	plan, err := NewRenderPlanFromPlanInput(tpl, bound)
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "empty accepted", mustTextFromParts(t, execution.Messages[0].Content))
}

type schemaUntaggedBad struct{ Value chan int }
type schemaTaggedWinner struct {
	Value string `json:"Value"`
}
type schemaDeeperBad struct{ schemaUntaggedBad }

func TestSchemaDominanceTagAndDepthControls(t *testing.T) {
	t.Parallel()
	for _, value := range []any{
		struct {
			schemaUntaggedBad
			schemaTaggedWinner
		}{schemaTaggedWinner: schemaTaggedWinner{Value: "ok"}},
		struct {
			schemaDeeperBad
			schemaTaggedWinner
		}{Value: "ok"},
		struct {
			Value   string   `json:"Value"`
			Ignored chan int `json:"-"`
		}{Value: "ok"},
	} {
		// Arrange / Act: tagged, shallower and ignored fields each avoid unsupported inference.
		schema, err := ExtractSchema(value)
		// Assert: validate encoding/json's actual field projection against the inferred schema.
		require.NoError(t, err)
		doc, err := json.Marshal(value)
		require.NoError(t, err)
		compiler, err := NewJSONSchemaValidator(nil).CompileSchema(MustJSONDocumentFromMap(schema), SchemaDialect2020)
		require.NoError(t, err)
		require.NoError(t, compiler.ValidateJSON(doc))
		require.Equal(t, map[string]any{"type": "string"}, schema["properties"].(map[string]any)["Value"])
	}
}

func TestScopedDependenciesPreserveSchemaValidation(t *testing.T) {
	t.Parallel()
	// Arrange: valid local scopes cannot bypass a root input schema constraint.
	type item struct {
		Name string `prompt:"name"`
	}
	type input struct {
		Items []item `prompt:"items"`
		Title string `prompt:"title"`
	}
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{Role: RoleUser, Content: TextContent(`{{range .Input.items}}{{.name}}{{end}}:{{.Input.title}}`)},
		},
		WithInputSchema(
			&SchemaDefinition{
				Schema: JSONDocument(
					`{"type":"object","properties":{"items":{"type":"array"},"title":{"type":"string","minLength":3}},"required":["title"]}`,
				),
			},
		),
	)
	require.NoError(t, err)
	plan, err := NewRenderPlanFromStruct(tpl, input{Items: []item{{Name: "A"}}, Title: "x"})
	require.NoError(t, err)
	// Act.
	_, err = plan.Execute(context.Background())
	// Assert.
	require.ErrorIs(t, err, ErrSchemaMismatch)
}

func TestTemplateAssignmentScopesAndIndexDependencies(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source string
		want   []string
	}{
		{`{{$x := .LateVars}}{{if true}}{{$x = .Input}}{{end}}{{$x.name}}`, []string{"name"}},
		{`{{$x := .Input}}{{if true}}{{$x = .LateVars}}{{end}}{{$x.name}}`, nil},
		{`{{$x := .LateVars}}{{range .Input.items}}{{$x = $.Input}}{{end}}{{$x.name}}`, []string{"items", "name"}},
		{`{{$x := .LateVars}}{{with .Input.user}}{{$x = $.Input}}{{end}}{{$x.name}}`, []string{"user", "name"}},
		{`{{$x := .LateVars}}{{if .Input.enabled}}{{$x = .Input}}{{else}}{{$x = .Tools}}{{end}}{{$x.name}}`, []string{"enabled", "name"}},
		{`{{$x := .LateVars}}{{if $x := .Input}}{{$x.name}}{{end}}{{$x.other}}`, []string{"name"}},
		{`{{with index .Input "profile"}}{{.name}}{{end}}`, []string{"profile"}},
		{`{{with index .Input.items 0}}{{.name}}{{end}}`, []string{"items"}},
	} {
		t.Run(test.source, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			tpl, err := template.New("scope").Parse(test.source)
			require.NoError(t, err)
			// Act / Assert.
			require.ElementsMatch(t, test.want, extractVarsFromTemplate(tpl))
		})
	}
	// Arrange: the escaped alias makes this optional message dependent on name.
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{
				Role:     RoleUser,
				Optional: true,
				Content:  TextContent(`{{$x := .LateVars}}{{if true}}{{$x = .Input}}{{end}}{{$x.name}}`),
			},
		},
		MustWithPartialVariablesJSON(JSONDocument(`{"name":"Ada"}`)),
	)
	require.NoError(t, err)
	// Act.
	execution, err := NewRenderPlan(tpl).Execute(context.Background())
	// Assert.
	require.NoError(t, err)
	require.Len(t, execution.Messages, 1)
	require.Equal(t, "Ada", mustTextFromParts(t, execution.Messages[0].Content))
}

func TestOptionalDependencyUsesLaterPartialDefinition(t *testing.T) {
	t.Parallel()
	// Arrange: the first message references a definition in the second message.
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Optional: true, Content: TextContent(`{{template "p" .Input}}`)},
		{Role: RoleSystem, Content: TextContent(`{{define "p"}}{{.name}}{{end}}static`)},
	}, MustWithPartialVariablesJSON(JSONDocument(`{"name":"Ada"}`)))
	require.NoError(t, err)
	// Act.
	execution, err := NewRenderPlan(tpl).Execute(context.Background())
	// Assert.
	require.NoError(t, err)
	require.Len(t, execution.Messages, 2)
	require.Equal(t, "Ada", mustTextFromParts(t, execution.Messages[0].Content))
}

func TestTemplateActionDeclarationsStayLocal(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		source string
		want   []string
	}{
		{`{{$x := .LateVars}}{{if true}}{{$x := .Input}}{{$x.name}}{{end}}{{$x.other}}`, []string{"name"}},
		{`{{$x := .LateVars}}{{if true}}{{$x = .Input}}{{$x := .Tools}}{{end}}{{$x.name}}`, []string{"name"}},
		{`{{$x := .LateVars}}{{if true}}{{$x := .Tools}}{{$x = .Input}}{{end}}{{$x.other}}`, nil},
		{`{{$x := .LateVars}}{{with .Input.user}}{{$x := $.Input}}{{$x.name}}{{end}}{{$x.other}}`, []string{"user", "name"}},
		{`{{$x := .LateVars}}{{range .Input.items}}{{$x := $.Input}}{{$x.name}}{{end}}{{$x.other}}`, []string{"items", "name"}},
	} {
		t.Run(test.source, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			parsed, err := template.New("scope").Parse(test.source)
			require.NoError(t, err)
			// Act / Assert: local declarations never alter the enclosing alias.
			require.ElementsMatch(t, test.want, extractVarsFromTemplate(parsed))
		})
	}
	// Arrange.
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{
				Role:    RoleUser,
				Content: TextContent(`{{$x := .LateVars}}{{if true}}{{$x := .Input}}{{$x.name}}{{end}}{{$x.other}}`),
			},
		},
		MustWithPartialVariablesJSON(JSONDocument(`{"name":"Ada"}`)),
	)
	require.NoError(t, err)
	// Act.
	plan, err := NewRenderPlan(tpl).WithLateInputJSON(JSONDocument(`{"other":"late"}`))
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	// Assert: other is optional late context, not a required input.
	require.NoError(t, err)
	require.Equal(t, "Adalate", mustTextFromParts(t, execution.Messages[0].Content))
}
