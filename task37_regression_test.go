package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestToolExecutionScopeFailsBeforeAnyHandler(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		tools  []ToolDefinition
		forced string
		calls  []ContentPart
	}{
		{"outside", []ToolDefinition{{Name: "lookup"}}, "", []ContentPart{
			ToolCallPart{ID: "a", Name: "lookup", Args: "{}"},
			ToolCallPart{ID: "b", Name: "delete_all", Args: "{}"},
		}},
		{"empty scope", nil, "", []ContentPart{ToolCallPart{ID: "a", Name: "lookup", Args: "{}"}}},
		{"unknown tool", []ToolDefinition{{Name: "lookup"}}, "", []ContentPart{
			ToolCallPart{ID: "a", Name: "lookup", Args: "{}"},
			ToolCallPart{ID: "b", Name: "unknown", Args: "{}"},
		}},
		{"forced", []ToolDefinition{{Name: "lookup"}, {Name: "delete_all"}}, "lookup",
			[]ContentPart{ToolCallPart{ID: "a", Name: "delete_all", Args: "{}"}}},
		{"duplicate IDs", []ToolDefinition{{Name: "lookup"}}, "", []ContentPart{
			ToolCallPart{ID: "a", Name: "lookup", Args: "{}"},
			ToolCallPart{ID: "a", Name: "lookup", Args: "{}"},
		}},
		{"duplicate identity-only calls", []ToolDefinition{{Name: "lookup"}}, "", []ContentPart{
			ToolCallPart{ID: "a", Name: "lookup"},
			ToolCallPart{ID: "a", Name: "lookup", Args: "{}"},
		}},
		{"duplicate delta calls", []ToolDefinition{{Name: "lookup"}}, "", []ContentPart{
			ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: "{"},
			ToolCallPart{ID: "a", Name: "lookup", ArgsChunk: "}"},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			invoked := 0
			reg := NewTypedToolRegistry()
			for _, name := range []string{"lookup", "delete_all"} {
				tool, err := NewTypedTool(name, func(struct{}) (string, error) {
					invoked++
					return "fixture", nil
				})
				require.NoError(t, err)
				require.NoError(t, RegisterTool(reg, tool))
			}
			exec := SimplePrompt("fixture")
			exec.Tools, exec.ForcedTool = tc.tools, tc.forced
			invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
				return NewResponse(tc.calls), nil
			}}
			// Act.
			_, err := ExecuteWithToolValidation(context.Background(), invoker, exec, reg)
			// Assert.
			require.ErrorIs(t, err, ErrToolScope)
			require.Zero(t, invoked)
		})
	}
}

func TestStructuredArraySeparators(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw   string
		valid bool
	}{
		{"[]", true}, {"[{}]", true}, {"[{},{}]", true},
		{"[,{}]", false}, {"[{},,{}]", false}, {"[{},]", false}, {"[{}{}]", false},
	} {
		t.Run(tc.raw, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			parser := newStructuredStreamParser[struct{}]()
			var parseErr error
			// Act: split every separator and object boundary into individual chunks.
			for _, ch := range tc.raw {
				if _, parseErr = parser.feed(string(ch)); parseErr != nil {
					break
				}
			}
			if parseErr == nil {
				parseErr = parser.finish()
			}
			// Assert.
			if tc.valid {
				require.NoError(t, parseErr)
			} else {
				require.Error(t, parseErr)
			}
		})
	}
}
