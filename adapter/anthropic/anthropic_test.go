package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	// Ignore HTTP/2 client readLoop goroutine left by default client after FetchImage (e.g. TestTranslate_ImagePartURLFetchFails).
	goleak.VerifyTestMain(m, goleak.IgnoreTopFunction("internal/poll.runtime_pollWait"))
}

func ExampleAdapter_Translate() {
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hello"}}},
		},
	}
	params, _ := a.TranslateTyped(context.Background(), exec)
	fmt.Println(params.Messages[0].Content[0].OfText.Text)
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
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	require.Len(t, params.Messages[0].Content, 1)
	assert.NotNil(t, params.Messages[0].Content[0].OfText)
	assert.Equal(t, "Hello", params.Messages[0].Content[0].OfText.Text)
	assert.Equal(t, defaultMaxTokens, params.MaxTokens)
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
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.System, 1)
	assert.Equal(t, "You are a helper.", params.System[0].Text)
	require.Len(t, params.Messages, 1)
}

func TestTranslate_ToolResult(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{ToolCallID: "call_1", Name: "get_weather", Content: "Sunny", IsError: false},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	assert.Equal(t, anthropic.MessageParamRoleUser, params.Messages[0].Role)
	require.Len(t, params.Messages[0].Content, 1)
	assert.NotNil(t, params.Messages[0].Content[0].OfToolResult)
	assert.Equal(t, "call_1", params.Messages[0].Content[0].OfToolResult.ToolUseID)
}

func TestTranslate_ModelConfig(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelConfig: map[string]any{"max_tokens": int64(500), "temperature": 0.3},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	assert.Equal(t, int64(500), params.MaxTokens)
	assert.True(t, params.Temperature.Valid())
	assert.InDelta(t, 0.3, params.Temperature.Value, 1e-9)
}

func TestTranslate_NilExecution(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Translate(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrNilExecution)
}

func TestTranslate_WithTools(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Call get_weather"}}},
		},
		Tools: []prompty.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get weather for a location",
				Parameters: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"location": map[string]any{"type": "string"},
					},
					"required": []any{"location"},
				},
			},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Tools, 1)
	tool := params.Tools[0].OfTool
	require.NotNil(t, tool)
	assert.Equal(t, "get_weather", tool.Name)
	assert.Equal(t, "Get weather for a location", tool.Description.Value)
	assert.Len(t, tool.InputSchema.Required, 1)
	assert.Equal(t, "location", tool.InputSchema.Required[0])
}

func TestToolSchemaFromParameters_RequiredAsStringSlice(t *testing.T) {
	t.Parallel()
	// When building ToolDefinition in Go, required is often []string; it must not be dropped.
	params := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"q":     map[string]any{"type": "string"},
			"limit": map[string]any{"type": "integer"},
		},
		"required": []string{"q", "limit"},
	}
	schema := toolSchemaFromParameters(params)
	require.Len(t, schema.Required, 2)
	assert.Equal(t, "q", schema.Required[0])
	assert.Equal(t, "limit", schema.Required[1])
}

func TestTranslate_ImagePartData(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image", Data: []byte{0xff, 0xd8}, MIMEType: "image/jpeg"},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	require.Len(t, params.Messages[0].Content, 1)
	assert.NotNil(t, params.Messages[0].Content[0].OfImage)
}

func TestTranslate_ImagePartURLFetchFails(t *testing.T) {
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
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	assert.Contains(t, err.Error(), "fetch image URL")
}

func TestTranslate_ImagePartDataTakesPrecedenceOverURL(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image", Data: []byte{0xff, 0xd8}, URL: "https://example.com/img.png", MIMEType: "image/jpeg"},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	require.Len(t, params.Messages[0].Content, 1)
	assert.NotNil(t, params.Messages[0].Content[0].OfImage)
}

func TestTranslate_AssistantToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "Calling tool."},
				prompty.ToolCallPart{ID: "call_1", Name: "get_weather", Args: `{"location":"NYC"}`},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	require.Len(t, params.Messages[0].Content, 2)
	assert.NotNil(t, params.Messages[0].Content[0].OfText)
	assert.NotNil(t, params.Messages[0].Content[1].OfToolUse)
	assert.Equal(t, "call_1", params.Messages[0].Content[1].OfToolUse.ID)
	assert.Equal(t, "get_weather", params.Messages[0].Content[1].OfToolUse.Name)
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

func TestTranslate_StopSequences(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelConfig: map[string]any{"stop": []string{"STOP", "END"}},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	assert.Equal(t, []string{"STOP", "END"}, params.StopSequences)
}

func TestParseResponse_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	msg := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{{Type: "text", Text: "Hello back"}},
	}
	parts, err := a.ParseResponse(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello back", parts[0].(prompty.TextPart).Text)
}

func TestParseResponse_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(context.Background(), "not a message")
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseResponse_ToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	toolInput, _ := json.Marshal(map[string]any{"location": "NYC"})
	msg := &anthropic.Message{
		Content: []anthropic.ContentBlockUnion{{
			Type:  "tool_use",
			ID:    "call_1",
			Name:  "get_weather",
			Input: json.RawMessage(toolInput),
		}},
	}
	parts, err := a.ParseResponse(context.Background(), msg)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Contains(t, tc.Args, "NYC")
}

func TestParseResponse_EmptyContent(t *testing.T) {
	t.Parallel()
	a := New()
	msg := &anthropic.Message{Content: []anthropic.ContentBlockUnion{}}
	_, err := a.ParseResponse(context.Background(), msg)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrEmptyResponse)
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

func TestParseStreamChunk_NotImplemented(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseStreamChunk(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrStreamNotImplemented)
}
