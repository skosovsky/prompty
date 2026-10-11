package prompty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTypedToolRejectsScalarArguments(t *testing.T) {
	t.Parallel()
	// Arrange: a scalar decoder is useful outside canonical object-root tools.
	value, err := DecodeToolArgs[int]("42")
	require.NoError(t, err)
	require.Equal(t, 42, value)
	called := false
	// Act.
	tool, err := NewTypedTool("scalar", func(context.Context, int) (string, error) {
		called = true
		return "unexpected", nil
	})
	// Assert: fail during construction, never after dispatch or side effects.
	require.ErrorContains(t, err, "object-root")
	require.Nil(t, tool)
	require.False(t, called)
}

func TestTypedRegistryOwnsRawResults(t *testing.T) {
	t.Parallel()
	for _, rawMessage := range []bool{false, true} {
		t.Run(map[bool]string{false: "bytes", true: "raw-message"}[rawMessage], func(t *testing.T) {
			t.Parallel()
			// Arrange: the handler retains its result buffer.
			buffer := []byte(`{"value":1}`)
			tool, err := NewTypedTool("lookup", func(context.Context, struct{}) (any, error) {
				if rawMessage {
					return json.RawMessage(buffer), nil
				}
				return buffer, nil
			})
			require.NoError(t, err)
			registry := NewTypedToolRegistry()
			require.NoError(t, RegisterTool(registry, tool))
			// Act.
			result, err := registry.InvokeTool(context.Background(), "lookup", `{}`)
			require.NoError(t, err)
			buffer[9] = '9'
			// Assert: publication is isolated in both directions.
			require.JSONEq(t, `{"value":1}`, string(result))
			result[9] = '2'
			require.JSONEq(t, `{"value":9}`, string(buffer))
		})
	}
}

type task45ObjectArrayArgs struct {
	Name string `json:"name"`
}

func (task45ObjectArrayArgs) JSONSchema() JSONDocument {
	return JSONDocument(`{"type":["object"],"properties":{"name":{"type":"string"}},` +
		`"required":["name"],"additionalProperties":false}`)
}

func TestTypedToolAcceptsObjectOnlyTypeArray(t *testing.T) {
	t.Parallel()
	// Arrange: singleton type arrays are an object-only JSON Schema form.
	tool, err := NewTypedTool("lookup", func(_ context.Context, args task45ObjectArrayArgs) (string, error) {
		return args.Name, nil
	})
	require.NoError(t, err)
	// Act.
	value, err := tool.Invoke(context.Background(), `{"name":"known"}`)
	// Assert: constructor admission and original-schema validation agree.
	require.NoError(t, err)
	require.Equal(t, "known", value)
	require.Error(t, tool.ValidateArgs(`[]`))
}
