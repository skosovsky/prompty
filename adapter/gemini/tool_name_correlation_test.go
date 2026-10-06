package gemini

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type nameCorrelationFixture struct {
	*Adapter

	calls     int
	duplicate bool
}

func (f *nameCorrelationFixture) Execute(context.Context, *Request) (*genai.GenerateContentResponse, error) {
	f.calls++
	parts := []*genai.Part{{FunctionCall: &genai.FunctionCall{Name: "lookup"}}}
	if f.duplicate {
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{Name: "lookup"}})
	}
	return &genai.GenerateContentResponse{Candidates: []*genai.Candidate{{
		Content: &genai.Content{Parts: parts}, FinishReason: genai.FinishReasonStop,
	}}}, nil
}

func TestCheckedIDlessToolCallsPreserveNameThroughContinuation(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"unique", "duplicate", "no requirement", "unknown knowledge"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: actual Gemini Translate/Parse and checked client, intercepted SDK execution.
			profile := geminiStateFixtureProfile()
			profile.Capabilities = map[string]prompty.CapabilitySupport{
				prompty.CapabilityTools:               prompty.CapabilitySupported,
				prompty.CapabilityToolNameCorrelation: prompty.CapabilitySupported,
			}
			if scenario == "unknown knowledge" {
				profile.Capabilities[prompty.CapabilityToolNameCorrelation] = prompty.CapabilityUnknown
			}
			fixture := &nameCorrelationFixture{
				Adapter:   New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile)),
				calls:     0,
				duplicate: scenario == "duplicate",
			}
			execution := prompty.SimplePrompt("fixture")
			execution.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
			if scenario != "no requirement" {
				execution.Requirements.Capabilities = []string{prompty.CapabilityToolNameCorrelation}
			}
			invoked := 0
			registry := prompty.NewTypedToolRegistry()
			tool, err := prompty.NewTypedTool(
				"lookup",
				func(_ context.Context, _ struct{}) (string, error) { invoked++; return "fixture", nil },
			)
			require.NoError(t, err)
			require.NoError(t, prompty.RegisterTool(registry, tool))
			// Act.
			updated, err := prompty.ExecuteWithTypedTools(
				context.Background(),
				adapter.NewClient(fixture),
				execution,
				registry,
			)
			// Assert: exact caller policy controls handler admission, no fabricated provider ID.
			if scenario != "unique" {
				require.Zero(t, invoked)
				if scenario == "unknown knowledge" {
					require.ErrorIs(t, err, adapter.ErrCapabilityRequirement)
					require.Zero(t, fixture.calls)
				} else {
					require.ErrorIs(t, err, prompty.ErrToolScope)
					require.Equal(t, 1, fixture.calls)
				}
				return
			}
			require.NoError(t, err)
			require.Equal(t, 1, invoked)
			require.Equal(t, 1, fixture.calls)
			request, err := fixture.Translate(updated)
			require.NoError(t, err)
			call := request.Contents[1].Parts[0].FunctionCall
			result := request.Contents[2].Parts[0].FunctionResponse
			require.Equal(t, "lookup", call.Name)
			require.Equal(t, "lookup", result.Name)
			require.Empty(t, call.ID)
			require.Empty(t, result.ID)
		})
	}
}
