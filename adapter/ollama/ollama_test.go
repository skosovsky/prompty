package ollama

import (
	"context"
	"fmt"
	"testing"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func ExampleAdapter_Translate() {
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hello"}}},
		},
	}
	req, _ := a.TranslateTyped(context.Background(), exec)
	fmt.Println(req.Messages[0].Content)
	// Output: Hello
}

func TestTranslate_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hello"}}},
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Equal(t, "Hello", req.Messages[0].Content)
}

func TestTranslate_SystemMessage(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "You are a helper."}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Equal(t, "You are a helper.", req.Messages[0].Content)
	assert.Equal(t, "user", req.Messages[1].Role)
	assert.Equal(t, "Hi", req.Messages[1].Content)
}

func TestTranslate_WithTools(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Call get_weather"}}},
		},
		Tools: []prompty.ToolDefinition{
			{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object", "properties": map[string]any{}}},
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, req.Tools, 1)
	assert.Equal(t, "function", req.Tools[0].Type)
	assert.Equal(t, "get_weather", req.Tools[0].Function.Name)
	assert.Equal(t, "Get weather", req.Tools[0].Function.Description)
}

func TestTranslate_ToolResult(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{ToolCallID: "call_1", Name: "get_weather", Content: []prompty.ContentPart{prompty.TextPart{Text: "Sunny"}}, IsError: false},
			}},
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "tool", req.Messages[0].Role)
	assert.Equal(t, "call_1", req.Messages[0].ToolCallID)
	assert.Equal(t, "Sunny", req.Messages[0].Content)
}

func TestTranslate_ModelConfig(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelConfig: map[string]any{
			"temperature": 0.5,
			"max_tokens":  int64(100),
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, req.Options)
	assert.InDelta(t, 0.5, req.Options["temperature"], 1e-9)
	assert.Equal(t, int64(100), req.Options["num_predict"])
}

func TestTranslate_ImagePartData(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "What is this?"},
				prompty.MediaPart{MediaType: "image", Data: []byte{0xff, 0xd8}, MIMEType: "image/jpeg"},
			}},
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Len(t, req.Messages[0].Images, 1)
	assert.Equal(t, api.ImageData([]byte{0xff, 0xd8}), req.Messages[0].Images[0])
}

func TestTranslate_MediaPartURLWithoutData_ReturnsErrMediaNotResolved(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image", URL: "https://example.com/img.png"},
			}},
		},
	}
	_, err := a.TranslateTyped(context.Background(), exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrMediaNotResolved)
}

func TestTranslate_ImagePartEmptyRejected(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image"},
			}},
		},
	}
	_, err := a.TranslateTyped(context.Background(), exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	assert.Contains(t, err.Error(), "neither Data nor URL")
}

func TestTranslate_NilExecution(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Translate(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrNilExecution)
}

func TestTranslate_AssistantToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				// Text part before tool calls -- Index should be 0 for first ToolCallPart.
				prompty.TextPart{Text: "Calling tools."},
				prompty.ToolCallPart{ID: "call_1", Name: "get_weather", Args: `{"location":"NYC"}`},
				prompty.ToolCallPart{ID: "call_2", Name: "get_time", Args: `{"tz":"UTC"}`},
			}},
		},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "assistant", req.Messages[0].Role)
	require.Len(t, req.Messages[0].ToolCalls, 2)
	assert.Equal(t, "call_1", req.Messages[0].ToolCalls[0].ID)
	assert.Equal(t, "get_weather", req.Messages[0].ToolCalls[0].Function.Name)
	assert.Equal(t, 0, req.Messages[0].ToolCalls[0].Function.Index)
	assert.Equal(t, "call_2", req.Messages[0].ToolCalls[1].ID)
	assert.Equal(t, "get_time", req.Messages[0].ToolCalls[1].Function.Name)
	assert.Equal(t, 1, req.Messages[0].ToolCalls[1].Function.Index)
}

func TestTranslate_UnsupportedRole(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: "unknown_role", Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	_, err := a.TranslateTyped(context.Background(), exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrUnsupportedRole)
}

func TestParseResponse_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &api.ChatResponse{
		Message: api.Message{Role: "assistant", Content: "Hello back"},
	}
	parts, err := a.ParseResponse(context.Background(), resp)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello back", parts[0].(prompty.TextPart).Text)
}

func TestParseResponse_ToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	args := api.NewToolCallFunctionArguments()
	args.Set("location", "NYC")
	resp := &api.ChatResponse{
		Message: api.Message{
			Role:    "assistant",
			Content: "",
			ToolCalls: []api.ToolCall{{
				ID: "call_1",
				Function: api.ToolCallFunction{
					Index:     0,
					Name:      "get_weather",
					Arguments: args,
				},
			}},
		},
	}
	parts, err := a.ParseResponse(context.Background(), resp)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Contains(t, tc.Args, "NYC")
}

func TestParseResponse_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(context.Background(), "not a response")
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseResponse_EmptyContentAndNoToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &api.ChatResponse{
		Message: api.Message{Role: "assistant", Content: ""},
	}
	_, err := a.ParseResponse(context.Background(), resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrEmptyResponse)
}

func TestTranslate_StopSequences(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelConfig: map[string]any{"stop": []string{"STOP", "END"}},
	}
	req, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, req.Options)
	assert.Equal(t, []string{"STOP", "END"}, req.Options["stop"])
}

func TestTranslate_InvalidToolCallArgs(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "call_1", Name: "fn", Args: "not valid json"},
			}},
		},
	}
	_, err := a.TranslateTyped(context.Background(), exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrMalformedArgs)
}

func TestTranslate_StructuredOutputNotSupported(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ResponseFormat: &prompty.SchemaDefinition{Schema: map[string]any{"type": "object"}},
	}
	_, err := a.TranslateTyped(context.Background(), exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrStructuredOutputNotSupported)
}

func TestParseStreamChunk_Text(t *testing.T) {
	t.Parallel()
	a := New()
	chunk := &api.ChatResponse{
		Message: api.Message{Content: "Hello "},
		Done:    false,
	}
	parts, err := a.ParseStreamChunk(context.Background(), chunk)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello ", parts[0].(prompty.TextPart).Text)
}

func TestParseStreamChunk_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseStreamChunk(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}
