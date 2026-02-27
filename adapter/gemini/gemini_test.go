package gemini

import (
	"fmt"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
	"google.golang.org/genai"
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
	req, _ := a.TranslateTyped(exec)
	fmt.Println(req.Contents[0].Parts[0].Text)
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
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.Len(t, req.Contents, 1)
	assert.Len(t, req.Contents[0].Parts, 1)
	assert.Equal(t, "Hello", req.Contents[0].Parts[0].Text)
	assert.Equal(t, string(genai.RoleUser), req.Contents[0].Role)
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
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.NotNil(t, req.Config.SystemInstruction)
	require.Len(t, req.Config.SystemInstruction.Parts, 1)
	assert.Equal(t, "You are a helper.", req.Config.SystemInstruction.Parts[0].Text)
	require.Len(t, req.Contents, 1)
	assert.Equal(t, "Hi", req.Contents[0].Parts[0].Text)
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
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.Len(t, req.Config.Tools, 1)
	decls := req.Config.Tools[0].FunctionDeclarations
	require.Len(t, decls, 1)
	assert.Equal(t, "get_weather", decls[0].Name)
	assert.Equal(t, "Get weather", decls[0].Description)
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
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 1)
	assert.NotNil(t, req.Contents[0].Parts[0].FunctionResponse)
	assert.Equal(t, "get_weather", req.Contents[0].Parts[0].FunctionResponse.Name)
	assert.Equal(t, "Sunny", req.Contents[0].Parts[0].FunctionResponse.Response["result"])
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
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.NotNil(t, req.Config.Temperature)
	assert.Equal(t, float32(0.5), *req.Config.Temperature)
	assert.Equal(t, int32(100), req.Config.MaxOutputTokens)
}

func TestTranslate_NilExecution(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.Translate(nil)
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
				prompty.TextPart{Text: "What is in this image?"},
				prompty.ImagePart{Data: imgData, MIMEType: "image/jpeg"},
			}},
		},
	}
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 2)
	assert.Equal(t, "What is in this image?", req.Contents[0].Parts[0].Text)
	assert.NotNil(t, req.Contents[0].Parts[1].InlineData)
	assert.Equal(t, "image/jpeg", req.Contents[0].Parts[1].InlineData.MIMEType)
}

func TestTranslate_ImagePartURL(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.ImagePart{URL: "https://example.com/img.png", MIMEType: "image/png"},
			}},
		},
	}
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.Len(t, req.Contents, 1)
	require.Len(t, req.Contents[0].Parts, 1)
	assert.NotNil(t, req.Contents[0].Parts[0].FileData)
	assert.Equal(t, "https://example.com/img.png", req.Contents[0].Parts[0].FileData.FileURI)
}

func TestTranslate_ImagePartEmptyRejected(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{
				prompty.ImagePart{},
			}},
		},
	}
	_, err := a.TranslateTyped(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	assert.Contains(t, err.Error(), "neither Data nor URL")
}

func TestTranslate_AssistantToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
				prompty.ToolCallPart{ID: "call_1", Name: "get_weather", Args: `{"location":"NYC"}`},
			}},
		},
	}
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.Len(t, req.Contents, 1)
	assert.Equal(t, string(genai.RoleModel), req.Contents[0].Role)
	require.Len(t, req.Contents[0].Parts, 1)
	assert.NotNil(t, req.Contents[0].Parts[0].FunctionCall)
	assert.Equal(t, "get_weather", req.Contents[0].Parts[0].FunctionCall.Name)
	assert.Equal(t, "NYC", req.Contents[0].Parts[0].FunctionCall.Args["location"])
}

func TestTranslate_UnsupportedRole(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: "unknown_role", Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	_, err := a.TranslateTyped(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrUnsupportedRole)
}

func TestTranslate_MaxTokensTruncationGuard(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
		ModelConfig: map[string]any{"max_tokens": int64(999_999_999_999)},
	}
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	assert.Equal(t, int32(2147483647), req.Config.MaxOutputTokens)
}

func TestParseResponse_TextOnly(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Parts: []*genai.Part{{Text: "Hello back"}},
			},
		}},
	}
	parts, err := a.ParseResponse(resp)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	assert.Equal(t, "Hello back", parts[0].(prompty.TextPart).Text)
}

func TestParseResponse_ToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{
				Parts: []*genai.Part{{
					FunctionCall: &genai.FunctionCall{
						ID:   "call_1",
						Name: "get_weather",
						Args: map[string]any{"location": "NYC"},
					},
				}},
			},
		}},
	}
	parts, err := a.ParseResponse(resp)
	require.NoError(t, err)
	require.Len(t, parts, 1)
	tc := parts[0].(prompty.ToolCallPart)
	assert.Equal(t, "call_1", tc.ID)
	assert.Equal(t, "get_weather", tc.Name)
	assert.Contains(t, tc.Args, `"location"`)
	assert.Contains(t, tc.Args, "NYC")
}

func TestParseResponse_InvalidType(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse("not a response")
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrInvalidResponse)
}

func TestParseResponse_EmptyCandidates(t *testing.T) {
	t.Parallel()
	a := New()
	_, err := a.ParseResponse(&genai.GenerateContentResponse{Candidates: nil})
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrEmptyResponse)
}

func TestParseResponse_EmptyContentAndNoToolCalls(t *testing.T) {
	t.Parallel()
	a := New()
	resp := &genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content: &genai.Content{Parts: []*genai.Part{{Text: ""}}},
		}},
	}
	_, err := a.ParseResponse(resp)
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
	_, err := a.TranslateTyped(exec)
	require.Error(t, err)
	assert.ErrorIs(t, err, adapter.ErrMalformedArgs)
}

func TestTranslate_EmptyMessages(t *testing.T) {
	t.Parallel()
	a := New()
	exec := &prompty.PromptExecution{Messages: nil}
	req, err := a.TranslateTyped(exec)
	require.NoError(t, err)
	require.NotNil(t, req)
	assert.Len(t, req.Contents, 0)
}
