package prompty

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStrictTextFromParts_PlainText(t *testing.T) {
	t.Parallel()
	got, err := StrictTextFromParts([]ContentPart{TextPart{Text: "hello"}})
	require.NoError(t, err)
	assert.Equal(t, "hello", got)
}

func TestStrictTextFromParts_EmptyFails(t *testing.T) {
	t.Parallel()
	_, err := StrictTextFromParts(nil)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEmptyTextResponse)
}

func TestStrictTextFromParts_OnlyEmptyTextPartsFails(t *testing.T) {
	t.Parallel()
	_, err := StrictTextFromParts([]ContentPart{
		TextPart{Text: ""},
		TextPart{Text: ""},
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrEmptyTextResponse)
}

func TestStrictTextFromParts_ToolCallFails(t *testing.T) {
	t.Parallel()
	_, err := StrictTextFromParts([]ContentPart{
		TextPart{Text: "x"},
		ToolCallPart{Name: "fn", Args: `{}`},
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNonTextResponse)
}

func TestRenderPlan_ExecuteAsText(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	})
	require.NoError(t, err)

	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{TextPart{Text: "ok"}}), nil
		},
	}
	plan := newRenderPlanFromMap(tpl, map[string]any{"q": "hi"})
	text, err := plan.ExecuteAsText(context.Background(), invoker)
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
}

func TestRenderPlan_RenderText_JoinsRenderedMessagesWithoutInvoker(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("system")},
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	})
	require.NoError(t, err)
	plan := newRenderPlanFromMap(tpl, map[string]any{"q": "hello"})

	text, err := plan.RenderText(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "system\n\nhello", text)
}

func TestPromptExecution_TextStrictRejectsNonTextPart(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{{
		Role: RoleAssistant,
		Content: []ContentPart{
			TextPart{Text: "x"},
			ToolCallPart{Name: "lookup", Args: "{}"},
		},
	}})

	_, err := exec.Text(true)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNonTextResponse)
}

func TestPromptExecution_TextStrictRejectsAmbiguousMessages(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		NewSystemMessage("system"),
		NewUserMessage("hello"),
	})

	_, err := exec.Text(true)

	require.Error(t, err)
	assert.ErrorIs(t, err, ErrAmbiguousTextExecution)
}

func TestRenderPlan_ExecuteAsText_NonTextResponse(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hi")},
	})
	require.NoError(t, err)

	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) {
			return NewResponse([]ContentPart{ToolCallPart{Name: "x", Args: "{}"}}), nil
		},
	}
	_, err = NewRenderPlan(tpl).ExecuteAsText(context.Background(), invoker)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNonTextResponse)
}

func TestJoinAdapterTextParts_SkipsMediaAndToolCalls(t *testing.T) {
	t.Parallel()
	got, err := JoinAdapterTextParts([]ContentPart{
		TextPart{Text: "a"},
		MediaPart{MediaType: "image", URL: "https://x"},
		ToolCallPart{Name: "fn", Args: `{}`},
		TextPart{Text: "b"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ab", got)
}

func TestResponse_TextRemoved_UseStrictText(t *testing.T) {
	t.Parallel()
	resp := NewResponse([]ContentPart{
		TextPart{Text: "ok"},
		ToolCallPart{Name: "fn", Args: `{}`},
	})
	_, err := resp.StrictText()
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrNonTextResponse)
}

func TestExecuteAsText_PreflightRejectsRequiredTools(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.RequiredTools = []string{"lookup"}
	_, err := ExecuteAsText(context.Background(), &scriptedInvoker{}, exec)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNonTextResponse)
}

func TestExecuteAsText_PreflightRejectsForcedTool(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.ForcedTool = "lookup"
	_, err := ExecuteAsText(context.Background(), &scriptedInvoker{}, exec)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNonTextResponse)
}

func TestExecuteAsText_PreflightBeforeInvoke(t *testing.T) {
	t.Parallel()
	calls := 0
	invoker := &scriptedInvoker{
		generate: func(context.Context, *PromptExecution) (*Response, error) {
			calls++
			return NewResponse([]ContentPart{TextPart{Text: "ok"}}), nil
		},
	}
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.Tools = []ToolDefinition{{Name: "lookup"}}
	_, err := ExecuteAsText(context.Background(), invoker, exec)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrNonTextResponse)
	assert.Zero(t, calls, "invoker must not run when text preflight fails")
}
