package openai

import (
	"context"
	"encoding/json"
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
	params, _ := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
				prompty.ToolResultPart{ToolCallID: "call_1", Name: "get_weather", Content: []prompty.ContentPart{prompty.TextPart{Text: "Sunny"}}, IsError: false},
			}},
		},
	}
	params, err := a.Translate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 1)
	assert.NotNil(t, params.Messages[0].OfTool)
	assert.Equal(t, "call_1", params.Messages[0].OfTool.ToolCallID)
	assert.Equal(t, "Sunny", params.Messages[0].OfTool.Content.OfString.Value)
}

func TestTranslate_BatchedToolResults(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{ToolCallID: "call_1", Name: "get_weather", Content: []prompty.ContentPart{prompty.TextPart{Text: "Sunny"}}, IsError: false},
				prompty.ToolResultPart{ToolCallID: "call_2", Name: "get_time", Content: []prompty.ContentPart{prompty.TextPart{Text: "12:00"}}, IsError: true},
			}},
		},
	}
	params, err := a.Translate(context.Background(), exec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 2)
	require.NotNil(t, params.Messages[0].OfTool)
	require.NotNil(t, params.Messages[1].OfTool)
	assert.Equal(t, "call_1", params.Messages[0].OfTool.ToolCallID)
	assert.Equal(t, "Sunny", params.Messages[0].OfTool.Content.OfString.Value)
	assert.Equal(t, "call_2", params.Messages[1].OfTool.ToolCallID)
	assert.Equal(t, "12:00", params.Messages[1].OfTool.Content.OfString.Value)
}

func TestTranslate_ToolResult_WithMediaPart_FailFast(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{
					ToolCallID: "call_1",
					Name:       "screenshot",
					Content: []prompty.ContentPart{
						prompty.TextPart{Text: "Here is the chart."},
						prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}},
					},
					IsError: false,
				},
			}},
		},
	}
	_, err := a.Translate(context.Background(), exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
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
	params, err := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
	params, err := a.Translate(context.Background(), exec)
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
	_, err := a.Translate(context.Background(), exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrUnsupportedRole)
	assert.Contains(t, err.Error(), "unknown_role")
}

func TestParseResponse_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	completion := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{
			Message: openai.ChatCompletionMessage{Content: "Hello back"},
		}},
	}
	resp, err := a.ParseResponse(context.Background(), completion)
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	assert.Equal(t, "Hello back", resp.Content[0].(prompty.TextPart).Text)
}

func TestParseResponse_FinishReason(t *testing.T) {
	t.Parallel()
	a := New()
	completion := &openai.ChatCompletion{
		Choices: []openai.ChatCompletionChoice{{
			Message:      openai.ChatCompletionMessage{Content: "done"},
			FinishReason: "stop",
		}},
	}
	resp, err := a.ParseResponse(context.Background(), completion)
	require.NoError(t, err)
	assert.Equal(t, "stop", resp.FinishReason)
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
	resp, err := a.ParseResponse(context.Background(), completion)
	require.NoError(t, err)
	require.Len(t, resp.Content, 1)
	tc := resp.Content[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.JSONEq(t, `{"location":"NYC"}`, tc.Args)
}

func TestParseResponse_NilCompletion(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(context.Background(), nil)
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

func TestExecuteStream_NoClient(t *testing.T) {
	t.Parallel()
	a := New() // no WithClient
	req, err := a.Translate(context.Background(), &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "hi"}}},
		},
	})
	require.NoError(t, err)
	seq := a.ExecuteStream(context.Background(), req)
	var gotErr error
	for _, e := range seq {
		gotErr = e
		break
	}
	require.Error(t, gotErr)
	assert.ErrorIs(t, gotErr, adapter.ErrNoClient)
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
	params, err := a.Translate(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params.ResponseFormat.OfJSONSchema)
	js := params.ResponseFormat.OfJSONSchema.JSONSchema
	assert.Equal(t, "reply_schema", js.Name)
	assert.True(t, js.Strict.Value, "OpenAI ResponseFormat must use strict mode")
	// strict mode requires additionalProperties: false for type object; normalization adds it
	gotSchema, ok := js.Schema.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "object", gotSchema["type"])
	assert.Equal(t, false, gotSchema["additionalProperties"], "strict mode requires additionalProperties: false")
	// DoD: serialized request JSON must contain "strict":true
	raw, err := json.Marshal(params)
	require.NoError(t, err)
	var m map[string]any
	require.NoError(t, json.Unmarshal(raw, &m))
	rf, ok := m["response_format"].(map[string]any)
	require.True(t, ok, "response_format must be present in serialized JSON")
	jsonSchema, ok := rf["json_schema"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, true, jsonSchema["strict"], "serialized JSON must contain strict: true for OpenAI strict mode")
}

func TestTranslate_ResponseFormat_RecursivelyNormalizesStrictSchema(t *testing.T) {
	t.Parallel()
	a := New()
	schema := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"nickname": map[string]any{"type": "string"},
			"profile": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bio": map[string]any{"type": "string"},
				},
			},
		},
		"required": []string{"profile"},
	}
	original := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"nickname": map[string]any{"type": "string"},
			"profile": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"bio": map[string]any{"type": "string"},
				},
			},
		},
		"required": []string{"profile"},
	}

	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Reply with JSON"}}},
		},
		ResponseFormat: &prompty.SchemaDefinition{Name: "reply_schema", Schema: schema},
	}
	params, err := a.Translate(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params.ResponseFormat.OfJSONSchema)

	gotSchema, ok := params.ResponseFormat.OfJSONSchema.JSONSchema.Schema.(map[string]any)
	require.True(t, ok)
	assert.Equal(t, false, gotSchema["additionalProperties"])
	assert.ElementsMatch(t, []string{"profile", "nickname"}, gotSchema["required"])
	nickname := gotSchema["properties"].(map[string]any)["nickname"].(map[string]any)
	assert.Equal(t, []any{"string", "null"}, nickname["type"])
	profile := gotSchema["properties"].(map[string]any)["profile"].(map[string]any)
	assert.Equal(t, false, profile["additionalProperties"])
	assert.ElementsMatch(t, []string{"bio"}, profile["required"])
	bio := profile["properties"].(map[string]any)["bio"].(map[string]any)
	assert.Equal(t, []any{"string", "null"}, bio["type"])
	assert.Equal(t, original, schema, "strict normalization must not mutate caller-owned schema")
}

func TestTranslate_WithRetryBatchedToolResultsSurviveOpenAITranslation(t *testing.T) {
	t.Parallel()
	a := New()

	callNum := 0
	var translatedExec *prompty.PromptExecution
	result, err := prompty.WithRetry(context.Background(), prompty.NewExecution([]prompty.ChatMessage{
		prompty.NewUserMessage("hi"),
	}), 1, func(_ context.Context, exec *prompty.PromptExecution) (string, error) {
		callNum++
		if callNum == 1 {
			msg := prompty.ChatMessage{
				Role: prompty.RoleAssistant,
				Content: []prompty.ContentPart{
					prompty.ToolCallPart{ID: "tool-1", Name: "lookup", Args: `{}`},
					prompty.ToolCallPart{ID: "tool-2", Name: "weather", Args: `{}`},
				},
			}
			return "", &prompty.ToolCallError{
				RawAssistantMessage: &msg,
				ToolResults: []prompty.ContentPart{
					prompty.ToolResultPart{ToolCallID: "tool-1", Name: "lookup", Content: []prompty.ContentPart{prompty.TextPart{Text: "lookup invalid"}}, IsError: true},
					prompty.ToolResultPart{ToolCallID: "tool-2", Name: "weather", Content: []prompty.ContentPart{prompty.TextPart{Text: "weather invalid"}}, IsError: true},
				},
				Err: fmt.Errorf("invalid tool batch"),
			}
		}
		translatedExec = exec
		return "ok", nil
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", result)
	require.NotNil(t, translatedExec)

	params, err := a.Translate(context.Background(), translatedExec)
	require.NoError(t, err)
	require.Len(t, params.Messages, 4)
	require.NotNil(t, params.Messages[2].OfTool)
	require.NotNil(t, params.Messages[3].OfTool)
	assert.Equal(t, "tool-1", params.Messages[2].OfTool.ToolCallID)
	assert.Equal(t, "lookup invalid", params.Messages[2].OfTool.Content.OfString.Value)
	assert.Equal(t, "tool-2", params.Messages[3].OfTool.ToolCallID)
	assert.Equal(t, "weather invalid", params.Messages[3].OfTool.Content.OfString.Value)
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
	params, err := a.Translate(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, params.Stop.OfStringArray)
	assert.Equal(t, []string{"STOP", "END"}, params.Stop.OfStringArray)
}

func TestTranslate_EmptyMessages(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{Messages: nil}
	params, err := a.Translate(context.Background(), exec)
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
	_, err := a.Translate(context.Background(), exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrMalformedArgs)
}
