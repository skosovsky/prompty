package adapter

import (
	"context"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m,
		goleak.IgnoreTopFunction("go.opencensus.io/stats/view.(*worker).start"),
	)
}

func TestPrepareTranslateExecution_DoesNotMutateInput(t *testing.T) {
	t.Parallel()
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "A"}}},
			{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "B"}}},
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Hi"}}},
		},
	}
	before := exec.Clone()
	working := PrepareTranslateExecution(exec)
	require.NotNil(t, working)
	assert.Len(t, working.Messages, 2, "normalized working copy merges system messages")
	assert.Equal(t, before.Messages, exec.Messages, "input PromptExecution must stay unchanged")
}

func TestStrictTextFromParts(t *testing.T) {
	t.Parallel()
	got, err := prompty.StrictTextFromParts([]prompty.ContentPart{prompty.TextPart{Text: "hello"}})
	require.NoError(t, err)
	assert.Equal(t, "hello", got)

	_, err = prompty.StrictTextFromParts([]prompty.ContentPart{
		prompty.TextPart{Text: "x"},
		prompty.ToolCallPart{ID: "1", Name: "f", Args: "{}"},
	})
	require.Error(t, err)
}

func TestJoinAdapterTextParts(t *testing.T) {
	t.Parallel()
	got, err := prompty.JoinAdapterTextParts([]prompty.ContentPart{
		prompty.TextPart{Text: "x"},
		prompty.MediaPart{MediaType: "image", URL: "https://x"},
		prompty.TextPart{Text: "y"},
		prompty.ToolCallPart{ID: "1", Name: "f", Args: "{}"},
	})
	require.NoError(t, err)
	assert.Equal(t, "xy", got)
}

func TestNewClient_Execute(t *testing.T) {
	t.Parallel()
	type mockReq struct{ text string }
	type mockResp struct{ text string }
	mock := &mockAdapter[mockReq, mockResp]{
		translate: func(_ *prompty.PromptExecution) (mockReq, error) {
			return mockReq{text: "req"}, nil
		},
		execute: func(_ context.Context, req mockReq) (mockResp, error) {
			return mockResp{text: "resp-" + req.text}, nil
		},
		parseResponse: func(raw mockResp) (*prompty.Response, error) {
			return &prompty.Response{
				Content: []prompty.ContentPart{prompty.TextPart{Text: raw.text}},
			}, nil
		},
	}
	client := NewClient(mock)
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "hi"}}},
		},
	}
	resp, err := client.Execute(context.Background(), exec)
	require.NoError(t, err)
	require.NotNil(t, resp)
	text, err := resp.StrictText()
	require.NoError(t, err)
	assert.Equal(t, "resp-req", text)
}

func TestNewClient_ExecuteStream_Polyfill(t *testing.T) {
	t.Parallel()
	type mockReq struct{}
	type mockResp struct{}
	mock := &mockAdapter[mockReq, mockResp]{
		translate: func(_ *prompty.PromptExecution) (mockReq, error) {
			return mockReq{}, nil
		},
		execute: func(_ context.Context, _ mockReq) (mockResp, error) {
			return mockResp{}, nil
		},
		parseResponse: func(_ mockResp) (*prompty.Response, error) {
			return &prompty.Response{
				Content: []prompty.ContentPart{prompty.TextPart{Text: "chunk"}},
			}, nil
		},
	}
	client := NewClient(mock)
	exec := &prompty.PromptExecution{
		Messages: []prompty.ChatMessage{
			{Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "hi"}}},
		},
	}
	seq := client.ExecuteStream(context.Background(), exec)
	var chunks []*prompty.ResponseChunk
	for chunk, err := range seq {
		require.NoError(t, err)
		chunks = append(chunks, chunk)
	}
	require.Len(t, chunks, 1)
	assert.True(t, chunks[0].IsFinished)
	chunkText, err := prompty.StrictTextFromParts(chunks[0].Content)
	require.NoError(t, err)
	assert.Equal(t, "chunk", chunkText)
}

type mockAdapter[Req, Resp any] struct {
	translate     func(*prompty.PromptExecution) (Req, error)
	execute       func(context.Context, Req) (Resp, error)
	parseResponse func(Resp) (*prompty.Response, error)
}

func (m *mockAdapter[Req, Resp]) Translate(exec *prompty.PromptExecution) (Req, error) {
	return m.translate(exec)
}
func (m *mockAdapter[Req, Resp]) Execute(ctx context.Context, req Req) (Resp, error) {
	return m.execute(ctx, req)
}
func (m *mockAdapter[Req, Resp]) ParseResponse(raw Resp) (*prompty.Response, error) {
	return m.parseResponse(raw)
}
