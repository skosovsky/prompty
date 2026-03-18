package contextx

import (
	"context"
	"errors"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type mockCounter struct {
	count func(string) (int, error)
}

func (m *mockCounter) Count(text string) (int, error) {
	if m.count != nil {
		return m.count(text)
	}
	return len(text) / 4, nil
}

func TestWithTokenBudget_TrimsWhenOverLimit(t *testing.T) {
	t.Parallel()
	callCount := 0
	var receivedExec *prompty.PromptExecution
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			callCount++
			receivedExec = exec
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) { return len(s) / 2, nil }}
	mw := WithTokenBudget(10, counter)
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "system"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "user1"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "assistant1"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "user2"}}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	assert.Equal(t, 1, callCount)
	// Original exec unchanged (deep copy passed to next)
	require.Len(t, exec.Messages, 4)
	// Downstream receives trimmed copy: system (3) + user2 (2) = 5 tokens, under 10
	require.Len(t, receivedExec.Messages, 2)
	assert.Equal(t, prompty.RoleSystem, receivedExec.Messages[0].Role)
	assert.Equal(t, prompty.RoleUser, receivedExec.Messages[1].Role)
	assert.Equal(t, "user2", receivedExec.Messages[1].Content[0].(prompty.TextPart).Text)
}

func TestWithTokenBudget_KeepsSystemMessage(t *testing.T) {
	t.Parallel()
	var receivedExec *prompty.PromptExecution
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			receivedExec = exec
			require.GreaterOrEqual(t, len(exec.Messages), 1)
			assert.Equal(t, prompty.RoleSystem, exec.Messages[0].Role)
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) { return len(s), nil }}
	mw := WithTokenBudget(5, counter)
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "sys"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "uuuuuuuu"}}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2, "original must be unmodified")
	require.Len(t, receivedExec.Messages, 1, "downstream receives trimmed copy")
	assert.Equal(t, prompty.RoleSystem, receivedExec.Messages[0].Role)
}

func TestWithTokenBudget_NoTrimWhenUnderLimit(t *testing.T) {
	t.Parallel()
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			require.Len(t, exec.Messages, 2)
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(_ string) (int, error) { return 1, nil }}
	mw := WithTokenBudget(100, counter)
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "x"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "y"}}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
}

func TestWithTokenBudget_KeepsToolWithAssistant(t *testing.T) {
	t.Parallel()
	var receivedExec *prompty.PromptExecution
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			receivedExec = exec
			// Must have User, Assistant(tool_call), Tool - no orphan tool
			roles := make([]string, len(exec.Messages))
			for i, m := range exec.Messages {
				roles[i] = string(m.Role)
			}
			assert.NotContains(t, roles, "tool", "tool without preceding assistant tool_call causes 400")
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) { return len(s) / 2, nil }}
	mw := WithTokenBudget(4, counter) // total ~5; budget 4 forces trim of first turn
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "sys"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "u1"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "c1", Name: "f", Args: "{}"},
			}},
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{ToolCallID: "c1", Name: "f", Content: []prompty.ContentPart{prompty.TextPart{Text: "r1"}}},
			}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "u2"}}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 5, "original must be unmodified")
	// Downstream receives trimmed: Turn 1 (user1+assistant+tool) removed; keep system + user2
	require.Len(t, receivedExec.Messages, 2)
	assert.Equal(t, prompty.RoleSystem, receivedExec.Messages[0].Role)
	assert.Equal(t, prompty.RoleUser, receivedExec.Messages[1].Role)
}

func TestWithTokenBudget_PropagatesCounterError(t *testing.T) {
	t.Parallel()
	countErr := errors.New("counter error")
	counter := &mockCounter{count: func(string) (int, error) { return 0, countErr }}
	mw := WithTokenBudget(10, counter)
	inv := mw(&invokerFunc{generate: func(context.Context, *prompty.PromptExecution) (*prompty.Response, error) { return nil, nil }})

	_, err := inv.Generate(context.Background(), &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "x"}}}},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, countErr)
}

func TestWithTokenBudget_ToolHeavyTurn_CountsArgsAndResult(t *testing.T) {
	t.Parallel()
	var receivedExec *prompty.PromptExecution
	// Counter: 1 token per 2 chars. Large Args and ToolResult should contribute to budget.
	// sys=2, u1=2, asst(tool_args)=5, tool(result)=5, u2=2 -> total 16. Budget 6 -> trim first turn.
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			receivedExec = exec
			require.Len(t, exec.Messages, 2, "system + user2 only; tool-heavy turn removed")
			assert.Equal(t, prompty.RoleSystem, exec.Messages[0].Role)
			assert.Equal(t, prompty.RoleUser, exec.Messages[1].Role)
			// No orphan tool message
			for _, m := range exec.Messages {
				assert.NotEqual(t, prompty.RoleTool, m.Role)
			}
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) { return len(s) / 2, nil }}
	mw := WithTokenBudget(6, counter)
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "sys"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "u1"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "c1", Name: "f", Args: `{"x":"long_args"}`},
			}},
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{ToolCallID: "c1", Name: "f", Content: []prompty.ContentPart{prompty.TextPart{Text: "long_result"}}},
			}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "u2"}}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 5, "original unchanged")
	require.Len(t, receivedExec.Messages, 2)
}

func TestWithTokenBudget_CountsReasoningPart(t *testing.T) {
	t.Parallel()
	var receivedExec *prompty.PromptExecution
	// ReasoningPart is counted. sys=1, u=1, asst(reasoning "chain"=5, text "ok"=2)=7 -> total 9.
	// Budget 4. Turn-based trim removes entire turn (user+assistant) -> system only.
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			receivedExec = exec
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) { return len(s), nil }}
	mw := WithTokenBudget(4, counter)
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "s"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "u"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.ReasoningPart{Text: "chain"},
				prompty.TextPart{Text: "ok"},
			}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 3, "original unchanged")
	require.Len(t, receivedExec.Messages, 1, "turn removed; system only fits budget 4")
	assert.Equal(t, prompty.RoleSystem, receivedExec.Messages[0].Role)
}

func TestWithTokenBudget_MediaPartCounted(t *testing.T) {
	t.Parallel()
	// MediaPart adds penalty (default 256). sys=1, u=1, img=256 -> 258. Budget 10 -> trim to sys only.
	var receivedExec *prompty.PromptExecution
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			receivedExec = exec
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) { return len(s), nil }}
	mw := WithTokenBudget(10, counter) // default 256 per MediaPart
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "s"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "u"},
				prompty.MediaPart{MediaType: "image", MIMEType: "image/png"},
			}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2, "original unchanged")
	require.Len(t, receivedExec.Messages, 1, "downstream trimmed: media-heavy turn exceeds budget")
	assert.Equal(t, prompty.RoleSystem, receivedExec.Messages[0].Role)
}

func TestWithTokenBudget_BudgetMetWhenTurnIncludedBeforeBreak(t *testing.T) {
	t.Parallel()
	// Off-by-one fix: when total-turnTokens <= maxTokens, we must set trimEnd=end before break.
	// Turn1=40, Turn2=20, total=100, budget=50. After removing Turn1: 60>50. Remove Turn2: 40<=50, done.
	var receivedExec *prompty.PromptExecution
	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			receivedExec = exec
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(s string) (int, error) {
		switch s {
		case "turn1":
			return 40, nil
		case "turn2":
			return 20, nil
		case "sys":
			return 10, nil
		default:
			return 1, nil
		}
	}}
	mw := WithTokenBudget(50, counter)
	inv := mw(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "sys"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "turn1"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "turn1"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "turn2"}}},
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.TextPart{Text: "turn2"}}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	// Must fit in 50: sys(10) + turn2(20) = 30. Both turns removed.
	require.Len(t, receivedExec.Messages, 3, "sys + turn2 only; budget 50 must be met")
	total := 0
	for _, m := range receivedExec.Messages {
		for _, p := range m.Content {
			if t, ok := p.(prompty.TextPart); ok {
				n, _ := counter.Count(t.Text)
				total += n
			}
		}
	}
	require.LessOrEqual(t, total, 50, "trimmed total must fit budget")
}

func TestWithTokenBudget_DownstreamMutationDoesNotLeakToOriginal(t *testing.T) {
	t.Parallel()

	base := &invokerFunc{
		generate: func(_ context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
			exec.Metadata.Extras["trace"].(map[string]any)["env"] = "prod"
			exec.Messages[0].Metadata["trace"].(map[string]any)["id"] = "mutated"
			exec.Messages[0].Content[0].(*prompty.TextPart).Text = "mutated"
			return &prompty.Response{}, nil
		},
	}
	counter := &mockCounter{count: func(_ string) (int, error) { return 1, nil }}
	inv := WithTokenBudget(100, counter)(base)

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{
				Role: prompty.RoleSystem,
				Content: []prompty.ContentPart{
					&prompty.TextPart{Text: "sys"},
				},
				Metadata: map[string]any{"trace": map[string]any{"id": "original"}},
			},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "hi"}}},
		},
		Metadata: prompty.PromptMetadata{
			Extras: map[string]any{"trace": map[string]any{"env": "dev"}},
		},
	}

	_, err := inv.Generate(context.Background(), exec)
	require.NoError(t, err)
	assert.Equal(t, "dev", exec.Metadata.Extras["trace"].(map[string]any)["env"])
	assert.Equal(t, "original", exec.Messages[0].Metadata["trace"].(map[string]any)["id"])
	assert.Equal(t, "sys", exec.Messages[0].Content[0].(*prompty.TextPart).Text)
}

type invokerFunc struct {
	generate func(context.Context, *prompty.PromptExecution) (*prompty.Response, error)
}

func (i *invokerFunc) Generate(ctx context.Context, exec *prompty.PromptExecution) (*prompty.Response, error) {
	return i.generate(ctx, exec)
}

func (i *invokerFunc) GenerateStream(ctx context.Context, exec *prompty.PromptExecution) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		resp, err := i.generate(ctx, exec)
		if err != nil {
			yield(nil, err)
			return
		}
		if resp != nil {
			yield(&prompty.ResponseChunk{Content: resp.Content, IsFinished: true}, nil)
		} else {
			yield(&prompty.ResponseChunk{IsFinished: true}, nil)
		}
	}
}
