package adapter

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

const originalToolLoweringFixture = `{"type":"object","properties":{"value":{"type":"integer","minimum":10}},"required":["value"]}`

type toolSchemaLoweringFixture struct {
	*checkedFixture

	args string
}

func (*toolSchemaLoweringFixture) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	if request.Role != prompty.SchemaToolInput {
		return prompty.SchemaMapping{Schema: prompty.CloneJSONDocument(request.Source)}, nil
	}
	return prompty.SchemaMapping{
		Schema: prompty.JSONDocument(`{"type":"object","properties":{"value":{"type":"integer"}}}`),
		Transformations: []prompty.SchemaTransformation{
			{ID: "fixture.drop-minimum", Pointer: "/properties/value/minimum", Reason: "fixture mapping limitation"},
			{ID: "fixture.drop-required", Pointer: "/required", Reason: "fixture mapping limitation"},
		},
	}, nil
}

func (f *toolSchemaLoweringFixture) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	f.calls++
	response := prompty.NewResponse(
		[]prompty.ContentPart{prompty.ToolCallPart{ID: "call", Name: "lookup", Args: f.args}},
	)
	response.Outcome = prompty.OutcomeToolCalls
	response.Usage = prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}}
	return response, nil
}

func (f *toolSchemaLoweringFixture) StreamEvents(
	context.Context,
	*prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		f.calls++
		for _, event := range []*prompty.ResponseChunk{
			{Kind: prompty.StreamDelta, Identity: prompty.StreamIdentity{PartIDs: []string{"tool"}},
				Content: []prompty.ContentPart{prompty.ToolCallPart{ID: "call", Name: "lookup", Args: f.args}}},
			{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeToolCalls},
			{Kind: prompty.StreamUsage, Usage: prompty.Usage{TotalTokens: 9, Known: []prompty.UsageCounter{prompty.UsageTotal}}},
		} {
			if !yield(event, nil) {
				return
			}
		}
	}
}

func toolLoweringExecution() *prompty.PromptExecution {
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = []prompty.ToolDefinition{
		{Name: "lookup", Parameters: prompty.JSONDocument(originalToolLoweringFixture)},
	}
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode:              prompty.SchemaLowering,
		AllowedTransforms: []string{"fixture.drop-minimum", "fixture.drop-required"},
	}
	return exec
}

func TestToolLoweringValidatesOriginalAcrossExecutionModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, args := range []string{`{"value":10}`, `{"value":1}`, `{}`, `{"value":null}`} {
			t.Run(string(mode)+args, func(t *testing.T) {
				t.Parallel()
				// Arrange: lowering permits below-minimum/missing values; null also exercises original rejection.
				fixture := &toolSchemaLoweringFixture{
					checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
					args:           args,
				}
				exec := toolLoweringExecution()
				var decoded struct {
					Value int `json:"value"`
				}
				require.NoError(t, json.Unmarshal([]byte(args), &decoded))
				before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
				require.NoError(t, err)
				// Act.
				response, err := executeToolSchemaFixture(t, NewClient(fixture), exec, mode)
				// Assert: post-dispatch mismatch retains known content, counts and exact schema association.
				if args == `{"value":10}` {
					require.NoError(t, err)
				} else {
					require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
				}
				require.NotNil(t, response)
				require.Equal(t, prompty.OutcomeToolCalls, response.Outcome)
				require.Equal(t, 9, response.Usage.TotalTokens)
				require.Equal(
					t,
					[]prompty.ContentPart{prompty.ToolCallPart{ID: "call", Name: "lookup", Args: args}},
					response.Content,
				)
				require.Len(t, response.Diagnostics.Schemas, 1)
				report := response.Diagnostics.Schemas[0]
				require.Equal(t, "lookup", report.Name)
				require.Equal(t, prompty.SchemaToolInput, report.Role)
				require.Equal(t, fixture.descriptor.Profiles[0].Identity, report.Profile)
				require.Equal(t, prompty.SchemaLowering, report.Guarantee)
				digest := sha256.Sum256([]byte(originalToolLoweringFixture))
				require.Equal(t, hex.EncodeToString(digest[:]), report.SourceDigest)
				require.NotEqual(t, report.SourceDigest, report.TranslatedDigest)
				require.Len(t, report.Transformations, 2)
				after, marshalErr := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
				require.NoError(t, marshalErr)
				require.Equal(t, before, after)
				require.Equal(t, 1, fixture.calls)
			})
		}
	}
}

func executeToolSchemaFixture(
	t *testing.T,
	client prompty.Invoker,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
) (*prompty.Response, error) {
	t.Helper()
	if mode == "" {
		return client.Execute(context.Background(), exec)
	}
	stream := client.ExecuteStream(context.Background(), exec, mode)
	terminals := 0
	for event := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	status := stream.Status()
	require.Equal(t, 1, terminals)
	if status.Err == nil {
		require.Equal(t, prompty.StreamCompleted, status.State)
	} else {
		require.Equal(t, prompty.StreamFailed, status.State)
	}
	return status.Result, status.Err
}

func TestToolLoweringDoesNotPermitInvalidArgumentsToReachHandler(t *testing.T) {
	t.Parallel()
	for _, args := range []string{`{"value":1}`, `{}`, `{"value":null}`} {
		t.Run(args, func(t *testing.T) {
			t.Parallel()
			// Arrange: registry accepts typed integers, unlike the execution's tighter contract.
			fixture := &toolSchemaLoweringFixture{
				checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
				args:           args,
			}
			registry := prompty.NewTypedToolRegistry()
			invoked := 0
			type input struct {
				Value int `json:"value"`
			}
			tool, err := prompty.NewTypedTool(
				"lookup",
				func(_ context.Context, _ input) (string, error) { invoked++; return "fixture", nil },
			)
			require.NoError(t, err)
			require.NoError(t, prompty.RegisterTool(registry, tool))
			// Act.
			_, err = prompty.ExecuteWithTypedTools(
				context.Background(),
				NewClient(fixture),
				toolLoweringExecution(),
				registry,
			)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
			require.Zero(t, invoked)
			require.Equal(t, 1, fixture.calls)
		})
	}
}
