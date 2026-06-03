package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type greetArgs struct {
	Name string `json:"name"`
}

func TestDecodeToolArgs_RejectsTrailingJSON(t *testing.T) {
	t.Parallel()
	_, err := DecodeToolArgs[greetArgs](`{"name":"Ada"}{"extra":1}`)
	require.Error(t, err)
}

func TestDecodeToolArgs_DisallowUnknownFields(t *testing.T) {
	t.Parallel()
	_, err := DecodeToolArgs[greetArgs](`{"name":"Ada","extra":1}`)
	require.Error(t, err)
}

func TestTypedToolRegistry_ValidateToolCall(t *testing.T) {
	t.Parallel()
	tool, err := NewTypedTool("greet", func(a greetArgs) (string, error) {
		return "hi " + a.Name, nil
	})
	require.NoError(t, err)

	reg := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(reg, tool))
	require.NoError(t, reg.ValidateToolCall("greet", `{"name":"Bob"}`))
	assert.Len(t, reg.Definitions(), 1)
}

func TestTypedToolRegistry_ValidateDoesNotInvokeHandler(t *testing.T) {
	t.Parallel()
	var invoked bool
	tool, err := NewTypedTool("greet", func(a greetArgs) (string, error) {
		invoked = true
		return "hi " + a.Name, nil
	})
	require.NoError(t, err)

	reg := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(reg, tool))
	require.NoError(t, reg.ValidateToolCall("greet", `{"name":"Bob"}`))
	assert.False(t, invoked, "ValidateToolCall must not run the handler")
}

func TestTypedToolRegistry_InvokeTool(t *testing.T) {
	t.Parallel()
	tool, err := NewTypedTool("greet", func(a greetArgs) (string, error) {
		return "hi " + a.Name, nil
	})
	require.NoError(t, err)

	reg := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(reg, tool))
	out, err := reg.InvokeTool("greet", `{"name":"Ada"}`)
	require.NoError(t, err)
	assert.JSONEq(t, `"hi Ada"`, string(out))
}

func TestEncodeToolResult_RejectsInvalidJSONBytes(t *testing.T) {
	t.Parallel()
	_, err := encodeToolResult([]byte(`{bad`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not valid JSON")
}

func TestTypedToolRegistry_InvokeTool_RejectsUnknownFields(t *testing.T) {
	t.Parallel()
	tool, err := NewTypedTool("greet", func(_ greetArgs) (string, error) {
		return "hi", nil
	})
	require.NoError(t, err)

	reg := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(reg, tool))
	_, err = reg.InvokeTool("greet", `{"name":"Ada","extra":1}`)
	require.Error(t, err)
}
