package openai

import (
	"context"
	"fmt"
	"testing"

	"github.com/openai/openai-go/v3"

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
	params, _ := a.TranslateTyped(context.Background(), exec)
	fmt.Println(params.Messages[0].OfUser.Content.OfString.Value)
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
	assert.NotNil(t, params.Messages[0].OfUser)
	assert.Equal(t, "Hello", params.Messages[0].OfUser.Content.OfString.Value)
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
	require.Len(t, params.Messages, 2)
	assert.NotNil(t, params.Messages[0].OfSystem)
	assert.Equal(t, "You are a helper.", params.Messages[0].OfSystem.Content.OfString.Value)
	assert.NotNil(t, params.Messages[1].OfUser)
}

func TestTranslate_WithTools(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Call get_weather"}}},
		},
		Tools: []prompty.ToolDefinition{
			{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Tools, 1)
	assert.Equal(t, "get_weather", params.Tools[0].GetFunction().Name)
	assert.Equal(t, "Get weather", params.Tools[0].GetFunction().Description.Value)
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
	assert.NotNil(t, params.Messages[0].OfTool)
	assert.Equal(t, "call_1", params.Messages[0].OfTool.ToolCallID)
	assert.Equal(t, "Sunny", params.Messages[0].OfTool.Content.OfString.Value)
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
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	assert.True(t, params.Temperature.Valid())
	assert.InDelta(t, 0.5, params.Temperature.Value, 1e-9)
	assert.True(t, params.MaxTokens.Valid())
	assert.Equal(t, int64(100), params.MaxTokens.Value)
}

func TestTranslate_NilExecution(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Translate(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrNilExecution)
}

func TestTranslate_ImagePartData(t *testing.T) {
	t.Parallel()
	a := New()
	imgData := []byte{0xff, 0xd8, 0xff, 0xe0}
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "What is this?"},
				prompty.MediaPart{MediaType: "image", Data: imgData, MIMEType: "image/jpeg"},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	require.NotNil(t, params.Messages[0].OfUser)
	parts := params.Messages[0].OfUser.Content.OfArrayOfContentParts
	require.Len(t, parts, 2)
	assert.NotNil(t, parts[1].OfImageURL)
	assert.Contains(t, parts[1].OfImageURL.ImageURL.URL, "data:image/jpeg;base64,")
}

func TestTranslate_ImagePartURL(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image", URL: "https://example.com/img.png", MIMEType: "image/png"},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params.Messages[0].OfUser)
	parts := params.Messages[0].OfUser.Content.OfArrayOfContentParts
	require.Len(t, parts, 1)
	assert.NotNil(t, parts[0].OfImageURL)
	assert.Equal(t, "https://example.com/img.png", parts[0].OfImageURL.ImageURL.URL)
}

func TestTranslate_UserMessagePreservesTextImageOrder(t *testing.T) {
	t.Parallel()
	a := New()
	imgData := []byte{0xff, 0xd8, 0xff}
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "before "},
				prompty.MediaPart{MediaType: "image", Data: imgData, MIMEType: "image/jpeg"},
				prompty.TextPart{Text: " after"},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params.Messages[0].OfUser)
	parts := params.Messages[0].OfUser.Content.OfArrayOfContentParts
	require.Len(t, parts, 3)
	assert.NotNil(t, parts[0].OfText)
	assert.Equal(t, "before ", parts[0].OfText.Text)
	assert.NotNil(t, parts[1].OfImageURL)
	assert.NotNil(t, parts[2].OfText)
	assert.Equal(t, " after", parts[2].OfText.Text)
}

func TestTranslate_AssistantToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "I will call the tool."},
				prompty.ToolCallPart{ID: "call_1", Name: "get_weather", Args: `{"location":"NYC"}`},
			}},
		},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	require.NotNil(t, params.Messages[0].OfAssistant)
	require.Len(t, params.Messages[0].OfAssistant.ToolCalls, 1)
	tc := params.Messages[0].OfAssistant.ToolCalls[0]
	assert.Equal(t, "call_1", tc.OfFunction.ID)
	assert.Equal(t, "get_weather", tc.OfFunction.Function.Name)
	assert.JSONEq(t, `{"location":"NYC"}`, tc.OfFunction.Function.Arguments)
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
	completion := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{Content: "Hello back"},
		}},
	}
	parts, err := a.ParseResponse(context.Background(), completion)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello back", parts[0].(prompty.TextPart).Text)
}

func TestParseResponse_ToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	completion := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{
				Content: "",
				ToolCalls: []openai.ChatCompletionMessageToolCallUnion{{
					ID:   "call_1",
					Type: "function",
					Function: openai.ChatCompletionMessageFunctionToolCallFunction{
						Name:      "get_weather",
						Arguments: `{"location":"NYC"}`,
					},
				}},
			},
		}},
	}
	parts, err := a.ParseResponse(context.Background(), completion)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.JSONEq(t, `{"location":"NYC"}`, tc.Args)
}

func TestParseResponse_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(context.Background(), "not a completion")
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseResponse_EmptyChoices(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(context.Background(), &openai.ChatCompletion{Choices: nil})
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrEmptyResponse)
}

func TestParseResponse_EmptyContentAndNoToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	completion := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{Message: openai.ChatCompletionMessage{Content: ""}}},
	}
	_, err := a.ParseResponse(context.Background(), completion)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrEmptyResponse)
}

func TestParseStreamChunk_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	parts, err := a.ParseStreamChunk(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, parts)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseStreamChunk_TextDelta(t *testing.T) {
	t.Parallel()
	a := New()
	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{{
			Delta: openai.ChatCompletionChunkChoiceDelta{Content: "Hello "},
		}},
	}
	parts, err := a.ParseStreamChunk(context.Background(), chunk)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello ", parts[0].(prompty.TextPart).Text)
}

func TestParseStreamChunk_ToolCallChunk(t *testing.T) {
	t.Parallel()
	a := New()
	chunk := &openai.ChatCompletionChunk{
		Choices: []openai.ChatCompletionChunkChoice{{
			Delta: openai.ChatCompletionChunkChoiceDelta{
				ToolCalls: []openai.ChatCompletionChunkChoiceDeltaToolCall{{
					ID: "call_1",
					Function: openai.ChatCompletionChunkChoiceDeltaToolCallFunction{
						Name:      "get_weather",
						Arguments: `{"loc":"NYC"}`,
					},
				}},
			},
		}},
	}
	parts, err := a.ParseStreamChunk(context.Background(), chunk)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Equal(t, `{"loc":"NYC"}`, tc.ArgsChunk)
}

func TestTranslate_ResponseFormat(t *testing.T) {
	t.Parallel()
	a := New()
	schema := map[string]any{"type": "object", "properties": map[string]any{"answer": map[string]any{"type": "string"}}}
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Reply with JSON"}}},
		},
		ResponseFormat: &prompty.SchemaDefinition{Name: "reply_schema", Schema: schema},
	}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params.ResponseFormat.OfJSONSchema)
	assert.Equal(t, "reply_schema", params.ResponseFormat.OfJSONSchema.JSONSchema.Name)
	assert.Equal(t, schema, params.ResponseFormat.OfJSONSchema.JSONSchema.Schema)
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
	require.NotNil(t, params.Stop.OfStringArray)
	assert.Equal(t, []string{"STOP", "END"}, params.Stop.OfStringArray)
}

func TestTranslate_EmptyMessages(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{Messages: nil}
	params, err := a.TranslateTyped(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params)
	assert.Empty(t, params.Messages)
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
