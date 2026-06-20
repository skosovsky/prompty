package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateExecutionContract_AllRequiredToolsPresent(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search", "lookup"}
	contract := ToolContractFunc(func(name string) bool {
		return name == "search" || name == "lookup"
	})

	err := ValidateExecutionContract(exec, contract)

	require.NoError(t, err)
}

func TestValidateExecutionContract_MissingRequiredTool(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"search", "lookup"}
	contract := ToolContractFunc(func(name string) bool {
		return name == "search"
	})

	err := ValidateExecutionContract(exec, contract)

	require.ErrorIs(t, err, ErrMissingRequiredTool)
	assert.Contains(t, err.Error(), "lookup")
}

func TestRenderPlan_ExecuteWithContract_ValidatesAfterMaterialization(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hello")},
	}, WithRequiredTools([]string{"search"}))
	require.NoError(t, err)
	plan := NewRenderPlan(tpl)

	_, err = plan.ExecuteWithContract(context.Background(), ToolContractFunc(func(string) bool { return false }))

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
