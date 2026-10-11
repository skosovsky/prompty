package gemini

import (
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestThinkingDynamicBudgetAndLevelAreSeparateModes(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{
		ProviderSettings: prompty.JSONDocument(`{"thinking":true,"thinking_budget":-1}`),
	}
	// Act.
	dynamic, err := New().Translate(exec)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, int32(-1), *dynamic.Config.ThinkingConfig.ThinkingBudget)
	require.True(t, dynamic.Config.ThinkingConfig.IncludeThoughts)
	// Arrange/Act.
	exec.ModelOptions.ProviderSettings = prompty.JSONDocument(`{"thinking_level":"high","thinking":true}`)
	level, err := New().Translate(exec)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, genai.ThinkingLevelHigh, level.Config.ThinkingConfig.ThinkingLevel)
	require.Nil(t, level.Config.ThinkingConfig.ThinkingBudget)
	// Arrange/Act.
	exec.ModelOptions.ProviderSettings = prompty.JSONDocument(`{"thinking_level":"high","thinking_budget":-1}`)
	_, err = New().Translate(exec)
	// Assert.
	require.ErrorIs(t, err, adapter.ErrInvalidProviderSettings)
}
