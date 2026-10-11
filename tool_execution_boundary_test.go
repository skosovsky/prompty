package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestMaterializationRejectsDisallowedDefinitionWithoutRequiredTools(t *testing.T) {
	t.Parallel()
	// Arrange: the original fail-open path had no RequiredTools.
	template, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleUser, Content: TextContent("work")}},
		WithTools([]ToolDefinition{{Name: "delete_all"}}),
	)
	require.NoError(t, err)
	plan, err := NewRenderPlan(template).WithToolScope(ToolScope{Allowed: []ToolManifest{{Name: "read"}}})
	require.NoError(t, err)
	// Act.
	execution, err := plan.Execute(context.Background())
	// Assert: nothing can be dispatched.
	require.ErrorIs(t, err, ErrToolScope)
	require.Nil(t, execution)
}

func TestExecutionScopePreservedAndRechecked(t *testing.T) {
	t.Parallel()
	// Arrange.
	definition := ToolDefinition{
		Name:         "read",
		Parameters:   JSONDocument(`{"type":"object","properties":{"id":{"const":9007199254740993}}}`),
		Capabilities: []string{"read"},
	}
	source := SimplePrompt("work")
	source.Tools = []ToolDefinition{definition}
	source.RequiredTools = []string{"read"}
	source.ForcedTool = "read"
	source, err := source.WithToolScope(
		ToolScope{
			Allowed:  []ToolManifest{ToolManifestFromDefinition(definition)},
			Required: []ToolRequirement{{Name: "read", Capabilities: []string{"read"}}},
		},
	)
	require.NoError(t, err)
	wire, err := MarshalExecution(source, WirePolicy{State: StatePreserve})
	require.NoError(t, err)
	restored, err := UnmarshalExecution(wire)
	require.NoError(t, err)
	// Act and Assert: getter/clone/message/wire boundaries retain isolated policy.
	for _, execution := range []*PromptExecution{source.Clone(), source.AddMessage(NewUserMessage("next")), restored} {
		require.NoError(t, ValidateExecutionToolScope(execution))
		scope := execution.ToolScope()
		require.NotNil(t, scope)
		scope.Allowed[0].Name = "delete_all"
		require.NoError(t, ValidateExecutionToolScope(execution))
		execution.Tools[0].Parameters = JSONDocument(`{"type":"object","properties":{"id":{"const":9007199254740992}}}`)
		calls := 0
		invoker := &scriptedInvoker{
			generate: func(context.Context, *PromptExecution) (*Response, error) { calls++; return &Response{}, nil },
		}
		_, invocationErr := ExecuteWithTypedTools(context.Background(), invoker, execution, NewTypedToolRegistry())
		require.ErrorIs(t, invocationErr, ErrToolScope)
		require.Zero(t, calls)
	}
	require.NoError(t, ValidateExecutionToolScope(source))
	wire.Tools[0].Name = "delete_all"
	_, err = UnmarshalExecution(wire)
	require.ErrorIs(t, err, ErrToolScope)
	wire.Format = "prompty.execution"
	_, err = UnmarshalExecution(wire)
	require.ErrorIs(t, err, ErrWireFormat)
}

func TestExecutionScopeRejectsInvalidDeclarations(t *testing.T) {
	t.Parallel()
	definition := ToolDefinition{Name: "read", Parameters: JSONDocument(`{"type":"object"}`)}
	for _, scope := range []ToolScope{
		{},
		{Allowed: []ToolManifest{{Name: "read"}}},
		{Allowed: []ToolManifest{ToolManifestFromDefinition(definition), ToolManifestFromDefinition(definition)}},
		{Allowed: []ToolManifest{{Name: " "}}},
		{Allowed: []ToolManifest{ToolManifestFromDefinition(definition)}, Required: []ToolRequirement{{Name: "read"}, {Name: "read"}}},
		{Allowed: []ToolManifest{ToolManifestFromDefinition(definition), {Name: "other"}}, Required: []ToolRequirement{{Name: "other"}}},
	} {
		// Arrange.
		execution := SimplePrompt("work")
		execution.Tools = []ToolDefinition{definition}
		// Act.
		_, err := execution.WithToolScope(scope)
		// Assert.
		require.ErrorIs(t, err, ErrToolScope)
	}
	for _, tools := range [][]ToolDefinition{{{Name: ""}}, {{Name: "read"}, {Name: "read"}}} {
		execution := SimplePrompt("work")
		execution.Tools = tools
		require.ErrorIs(t, ValidateExecutionToolScope(execution), ErrToolScope)
	}
}

func TestTypedToolCancellationKeepsCompletedResultAndErrors(t *testing.T) {
	t.Parallel()
	// Arrange: the first side effect completes while canceling the operation.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	handlerErr := errors.New("side effect completed with warning")
	calls := 0
	tool, err := NewTypedTool("write", func(callCtx context.Context, _ struct{}) (string, error) {
		require.Same(t, ctx, callCtx)
		calls++
		cancel()
		return "written", handlerErr
	})
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, tool))
	execution := SimplePrompt("work")
	execution.Tools = registry.Definitions()
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		return &Response{
			Outcome: OutcomeCompleted,
			Content: []ContentPart{
				ToolCallPart{ID: "first", Name: "write", Args: "{}"},
				ToolCallPart{ID: "second", Name: "write", Args: "{}"},
			},
		}, nil
	}}
	// Act.
	result, err := ExecuteWithTypedTools(ctx, invoker, execution, registry)
	// Assert: known data survives; no second call or retry.
	require.ErrorIs(t, err, context.Canceled)
	require.ErrorIs(t, err, handlerErr)
	require.Equal(t, 1, calls)
	require.Len(t, result.Messages, 3)
	part, ok := result.Messages[2].Content[0].(ToolResultPart)
	require.True(t, ok)
	require.Equal(t, "first", part.ToolCallID)
	require.True(t, part.IsError)
	require.Equal(t, "written", mustTextFromParts(t, part.Content))
}

func TestTypedToolHTTPReceivesCancellation(t *testing.T) {
	t.Parallel()
	// Arrange: cancellation happens only after the remote request starts.
	started := make(chan struct{})
	canceled := make(chan struct{})
	server := httptest.NewServer(
		http.HandlerFunc(
			func(_ http.ResponseWriter, r *http.Request) { close(started); <-r.Context().Done(); close(canceled) },
		),
	)
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	observedDeadline := make(chan time.Time, 1)
	tool, err := NewTypedTool("http", func(callCtx context.Context, _ struct{}) (string, error) {
		deadline, _ := callCtx.Deadline()
		observedDeadline <- deadline
		request, requestErr := http.NewRequestWithContext(callCtx, http.MethodGet, server.URL, nil)
		if requestErr != nil {
			return "", requestErr
		}
		response, requestErr := server.Client().Do(request)
		if response != nil {
			response.Body.Close()
		}
		return "request started", requestErr
	})
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, tool))
	done := make(chan error, 1)
	// Act: a host goroutine exercises the synchronous cooperative API.
	go func() { _, invokeErr := registry.InvokeTool(ctx, "http", "{}"); done <- invokeErr }()
	<-started
	cancel()
	// Assert.
	require.ErrorIs(t, <-done, context.Canceled)
	expectedDeadline, _ := ctx.Deadline()
	require.Equal(t, expectedDeadline, <-observedDeadline)
	<-canceled
}

func TestModelPartialResponseErrorDoesNotInvokeTools(t *testing.T) {
	t.Parallel()
	// Arrange.
	calls := 0
	tool, err := NewTypedTool(
		"write",
		func(context.Context, struct{}) (string, error) { calls++; return "written", nil },
	)
	require.NoError(t, err)
	registry := NewTypedToolRegistry()
	require.NoError(t, RegisterTool(registry, tool))
	execution := SimplePrompt("work")
	execution.Tools = registry.Definitions()
	transportErr := errors.New("post-dispatch failure")
	response := &Response{
		Outcome: OutcomeCompleted,
		Content: []ContentPart{ToolCallPart{ID: "first", Name: "write", Args: "{}"}},
	}
	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) { return response, transportErr },
	}
	// Act.
	result, err := ExecuteWithTypedTools(context.Background(), invoker, execution, registry)
	// Assert: reconciliation gets a cloned known response, with no side effect.
	require.ErrorIs(t, err, transportErr)
	require.Zero(t, calls)
	require.Len(t, result.Messages, 1)
	var failure *ToolDispatchError
	require.ErrorAs(t, err, &failure)
	require.Equal(t, response, failure.Response)
	response.Content = nil
	require.Len(t, failure.Response.Content, 1)
}

func TestCanceledToolExecutionNeverStartsModelOrHandler(t *testing.T) {
	t.Parallel()
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) { calls++; return &Response{}, nil },
	}
	// Act.
	_, err := ExecuteWithTypedTools(ctx, invoker, SimplePrompt("work"), NewTypedToolRegistry())
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Zero(t, calls)
}

type invocationContextKey struct{}

func TestConcurrentRegistryCallsUseOwnContexts(t *testing.T) {
	t.Parallel()
	// Arrange: finish registration before concurrent read-only invocation.
	registry := NewTypedToolRegistry()
	tool, err := NewTypedTool("read", func(ctx context.Context, _ struct{}) (string, error) {
		return ctx.Value(invocationContextKey{}).(string), nil
	})
	require.NoError(t, err)
	require.NoError(t, RegisterTool(registry, tool))
	var workers sync.WaitGroup
	failures := make(chan error, 16)
	// Act.
	for index := range 16 {
		workers.Go(func() {
			expected := strconv.Itoa(index)
			ctx := context.WithValue(context.Background(), invocationContextKey{}, expected)
			raw, invokeErr := registry.InvokeTool(ctx, "read", "{}")
			if invokeErr != nil {
				failures <- invokeErr
				return
			}
			var value string
			if decodeErr := json.Unmarshal(raw, &value); decodeErr != nil {
				failures <- decodeErr
				return
			}
			if value != expected {
				failures <- fmt.Errorf("got %q, want %q", value, expected)
			}
		})
	}
	workers.Wait()
	close(failures)
	// Assert.
	for failure := range failures {
		require.NoError(t, failure)
	}
}

func TestRequiredForcedAllowedToolExecutes(t *testing.T) {
	t.Parallel()
	// Arrange: unrelated registry entries cannot expand the declared boundary.
	registry := NewTypedToolRegistry()
	calls := 0
	for _, name := range []string{"read", "delete_all"} {
		tool, err := NewTypedTool(name, func(context.Context, struct{}) (string, error) { calls++; return name, nil })
		require.NoError(t, err)
		require.NoError(t, RegisterTool(registry, tool))
	}
	execution := SimplePrompt("work")
	execution.Tools = registry.Definitions()[:1]
	execution.RequiredTools = []string{"read"}
	execution.ForcedTool = "read"
	execution, err := execution.WithToolScope(
		ToolScope{
			Allowed:  []ToolManifest{ToolManifestFromDefinition(execution.Tools[0])},
			Required: []ToolRequirement{{Name: "read"}},
		},
	)
	require.NoError(t, err)
	invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
		return NewResponse([]ContentPart{ToolCallPart{ID: "read-1", Name: "read", Args: "{}"}}), nil
	}}
	// Act.
	result, err := ExecuteWithTypedTools(context.Background(), invoker, execution, registry)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, 1, calls)
	require.Len(t, result.Messages, 3)
	require.NoError(t, ValidateExecutionToolScope(result))
}

func TestModelCannotExpandToolBoundaryByMutatingItsInput(t *testing.T) {
	t.Parallel()
	// Arrange.
	registry := NewTypedToolRegistry()
	calls := 0
	tool, err := NewTypedTool(
		"write",
		func(context.Context, struct{}) (string, error) { calls++; return "written", nil },
	)
	require.NoError(t, err)
	require.NoError(t, RegisterTool(registry, tool))
	execution := SimplePrompt("work")
	execution.Tools = []ToolDefinition{{Name: "read"}}
	invoker := &scriptedInvoker{generate: func(_ context.Context, input *PromptExecution) (*Response, error) {
		input.Tools = registry.Definitions()
		return NewResponse([]ContentPart{ToolCallPart{ID: "write-1", Name: "write", Args: "{}"}}), nil
	}}
	// Act.
	_, err = ExecuteWithTypedTools(context.Background(), invoker, execution, registry)
	// Assert.
	require.ErrorIs(t, err, ErrToolScope)
	require.Zero(t, calls)
	require.Equal(t, "read", execution.Tools[0].Name)
}

func TestDirectTypedInvocationKeepsSuccessfulResultOnCancellation(t *testing.T) {
	t.Parallel()
	// Arrange.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	tool, err := NewTypedTool(
		"write",
		func(context.Context, struct{}) (string, error) { calls++; cancel(); return "written", nil },
	)
	require.NoError(t, err)
	// Act.
	result, err := tool.Invoke(ctx, "{}")
	// Assert: the cancellation error must not erase a completed successful value.
	require.Equal(t, "written", result)
	require.ErrorIs(t, err, context.Canceled)
	require.Equal(t, 1, calls)
}

func TestCanceledRegistryInvocationReturnsNoSyntheticResult(t *testing.T) {
	t.Parallel()
	// Arrange.
	calls := 0
	registry := NewTypedToolRegistry()
	tool, err := NewTypedTool("write", func(context.Context, struct{}) (bool, error) { calls++; return true, nil })
	require.NoError(t, err)
	require.NoError(t, RegisterTool(registry, tool))
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	raw, err := registry.InvokeTool(ctx, "write", "{}")
	// Assert.
	require.ErrorIs(t, err, context.Canceled)
	require.Nil(t, raw)
	require.Zero(t, calls)
}
