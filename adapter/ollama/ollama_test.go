package ollama

import (
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
			{
				Role:    prompty.RoleUser,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "Hello"}},
			},
		},
	}
	req, _ := a.Translate(exec)
	fmt.Println(req.Messages[0].Content)
	// Output: Hello
}

func TestTranslate_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{
				Role:    prompty.RoleUser,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "Hello"}},
			},
		},
	}
	before := exec.Clone()
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Equal(t, "Hello", req.Messages[0].Content)
	assert.Equal(t, before.Messages, exec.Messages)
}

func TestTranslate_DoesNotMutatePromptExecution(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "A"}}},
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "B"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	before := exec.Clone()
	_, err := a.Translate(exec)
	require.NoError(t, err)
	assert.Equal(t, before.Messages, exec.Messages)
}

func TestTranslate_SystemMessage(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{
				Role:    prompty.RoleSystem,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "You are a helper."}},
			},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	req, err := a.Translate(exec)
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
			{
				Role:    prompty.RoleUser,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "Call get_weather"}},
			},
		},
		Tools: []prompty.ToolDefinition{
			{
				Name:        "get_weather",
				Description: "Get weather",
				Parameters: prompty.MustJSONDocumentFromMap(map[string]any{
					"type":       "object",
					"properties": map[string]any{},
				}),
			},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Tools, 1)
	assert.Equal(t, "function", req.Tools[0].Type)
	assert.Equal(t, "get_weather", req.Tools[0].Function.Name)
	assert.Equal(t, "Get weather", req.Tools[0].Function.Description)
}

func TestTranslate_WithForcedToolInstruction(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{
				Role:    prompty.RoleUser,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "Call get_weather"}},
			},
		},
		Tools: []prompty.ToolDefinition{
			{Name: "get_weather", Parameters: prompty.MustJSONDocumentFromMap(map[string]any{"type": "object"})},
		},
		ForcedTool: "get_weather",
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	assert.Equal(t, "system", req.Messages[0].Role)
	assert.Contains(t, req.Messages[0].Content, `"get_weather"`)
}

func TestTranslate_ToolResult(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{
					ToolCallID: "call_1",
					Name:       "get_weather",
					Content:    []prompty.ContentPart{prompty.TextPart{Text: "Sunny"}},
					IsError:    false,
				},
			}},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Equal(t, "tool", req.Messages[0].Role)
	assert.Equal(t, "call_1", req.Messages[0].ToolCallID)
	assert.Equal(t, "Sunny", req.Messages[0].Content)
}

func TestTranslate_BatchedToolResults(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleTool, Content: []prompty.ContentPart{
				prompty.ToolResultPart{
					ToolCallID: "call_1",
					Name:       "get_weather",
					Content:    []prompty.ContentPart{prompty.TextPart{Text: "Sunny"}},
					IsError:    false,
				},
				prompty.ToolResultPart{
					ToolCallID: "call_2",
					Name:       "get_time",
					Content:    []prompty.ContentPart{prompty.TextPart{Text: "12:00"}},
					IsError:    true,
				},
			}},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 2)
	assert.Equal(t, "tool", req.Messages[0].Role)
	assert.Equal(t, "call_1", req.Messages[0].ToolCallID)
	assert.Equal(t, "Sunny", req.Messages[0].Content)
	assert.Equal(t, "tool", req.Messages[1].Role)
	assert.Equal(t, "call_2", req.Messages[1].ToolCallID)
	assert.Equal(t, "12:00", req.Messages[1].Content)
}

func TestTranslate_ModelOptions(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelOptions: &prompty.ModelOptions{
			Temperature: new(0.5),
			MaxTokens:   new(int64(100)),
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.NotNil(t, req.Options)
	assert.InDelta(t, 0.5, req.Options["temperature"], 1e-6)
	assert.Equal(t, int64(100), req.Options["num_predict"])
}

func TestTranslate_ProviderSettingsMapping(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelOptions: &prompty.ModelOptions{
			ProviderSettings: prompty.MustJSONDocumentFromMap(map[string]any{
				"top_k":          10,
				"seed":           42,
				"num_ctx":        4096,
				"repeat_penalty": 1.1,
			}),
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.NotNil(t, req.Options)
	assert.Equal(t, 10, req.Options["top_k"])
	assert.Equal(t, 42, req.Options["seed"])
	assert.Equal(t, 4096, req.Options["num_ctx"])
	assert.InDelta(t, 1.1, req.Options["repeat_penalty"], 1e-6)
}

func TestTranslate_ProviderSettingsMapping_RejectsInvalidValues(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelOptions: &prompty.ModelOptions{
			ProviderSettings: prompty.MustJSONDocumentFromMap(map[string]any{
				"top_k":          10,
				"num_ctx":        "invalid",
				"repeat_penalty": "invalid",
			}),
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidProviderSettings)
}

func TestTranslate_ProviderSettingsMapping_RejectsUnknownKey(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelOptions: &prompty.ModelOptions{
			ProviderSettings: prompty.MustJSONDocumentFromMap(map[string]any{
				"top_k":          10,
				"unknown_ollama": true,
			}),
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidProviderSettings)
}

func TestTranslate_ImagePartData(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.TextPart{Text: "What is this?"},
				prompty.MediaPart{
					MediaType: "image",
					Data:      []byte{0xff, 0xd8},
					MIMEType:  "image/jpeg",
				},
			}},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Len(t, req.Messages[0].Images, 1)
	assert.Equal(t, api.ImageData([]byte{0xff, 0xd8}), req.Messages[0].Images[0])
}

func TestTranslate_ImagePartData_MIMEFallbackWhenMediaTypeEmpty(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MIMEType: "image/png", Data: []byte{0x89, 0x50, 0x4e, 0x47}},
			}},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Len(t, req.Messages[0].Images, 1)
	assert.Equal(t, api.ImageData([]byte{0x89, 0x50, 0x4e, 0x47}), req.Messages[0].Images[0])
}

func TestTranslate_ImagePartData_MediaTypeFallbackWhenMIMEEmpty(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "image", Data: []byte{0x89, 0x50, 0x4e, 0x47}},
			}},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Len(t, req.Messages[0].Images, 1)
	assert.Equal(t, api.ImageData([]byte{0x89, 0x50, 0x4e, 0x47}), req.Messages[0].Images[0])
}

func TestTranslate_ImagePartData_MIMEWinsOverConflictingMediaType(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{
					MediaType: "document",
					MIMEType:  "image/png",
					Data:      []byte{0x89, 0x50, 0x4e, 0x47},
				},
			}},
		},
	}
	req, err := a.Translate(exec)
	require.NoError(t, err)
	require.Len(t, req.Messages, 1)
	assert.Len(t, req.Messages[0].Images, 1)
	assert.Equal(t, api.ImageData([]byte{0x89, 0x50, 0x4e, 0x47}), req.Messages[0].Images[0])
}

func TestTranslate_MediaPartURLWithoutData_ReturnsErrMediaNotResolved(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{URL: "https://example.com/img.png", MIMEType: "image/png"},
			}},
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrMediaNotResolved)
}

func TestTranslate_ImagePartEmptyRejected(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{MIMEType: "image/png"},
			}},
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	assert.Contains(t, err.Error(), "neither Data nor URL")
}

func TestTranslate_UserAudioPartRejected(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{
					MediaType: "audio",
					MIMEType:  "audio/mpeg",
					Data:      []byte{0x01, 0x02},
				},
			}},
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	assert.Contains(t, err.Error(), "only supports image media")
}

func TestTranslate_UserMediaPartNonImageMIMERejected_EvenWithImageMediaType(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.MediaPart{
					MediaType: "image",
					MIMEType:  "application/pdf",
					Data:      []byte("%PDF-1.7"),
				},
			}},
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	assert.Contains(t, err.Error(), "only supports image media")
}

func TestTranslate_NilExecution(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Translate(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrNilExecution)
}

func TestTranslate_ForcedToolNotDeclared_ReturnsError(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Translate(&prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{
				Role:    prompty.RoleUser,
				Content: []prompty.ContentPart{prompty.TextPart{Text: "Call tool"}},
			},
		},
		Tools:      []prompty.ToolDefinition{{Name: "other_tool"}},
		ForcedTool: "missing_tool",
	})
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrForcedToolNotFound)
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
	req, err := a.Translate(exec)
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

func TestTranslate_AssistantToolCall_RejectsArgsChunkWithoutGlue(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "call_1", Name: "get_weather", ArgsChunk: `{"location":"NYC"}`},
			}},
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrIncompleteToolCallArgs)
}

func TestTranslate_UnsupportedRole(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: "unknown_role", Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	_, err := a.Translate(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrUnsupportedRole)
}

func TestParseResponse_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &api.ChatResponse{
		Message: api.Message{Role: "assistant", Content: "Hello back"},
	}
	pResp, err := a.ParseResponse(resp)
	require.NoError(t, err)
	require.Len(t, pResp.Content, 1)
	assert.Equal(t, "Hello back", pResp.Content[0].(prompty.TextPart).Text)
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
	pResp, err := a.ParseResponse(resp)
	require.NoError(t, err)
	require.Len(t, pResp.Content, 1)
	tc := pResp.Content[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Contains(t, tc.Args, "NYC")
}

func TestParseResponse_EmptyToolCallArgsFailsClosed(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &api.ChatResponse{
		Message: api.Message{
			ToolCalls: []api.ToolCall{{
				ID: "call_1",
				Function: api.ToolCallFunction{
					Name:      "get_weather",
					Arguments: api.NewToolCallFunctionArguments(),
				},
			}},
		},
	}
	_, err := a.ParseResponse(resp)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrMalformedArgs)
}

func TestParseResponse_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseResponse_EmptyContentAndNoToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &api.ChatResponse{
		Message: api.Message{Role: "assistant", Content: ""},
	}
	_, err := a.ParseResponse(resp)
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
		ModelOptions: &prompty.ModelOptions{Stop: []string{"STOP", "END"}},
	}
	req, err := a.Translate(exec)
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
	_, err := a.Translate(exec)
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
		ResponseFormat: &prompty.SchemaDefinition{
			Schema: prompty.MustJSONDocumentFromMap(map[string]any{"type": "object"}),
		},
	}
	_, err := a.Translate(exec)
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
	parts, err := a.ParseStreamChunk(chunk)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello ", parts[0].(prompty.TextPart).Text)
}

func TestParseStreamChunk_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseStreamChunk(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseStreamChunk_MalformedToolArgsFails(t *testing.T) {
	t.Parallel()
	a := New()
	args := api.NewToolCallFunctionArguments()
	args.Set("bad", make(chan int))
	chunk := &api.ChatResponse{
		Message: api.Message{
			ToolCalls: []api.ToolCall{{
				ID: "call_1",
				Function: api.ToolCallFunction{
					Name:      "get_weather",
					Arguments: args,
				},
			}},
		},
		Done: false,
	}
	_, err := a.ParseStreamChunk(chunk)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrMalformedArgs)
}
