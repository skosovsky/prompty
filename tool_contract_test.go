package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateExecutionContract_AllRequiredToolManifestsPresent(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search"}
	exec.Tools = []ToolDefinition{{
		Name:         "search",
		Parameters:   MustJSONDocumentFromMap(map[string]any{"type": "object"}),
		Capabilities: []string{"read"},
	}}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{
			Name:         name,
			Parameters:   MustJSONDocumentFromMap(map[string]any{"type": "object"}),
			Capabilities: []string{"read", "cached"},
		}, true
	})

	err := ValidateExecutionContract(exec, contract)

	require.NoError(t, err)
}

func TestValidateExecutionContract_SchemaMatchIgnoresJSONKeyOrder(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search"}
	exec.Tools = []ToolDefinition{{
		Name: "search",
		Parameters: JSONDocument(
			`{"type":"object","properties":{"q":{"type":"string"},"limit":{"type":"number"}}}`,
		),
	}}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{
			Name: name,
			Parameters: JSONDocument(
				`{"properties":{"limit":{"type":"number"},"q":{"type":"string"}},"type":"object"}`,
			),
		}, true
	})

	err := ValidateExecutionContract(exec, contract)

	require.NoError(t, err)
}

func TestValidateExecutionContract_MissingRequiredTool(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search", "lookup"}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{Name: name}, name == "search"
	})

	err := ValidateExecutionManifestContract(exec, ManifestDescriptor{ID: "agent", Digest: "abc"}, contract)

	require.ErrorIs(t, err, ErrMissingRequiredTool)
	assert.Contains(t, err.Error(), "agent@abc")
	assert.Contains(t, err.Error(), "lookup")
}

func TestValidateExecutionContract_EmptyRequiredToolNameFailsClosed(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{""}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{Name: name}, true
	})

	err := ValidateExecutionManifestContract(exec, ManifestDescriptor{ID: "agent", Digest: "abc"}, contract)

	require.ErrorIs(t, err, ErrToolContractMismatch)
	assert.Contains(t, err.Error(), "agent@abc")
	assert.Contains(t, err.Error(), "required tool name is required")
}

func TestValidateExecutionContract_NameMismatchWithoutPromptAllowedTools(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search"}
	contract := ToolManifestContractFunc(func(string) (ToolManifest, bool) {
		return ToolManifest{Name: "lookup"}, true
	})

	err := ValidateExecutionManifestContract(exec, ManifestDescriptor{ID: "agent", Digest: "abc"}, contract)

	require.ErrorIs(t, err, ErrToolContractMismatch)
	assert.Contains(t, err.Error(), "agent@abc")
	assert.Contains(t, err.Error(), "search")
	assert.Contains(t, err.Error(), `got "lookup"`)
}

func TestValidateExecutionContract_SchemaMismatch(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search"}
	exec.Tools = []ToolDefinition{{
		Name:       "search",
		Parameters: MustJSONDocumentFromMap(map[string]any{"type": "object"}),
	}}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{
			Name:       name,
			Parameters: MustJSONDocumentFromMap(map[string]any{"type": "string"}),
		}, true
	})

	err := ValidateExecutionManifestContract(exec, ManifestDescriptor{ID: "agent"}, contract)

	require.ErrorIs(t, err, ErrToolContractMismatch)
	assert.Contains(t, err.Error(), "parameters schema mismatch")
	assert.Contains(t, err.Error(), "search")
}

func TestValidateExecutionContract_MissingCapability(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search"}
	exec.Tools = []ToolDefinition{{
		Name:         "search",
		Capabilities: []string{"read"},
	}}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{Name: name, Capabilities: []string{"write"}}, true
	})

	err := ValidateExecutionContract(exec, contract)

	require.ErrorIs(t, err, ErrToolContractMismatch)
	assert.Contains(t, err.Error(), "missing capability")
}

func TestValidateExecutionContract_RequiredToolOutsidePromptAllowedTools(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search"}
	exec.Tools = []ToolDefinition{{Name: "lookup"}}
	contract := ToolManifestContractFunc(func(name string) (ToolManifest, bool) {
		return ToolManifest{Name: name}, true
	})

	err := ValidateExecutionContract(exec, contract)

	require.ErrorIs(t, err, ErrToolContractMismatch)
	assert.Contains(t, err.Error(), "outside prompt allowed tools")
}

func TestRenderPlan_ExecuteWithContract_ValidatesAfterMaterialization(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hello")},
	}, WithRequiredTools([]string{"search"}))
	require.NoError(t, err)
	plan := NewRenderPlan(tpl)

	_, err = plan.ExecuteWithContract(context.Background(), ToolManifestContractFunc(
		func(string) (ToolManifest, bool) { return ToolManifest{}, false },
	))

	require.ErrorIs(t, err, ErrMissingRequiredTool)
}

func TestRenderPlan_ExecuteWithContract_AllowsNoRequiredToolsWithoutContract(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hello")},
	})
	require.NoError(t, err)
	plan := NewRenderPlan(tpl)

	exec, err := plan.ExecuteWithContract(context.Background(), nil)

	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "hello", mustTextFromParts(t, exec.Messages[0].Content))
}

func TestValidateToolScope_RequiredToolOutsideAllowedScope(t *testing.T) {
	t.Parallel()
	scope := ToolScope{
		Required: []ToolRequirement{{Name: "search"}},
		Allowed:  []ToolManifest{{Name: "lookup"}},
	}

	err := ValidateToolScope(ManifestDescriptor{ID: "agent"}, scope)

	require.ErrorIs(t, err, ErrMissingRequiredTool)
	assert.Contains(t, err.Error(), "outside allowed scope")
	assert.Contains(t, err.Error(), "agent")
}
