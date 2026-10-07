package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIDlessToolCorrelationRequiresCheckedUniqueNames(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		mutate  func(*PromptExecution, *Response)
		allowed bool
	}{
		{"unique names", nil, true},
		{"valid explicit override", func(e *PromptExecution, r *Response) {
			e.Requirements.Override = &CapabilityProfile{Identity: r.Diagnostics.Profile, Provenance: "caller fixture",
				Capabilities: map[string]CapabilitySupport{CapabilityTools: CapabilitySupported, CapabilityToolNameCorrelation: CapabilitySupported}}
		}, true},
		{"foreign override", func(e *PromptExecution, _ *Response) { e.Requirements.Override = &CapabilityProfile{} }, false},
		{"unsupported override", func(e *PromptExecution, r *Response) {
			e.Requirements.Override = &CapabilityProfile{Identity: r.Diagnostics.Profile, Provenance: "caller fixture",
				Capabilities: map[string]CapabilitySupport{CapabilityTools: CapabilitySupported, CapabilityToolNameCorrelation: CapabilityUnsupported}}
		}, false},
		{"no requirement", func(e *PromptExecution, _ *Response) { e.Requirements.Capabilities = nil }, false},
		{"no provenance", func(_ *PromptExecution, r *Response) { r.Diagnostics.Provenance = "" }, false},
		{"no snapshot", func(_ *PromptExecution, r *Response) { r.Diagnostics.Snapshot = "" }, false},
		{"no destination", func(_ *PromptExecution, r *Response) { r.Diagnostics.Profile.Endpoint = "" }, false},
		{"unknown correlation", func(_ *PromptExecution, r *Response) { r.Diagnostics.Checks[1].Status = CapabilityUnknown }, false},
		{"unsupported correlation", func(_ *PromptExecution, r *Response) { r.Diagnostics.Checks[1].Status = CapabilityUnsupported }, false},
		{"missing tools check", func(_ *PromptExecution, r *Response) { r.Diagnostics.Checks = r.Diagnostics.Checks[1:] }, false},
		{"duplicate check", func(_ *PromptExecution, r *Response) {
			r.Diagnostics.Checks = append(r.Diagnostics.Checks, r.Diagnostics.Checks[1])
		}, false},
		{"foreign model", func(e *PromptExecution, _ *Response) { e.ModelOptions = &ModelOptions{Model: "foreign"} }, false},
		{"repeated IDless name", func(_ *PromptExecution, r *Response) { r.Content[1] = ToolCallPart{Name: "lookup", Args: "{}"} }, false},
		{"mixed duplicate name", func(_ *PromptExecution, r *Response) {
			r.Content[1] = ToolCallPart{ID: "native", Name: "lookup", Args: "{}"}
		}, false},
		{"IDless delta", func(_ *PromptExecution, r *Response) { r.Content[1] = ToolCallPart{Name: "weather", ArgsChunk: "{}"} }, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: caller invoker supplies checked profile evidence, not fabricated IDs.
			invoked := 0
			registry := NewTypedToolRegistry()
			for _, name := range []string{"lookup", "weather"} {
				tool, err := NewTypedTool(
					name,
					func(_ context.Context, _ struct{}) (string, error) { invoked++; return "fixture", nil },
				)
				require.NoError(t, err)
				require.NoError(t, RegisterTool(registry, tool))
			}
			execution := SimplePrompt("fixture")
			execution.Tools = []ToolDefinition{{Name: "lookup"}, {Name: "weather"}}
			execution.Requirements.Capabilities = []string{CapabilityToolNameCorrelation}
			response := NewResponse(
				[]ContentPart{ToolCallPart{Name: "lookup", Args: "{}"}, ToolCallPart{Name: "weather", Args: "{}"}},
			)
			response.Diagnostics = ExecutionDiagnostics{
				Profile:  ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "known"},
				Snapshot: "host-checked-fixture", Provenance: "local checked-name fixture",
				Checks: []RequirementCheck{{Requirement: CapabilityTools, Status: CapabilitySupported},
					{Requirement: CapabilityToolNameCorrelation, Status: CapabilitySupported}},
			}
			if test.mutate != nil {
				test.mutate(execution, response)
			}
			invoker := &scriptedInvoker{
				generate: func(context.Context, *PromptExecution) (*Response, error) { return response, nil },
			}
			// Act.
			updated, err := ExecuteWithTypedTools(context.Background(), invoker, execution, registry)
			// Assert: the whole batch is admitted/rejected before any local side effect.
			if !test.allowed {
				require.ErrorIs(t, err, ErrToolScope)
				require.Zero(t, invoked)
				return
			}
			require.NoError(t, err)
			require.Equal(t, 2, invoked)
			for i, name := range []string{"lookup", "weather"} {
				call := updated.Messages[1].Content[i].(ToolCallPart)
				result := updated.Messages[i+2].Content[0].(ToolResultPart)
				require.Empty(t, call.ID)
				require.Empty(t, result.ToolCallID)
				require.Equal(t, name, result.Name)
			}
		})
	}
}
