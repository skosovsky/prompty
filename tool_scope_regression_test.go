package prompty

import (
	"context"
	"encoding/json"
	"iter"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

type mutatingBoundaryCompiler struct{}

func (mutatingBoundaryCompiler) CompileSchema(source JSONDocument, dialect string) (ValueValidator, error) {
	// The delegate enforces exactly the mutated schema, not a false validator.
	copy(source, []byte(strings.ReplaceAll(string(source), "object", "string")))
	return NewJSONSchemaValidator(nil).CompileSchema(source, dialect)
}

func TestToolCompilerSourceMutationRejectedBeforeModel(t *testing.T) {
	t.Parallel()
	// Arrange: the compiler receives the same type of writable bytes as normal ports.
	execution := SimplePrompt("work")
	execution.Tools = []ToolDefinition{{Name: "read", Parameters: JSONDocument(`{"type":"object"}`)}}
	execution, err := execution.WithToolScope(
		ToolScope{Allowed: []ToolManifest{ToolManifestFromDefinition(execution.Tools[0])}},
	)
	require.NoError(t, err)
	execution.Validator = mutatingBoundaryCompiler{}
	calls := 0
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		calls++
		return NewResponse([]ContentPart{TextPart{Text: "ok"}}), nil
	}}
	// Act.
	result, err := ExecuteWithTypedTools(context.Background(), invoker, execution, NewTypedToolRegistry())
	// Assert: no changed schema can cross dispatch, or alter the original snapshot.
	require.ErrorIs(t, err, ErrSchemaUnsupported)
	require.Zero(t, calls)
	require.NoError(t, ValidateExecutionToolScope(execution))
	require.NoError(t, ValidateExecutionToolScope(result))
	require.Equal(t, JSONDocument(`{"type":"object"}`), result.Tools[0].Parameters)
}

type scopeBoundaryOutput struct {
	Value int `json:"value"`
}

func TestCoreHelpersRecheckBoundScopeWithBYOTInvoker(t *testing.T) {
	t.Parallel()
	for _, helper := range []struct {
		name string
		run  func(context.Context, Invoker, *PromptExecution) error
	}{
		{"text", func(ctx context.Context, invoker Invoker, execution *PromptExecution) error {
			_, err := ExecuteAsText(ctx, invoker, execution)
			return err
		}},
		{"structured", func(ctx context.Context, invoker Invoker, execution *PromptExecution) error {
			_, err := ExecuteWithStructuredOutput[scopeBoundaryOutput](ctx, invoker, execution)
			return err
		}},
		{"stateful structured", func(ctx context.Context, invoker Invoker, execution *PromptExecution) error {
			_, err := NewStructuredExecutor[scopeBoundaryOutput](invoker, execution)(ctx)
			return err
		}},
		{"stream native", func(ctx context.Context, invoker Invoker, execution *PromptExecution) error {
			_, err := collectSeq(StreamStructuredOutput[scopeBoundaryOutput](ctx, invoker, execution, StreamNative))
			return err
		}},
		{"stream buffered", func(ctx context.Context, invoker Invoker, execution *PromptExecution) error {
			_, err := collectSeq(StreamStructuredOutput[scopeBoundaryOutput](ctx, invoker, execution, StreamBuffered))
			return err
		}},
	} {
		for _, remove := range []bool{false, true} {
			t.Run(
				helper.name+map[bool]string{false: "/renamed", true: "/missing required"}[remove],
				func(t *testing.T) {
					t.Parallel()
					// Arrange: a custom invoker has no adapter preflight to supply missing checks.
					execution := SimplePrompt("work")
					execution.Tools = []ToolDefinition{{Name: "read"}}
					execution, err := execution.WithToolScope(
						ToolScope{Allowed: []ToolManifest{{Name: "read"}}, Required: []ToolRequirement{{Name: "read"}}},
					)
					require.NoError(t, err)
					if remove {
						execution.Tools = nil
					} else {
						execution.Tools[0].Name = "delete_all"
					}
					calls := 0
					invoker := &scriptedInvoker{
						generate: func(context.Context, *PromptExecution) (*Response, error) {
							calls++
							return NewResponse([]ContentPart{TextPart{Text: `{"value":1}`}}), nil
						},
						generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
							calls++
							return func(yield func(*ResponseChunk, error) bool) {
								yield(
									&ResponseChunk{
										Identity: StreamIdentity{PartIDs: []string{"text"}},
										Content:  []ContentPart{TextPart{Text: `{"value":1}`}},
										Kind:     StreamFinish,
										Outcome:  OutcomeCompleted,
									},
									nil,
								)
							}
						},
					}
					// Act.
					err = helper.run(context.Background(), invoker, execution)
					// Assert: every core helper enforces the retained declaration itself.
					require.ErrorIs(t, err, ErrToolScope)
					require.Zero(t, calls)
				},
			)
		}
	}
}

type cancelingBoundaryInvoker struct {
	cancel context.CancelFunc
	calls  int
}

func (*cancelingBoundaryInvoker) ValidateToolCall(string, string) error { return nil }
func (invoker *cancelingBoundaryInvoker) InvokeTool(context.Context, string, string) (json.RawMessage, error) {
	invoker.calls++
	invoker.cancel()
	return json.RawMessage(`"written"`), nil
}
func TestCustomToolInvokerCancellationMarksRetainedResult(t *testing.T) {
	t.Parallel()
	// Arrange: a BYOT invoker returns success after completing its side effect and canceling.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	tools := &cancelingBoundaryInvoker{cancel: cancel}
	execution := SimplePrompt("work")
	execution.Tools = []ToolDefinition{{Name: "write"}}
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		return NewResponse(
			[]ContentPart{
				ToolCallPart{ID: "first", Name: "write", Args: "{}"},
				ToolCallPart{ID: "second", Name: "write", Args: "{}"},
			},
		), nil
	}}
	// Act.
	result, err := ExecuteWithTypedTools(ctx, invoker, execution, tools)
	// Assert: registry-independent invocation status is consistent with cancellation.
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, tools.calls)
	require.Len(t, result.Messages, 3)
	part := result.Messages[2].Content[0].(ToolResultPart)
	require.True(t, part.IsError)
	require.Equal(t, "written", mustTextFromParts(t, part.Content))
}
