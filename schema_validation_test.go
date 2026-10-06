package prompty

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginalSchemaRejectsDecodableInvalidValues(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"value":1}`, `{}`, `{"value":null}`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			type output struct {
				Value int `json:"value"`
			}
			exec := SimplePrompt("fixture")
			exec.ResponseFormat = &SchemaDefinition{Schema: JSONDocument(
				`{"type":"object","properties":{"value":{"type":"integer","minimum":10}},"required":["value"]}`)}
			invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
				return NewResponse([]ContentPart{TextPart{Text: raw}}), nil
			}}
			// Act.
			result, err := ExecuteWithStructuredOutput[output](context.Background(), invoker, exec)
			// Assert.
			require.Nil(t, result)
			require.ErrorIs(t, err, ErrSchemaMismatch)
		})
	}
}

func TestToolBatchUsesOriginalExecutionSchemasBeforeAnyHandler(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"value":1}`, `{}`, `{"value":null}`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			// Arrange: typed registry allows integers; execution requires minimum 10.
			type input struct {
				Value int `json:"value"`
			}
			invoked := 0
			registry := NewTypedToolRegistry()
			tool, err := NewTypedTool(
				"lookup",
				func(_ context.Context, _ input) (string, error) { invoked++; return "fixture", nil },
			)
			require.NoError(t, err)
			require.NoError(t, RegisterTool(registry, tool))
			exec := SimplePrompt("fixture")
			exec.Tools = []ToolDefinition{{Name: "lookup", Parameters: JSONDocument(
				`{"type":"object","properties":{"value":{"type":"integer","minimum":10}},"required":["value"]}`)}}
			invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
				return NewResponse([]ContentPart{
					ToolCallPart{ID: "first", Name: "lookup", Args: `{"value":10}`},
					ToolCallPart{ID: "second", Name: "lookup", Args: raw},
				}), nil
			}}
			// Act.
			_, executionErr := ExecuteWithTypedTools(context.Background(), invoker, exec, registry)
			// Assert: a valid earlier sibling also remains unexecuted.
			require.ErrorIs(t, executionErr, ErrSchemaMismatch)
			require.Zero(t, invoked)
		})
	}
}

func TestUnsupportedToolSchemaStopsBeforeInvocation(t *testing.T) {
	t.Parallel()
	// Arrange.
	called := false
	exec := SimplePrompt("fixture")
	exec.Tools = []ToolDefinition{
		{Name: "lookup", Parameters: JSONDocument(`{"$ref":"https://fixture.invalid/not-provided"}`)},
	}
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		called = true
		return NewResponse(nil), nil
	}}
	// Act.
	_, err := ExecuteWithTypedTools(context.Background(), invoker, exec, NewTypedToolRegistry())
	// Assert.
	require.ErrorIs(t, err, ErrSchemaUnsupported)
	require.False(t, called)
}

func TestStructuredStreamValidatesWholeDocument(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, raw, schema string
		want              error
	}{
		{"valid array", `[{"value":10},{"value":11}]`, `{"type":"array","minItems":2,"maxItems":2,"items":{"type":"object","properties":{"value":{"type":"integer","minimum":10}},"required":["value"]}}`, nil},
		{"too few", `[{"value":10}]`, `{"type":"array","minItems":2}`, ErrSchemaMismatch},
		{"too many", `[{"value":10},{"value":11}]`, `{"type":"array","maxItems":1}`, ErrSchemaMismatch},
		{"invalid item", `[{"value":1}]`, `{"type":"array","items":{"type":"object","properties":{"value":{"minimum":10}}}}`, ErrSchemaMismatch},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			type output struct {
				Value int `json:"value"`
			}
			exec := SimplePrompt("fixture")
			exec.ResponseFormat = &SchemaDefinition{Schema: JSONDocument(tc.schema)}
			invoker := &scriptedInvoker{
				generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
					return func(yield func(*ResponseChunk, error) bool) {
						yield(
							&ResponseChunk{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
								Content: []ContentPart{TextPart{Text: tc.raw}},
								Kind:    StreamFinish,
								Outcome: OutcomeCompleted,
							},
							nil,
						)
					}
				},
			}
			// Act: items are provisional even if all Go decoding succeeds.
			items, err := collectSeq(StreamStructuredOutput[output](context.Background(), invoker, exec, StreamNative))
			// Assert.
			require.NotEmpty(t, items)
			if tc.want == nil {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, tc.want)
			}
		})
	}
}

func TestStructuredStreamRefusalAndIncompleteAreNotSuccess(t *testing.T) {
	t.Parallel()
	for _, outcome := range []Outcome{OutcomeRefusal, OutcomeIncomplete, OutcomeToolCalls} {
		t.Run(string(outcome), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			invoker := &scriptedInvoker{
				generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
					return func(yield func(*ResponseChunk, error) bool) {
						yield(
							&ResponseChunk{Identity: StreamIdentity{MessageID: "", PartIDs: []string{"text"}},
								Content: []ContentPart{TextPart{Text: `{"answer":"looks valid"}`}},
								Kind:    StreamFinish,
								Outcome: outcome,
							},
							nil,
						)
					}
				},
			}
			// Act.
			items, err := collectSeq(
				StreamStructuredOutput[valueSchemaResult](
					context.Background(),
					invoker,
					SimplePrompt("fixture"),
					StreamNative,
				),
			)
			// Assert.
			require.Empty(t, items) // non-completed outcome was already known in the content frame
			require.ErrorIs(t, err, ErrResponseOutcome)
			require.NotErrorIs(t, err, ErrSchemaMismatch)
		})
	}
}

func TestStructuredStreamUnsupportedSchemaStopsBeforeInvocation(t *testing.T) {
	t.Parallel()
	// Arrange.
	called := false
	invoker := &scriptedInvoker{
		generateStream: func(context.Context, *PromptExecution) iter.Seq2[*ResponseChunk, error] {
			called = true
			return nil
		},
	}
	exec := SimplePrompt("fixture")
	exec.ResponseFormat = &SchemaDefinition{Schema: JSONDocument(`{"$ref":"https://fixture.invalid/unprovided.json"}`)}
	// Act.
	_, err := collectSeq(StreamStructuredOutput[valueSchemaResult](context.Background(), invoker, exec, StreamNative))
	// Assert.
	require.ErrorIs(t, err, ErrSchemaUnsupported)
	require.False(t, called)
}

func TestSchemaReferencesRequireExplicitResources(t *testing.T) {
	t.Parallel()
	// Arrange.
	source := JSONDocument(`{"$ref":"https://example.invalid/private.json"}`)
	resource := JSONDocument(`{"type":"integer","minimum":10}`)
	validator := NewJSONSchemaValidator(map[string]JSONDocument{"https://example.invalid/private.json": resource})
	resource[0] = 'x'
	// Act.
	_, missing := NewJSONSchemaValidator(nil).CompileSchema(source, SchemaDialect2020)
	compiled, supplied := validator.CompileSchema(source, SchemaDialect2020)
	// Assert.
	require.ErrorIs(t, missing, ErrSchemaUnsupported)
	require.NoError(t, supplied)
	require.ErrorIs(t, compiled.ValidateJSON(JSONDocument("1")), ErrSchemaMismatch)
	require.NoError(t, compiled.ValidateJSON(JSONDocument("10")))
}

func TestIncompleteJSONOutcomeIsNotRepairableValidation(t *testing.T) {
	t.Parallel()
	// Arrange.
	type output struct {
		Value int `json:"value"`
	}
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		return &Response{
			Content: []ContentPart{TextPart{Text: `{"value":1}`}},
			Outcome: OutcomeIncomplete, FinishReason: "length",
		}, nil
	}}
	// Act.
	result, err := ExecuteWithStructuredOutput[output](context.Background(), invoker, SimplePrompt("fixture"))
	// Assert.
	require.Nil(t, result)
	require.ErrorIs(t, err, ErrResponseOutcome)
	var validation *ValidationError
	require.NotErrorAs(t, err, &validation)
}
