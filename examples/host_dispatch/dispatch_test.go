package hostdispatch

import (
	"context"
	"errors"
	"testing"

	"github.com/skosovsky/toolsy"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func sessionFor(t *testing.T, tools ...toolsy.Tool) *toolsy.Session {
	t.Helper()
	registry, err := toolsy.NewRegistryBuilder().Add(tools...).Build()
	require.NoError(t, err)
	session, err := toolsy.NewSession(registry)
	require.NoError(t, err)
	return session
}

func newBatch(t *testing.T, execution *prompty.PromptExecution, response *prompty.Response) *Batch {
	t.Helper()
	validator, err := NewValidator()
	require.NoError(t, err)
	batch, err := Admit(
		t.Context(),
		execution,
		response,
		nil,
		validator,
		prompty.ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"},
	)
	require.NoError(t, err)
	return batch
}

func responseFor(names ...string) *prompty.Response {
	parts := make([]prompty.ContentPart, len(names))
	for i, name := range names {
		parts[i] = prompty.ToolCallPart{ID: name, Name: name, Args: `{"id":9007199254740993}`}
	}
	return prompty.NewResponse(parts)
}

func TestFullSDKDispatchAndContinuation(t *testing.T) {
	t.Parallel()
	// Arrange: actual SDK HTTP, real Session/Extractor and registered context codec.
	fixture := NewFixture()
	t.Cleanup(fixture.Server.Close)
	execution, err := Execution("lookup")
	require.NoError(t, err)
	var effects []toolsy.ToolInput
	session := sessionFor(
		t,
		LookupTool{Name: "lookup", Effect: func(input toolsy.ToolInput) { effects = append(effects, input) }},
	)
	validator, err := NewValidator()
	require.NoError(t, err)
	// Act.
	response, dispatchErr := fixture.Client.Execute(t.Context(), execution)
	require.NoError(t, dispatchErr)
	checkpoint, err := SaveResponse(response, fixture.Destination)
	require.NoError(t, err)
	restored, err := RestoreResponse(checkpoint, fixture.Destination)
	require.NoError(t, err)
	batch, err := Admit(t.Context(), execution, restored, dispatchErr, validator, fixture.Destination)
	require.NoError(t, err)
	require.NoError(t, batch.Run(t.Context(), session))
	continuation, err := batch.Execution()
	require.NoError(t, err)
	final, err := fixture.Client.Execute(t.Context(), continuation)
	// Assert: correlation inside execution and on actual next provider request.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeCompleted, final.Outcome)
	require.Len(t, effects, 2)
	require.Equal(t, "native-one", effects[0].CallID)
	require.Equal(t, "native-two", effects[1].CallID)
	for _, effect := range effects {
		require.JSONEq(t, `{"id":9007199254740993}`, string(effect.ArgsJSON))
		require.Contains(t, string(effect.ArgsJSON), "9007199254740993")
	}
	requests := fixture.Requests()
	require.Len(t, requests, 2)
	wire, err := DecodeRequests(requests[1])
	require.NoError(t, err)
	messages := wire["messages"].([]any)
	calls := messages[1].(map[string]any)["tool_calls"].([]any)
	require.Equal(t, "native-one", calls[0].(map[string]any)["id"])
	require.Equal(t, "native-two", calls[1].(map[string]any)["id"])
	for i, id := range []string{"native-one", "native-two"} {
		message := messages[i+2].(map[string]any)
		require.Equal(t, id, message["tool_call_id"])
		require.Equal(t, `{"id":9007199254740993}`, message["content"])
	}
	require.Len(t, execution.Messages, 1)
}

func TestAdmissionRejectsWholeBatchBeforeEffects(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		calls  []prompty.ContentPart
		mutate func(*prompty.PromptExecution)
	}{
		{"invalid last", []prompty.ContentPart{prompty.ToolCallPart{ID: "one", Name: "lookup", Args: `{"id":1}`}, prompty.ToolCallPart{ID: "two", Name: "lookup", Args: `{"id":"bad"}`}}, nil},
		{"scope", []prompty.ContentPart{prompty.ToolCallPart{ID: "one", Name: "admin", Args: `{"id":1}`}}, nil},
		{"duplicate", []prompty.ContentPart{prompty.ToolCallPart{ID: "same", Name: "lookup", Args: `{"id":1}`}, prompty.ToolCallPart{ID: "same", Name: "lookup", Args: `{"id":1}`}}, nil},
		{"idless ambiguous", []prompty.ContentPart{prompty.ToolCallPart{Name: "lookup", Args: `{"id":1}`}, prompty.ToolCallPart{Name: "lookup", Args: `{"id":1}`}}, nil},
		{"forced", []prompty.ContentPart{prompty.ToolCallPart{ID: "one", Name: "lookup", Args: `{"id":1}`}}, func(e *prompty.PromptExecution) { e.ForcedTool = "other" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			execution, err := Execution("lookup", "other")
			require.NoError(t, err)
			if tc.mutate != nil {
				tc.mutate(execution)
			}
			validator, err := NewValidator()
			require.NoError(t, err)
			effects := 0
			session := sessionFor(t, LookupTool{Name: "lookup", Effect: func(toolsy.ToolInput) { effects++ }})
			// Act.
			batch, err := Admit(
				t.Context(),
				execution,
				prompty.NewResponse(tc.calls),
				nil,
				validator,
				prompty.ProfileIdentity{},
			)
			if batch != nil {
				_ = batch.Run(t.Context(), session)
			}
			// Assert.
			require.Error(t, err)
			require.Nil(t, batch)
			require.Zero(t, effects)
		})
	}
}

func TestHostApprovalPreservesErrorAndResumeCursor(t *testing.T) {
	t.Parallel()
	// Arrange: caller gate emits the real consumer pending-approval error before effect.
	execution, err := Execution("before", "approve", "after")
	require.NoError(t, err)
	batch := newBatch(t, execution, responseFor("before", "approve", "after"))
	approved := false
	pending := &toolsy.PendingApprovalError{}
	var effects []string
	record := func(in toolsy.ToolInput) { effects = append(effects, in.CallID) }
	session := sessionFor(
		t,
		LookupTool{Name: "before", Effect: record},
		LookupTool{
			Name:   "approve",
			Effect: record,
			Before: func(_ context.Context, _ toolsy.ToolInput, yield func(toolsy.Chunk) error) error {
				if approved {
					return nil
				}
				if yieldErr := yield(
					toolsy.Chunk{Event: toolsy.EventControl, Control: &toolsy.PauseSignal{Reason: "approval"}},
				); yieldErr != nil {
					return yieldErr
				}
				return pending
			},
		},
		LookupTool{Name: "after", Effect: record},
	)
	// Act.
	err = batch.Run(t.Context(), session)
	// Assert: partial evidence, original identity, no false result or cancellation.
	require.ErrorIs(t, err, toolsy.ErrPause)
	var got *toolsy.PendingApprovalError
	require.ErrorAs(t, err, &got)
	require.Same(t, pending, got)
	require.Equal(t, []string{"before"}, effects)
	call, reason := batch.Pending()
	require.Equal(t, "approve", call.ID)
	require.ErrorIs(t, reason, toolsy.ErrPause)
	_, err = batch.Execution()
	require.Error(t, err)
	require.NoError(t, t.Context().Err())
	// Act: explicit caller decision, same pending ID and no completed-call replay.
	approved = true
	require.NoError(t, batch.ResumeApproval(t.Context(), session))
	// Assert.
	require.Equal(t, []string{"before", "approve", "after"}, effects)
	updated, err := batch.Execution()
	require.NoError(t, err)
	require.Len(t, updated.Messages, 5)
}

func TestBusinessFailureAndUnsupportedProvider(t *testing.T) {
	t.Parallel()
	// Arrange / Act: real RunCall returns business error with nil Go error.
	call := prompty.ToolCallPart{ID: "failure", Name: "lookup", Args: `{"id":1}`}
	session := sessionFor(t, LookupTool{Name: "lookup", BusinessFailure: true})
	outcome, err := session.RunCall(
		t.Context(),
		toolsy.ToolCall{ToolName: call.Name, Input: toolsy.ToolInput{CallID: call.ID, ArgsJSON: []byte(call.Args)}},
	)
	// Assert.
	require.NoError(t, err)
	require.NotNil(t, outcome.ExecutionError)
	result, err := Project(call, outcome)
	require.NoError(t, err)
	require.True(t, result.IsError)
	require.Equal(t, "Tool rejected the request.", result.Content[0].(prompty.TextPart).Text)
	fixture := NewFixture()
	t.Cleanup(fixture.Server.Close)
	execution, err := Execution("lookup")
	require.NoError(t, err)
	execution = execution.AddMessage(prompty.ChatMessage{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{call}}).
		AddMessage(prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{result}})
	_, err = fixture.Client.Execute(t.Context(), execution)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	require.Empty(t, fixture.Requests())
}

func TestProjectionPolicy(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		mutate func(*toolsy.ToolOutcome)
	}{
		{"internal", func(o *toolsy.ToolOutcome) { o.Envelope.Audience = toolsy.AudienceInternal }},
		{"user", func(o *toolsy.ToolOutcome) { o.Envelope.Audience = toolsy.AudienceUser }},
		{"unknown audience", func(o *toolsy.ToolOutcome) { o.Envelope.Audience = "unknown" }},
		{"binary", func(o *toolsy.ToolOutcome) { o.Envelope.DeliveryClass = toolsy.DeliveryClassBinary }},
		{"unknown MIME", func(o *toolsy.ToolOutcome) { o.ResultMimeType = "application/octet-stream" }},
		{"unknown delivery", func(o *toolsy.ToolOutcome) { o.Envelope.DeliveryClass = "unknown" }},
		{"invalid UTF8 text", func(o *toolsy.ToolOutcome) {
			o.Result = []byte{255}
			o.ResultMimeType = "text/plain"
			o.Envelope.DeliveryClass = toolsy.DeliveryClassText
		}},
		{"invalid UTF8 JSON", func(o *toolsy.ToolOutcome) { o.Result = []byte{'{', '"', 'x', '"', ':', '"', 255, '"', '}'} }},
		{"invalid JSON", func(o *toolsy.ToolOutcome) { o.Result = []byte(`{`) }},
		{"control", func(o *toolsy.ToolOutcome) {
			o.Controls = []toolsy.ControlSignal{&toolsy.PauseSignal{Reason: "private"}}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			o := toolsy.ToolOutcome{
				Status:         toolsy.OutcomeSuccess,
				Result:         []byte(`{"safe":true}`),
				ResultMimeType: "application/json",
				Envelope: toolsy.ToolEnvelope{
					Audience:      toolsy.AudienceModel,
					DeliveryClass: toolsy.DeliveryClassStructured,
				},
			}
			tc.mutate(&o)
			// Act / Assert.
			r, err := Project(prompty.ToolCallPart{ID: "call", Name: "lookup"}, o)
			require.ErrorIs(t, err, ErrProjection)
			require.Empty(t, r.Content)
		})
	}
}

func TestOriginalSchemasAfterProviderLowering(t *testing.T) {
	t.Parallel()
	// Arrange: transport lowering admits nullable id, original schema does not.
	source := prompty.JSONDocument(
		`{"type":"object","properties":{"id":{"type":"integer"}},"additionalProperties":false}`,
	)
	compiled, err := prompty.CompileProviderSchema(
		prompty.SchemaCompileRequest{
			Source:  source,
			Role:    prompty.SchemaToolInput,
			Profile: prompty.ProfileIdentity{Provider: "openai", Endpoint: "https://fixture.invalid", Model: "fixture"},
			Policy: prompty.SchemaPolicy{
				Mode:              prompty.SchemaLowering,
				AllowedTransforms: []string{"fixture.nullable"},
			},
			Validator: prompty.NewJSONSchemaValidator(nil),
		},
		nullableToolMapper{},
	)
	require.NoError(t, err)
	require.Contains(t, string(compiled.Translated()), "null")
	execution, err := Execution("lookup")
	require.NoError(t, err)
	// Rebind the original schema with its matching original scope, never translated.
	definition := execution.Tools[0]
	definition.Parameters = compiled.Source()
	execution = &prompty.PromptExecution{Tools: []prompty.ToolDefinition{definition}}
	execution, err = execution.WithToolScope(
		prompty.ToolScope{Allowed: []prompty.ToolManifest{prompty.ToolManifestFromDefinition(definition)}},
	)
	require.NoError(t, err)
	validator, err := NewValidator()
	require.NoError(t, err)
	// Act.
	batch, err := Admit(
		t.Context(),
		execution,
		prompty.NewResponse(
			[]prompty.ContentPart{prompty.ToolCallPart{ID: "one", Name: "lookup", Args: `{"id":null}`}},
		),
		nil,
		validator,
		prompty.ProfileIdentity{},
	)
	// Assert.
	require.Error(t, err)
	require.Nil(t, batch)
	require.Equal(t, string(source), string(execution.Tools[0].Parameters))
}

func TestDispatchErrorAndResumeInfrastructureNeverReplay(t *testing.T) {
	t.Parallel()
	// Arrange.
	execution, err := Execution("lookup")
	require.NoError(t, err)
	validator, err := NewValidator()
	require.NoError(t, err)
	cause := errors.New("transport unknown")
	// Act / Assert: a partial provider response grants no effects.
	batch, err := Admit(t.Context(), execution, responseFor("lookup"), cause, validator, prompty.ProfileIdentity{})
	require.ErrorIs(t, err, cause)
	require.Nil(t, batch)
	batch = newBatch(t, execution, responseFor("lookup"))
	invoked := 0
	session := sessionFor(
		t,
		LookupTool{
			Name:   "lookup",
			Before: func(context.Context, toolsy.ToolInput, func(toolsy.Chunk) error) error { invoked++; return cause },
		},
	)
	err = batch.Run(t.Context(), session)
	require.Error(t, err)
	err = batch.Run(t.Context(), session)
	require.Error(t, err)
	require.Equal(t, 1, invoked)
}

// nullableToolMapper models an explicitly permitted tool transport transformation.
// Current SDK tool mappers preserve schemas; this regression protects future ports.
type nullableToolMapper struct{}

func (nullableToolMapper) MapSchema(_ prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	return prompty.SchemaMapping{
		Schema: prompty.JSONDocument(
			`{"type":"object","properties":{"id":{"type":["integer","null"]}},"additionalProperties":false}`,
		),
		Transformations: []prompty.SchemaTransformation{
			{ID: "fixture.nullable", Pointer: "/properties/id", Reason: "fixture transport accepts null"},
		},
	}, nil
}
