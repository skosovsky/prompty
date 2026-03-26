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
	goleak.VerifyTestMain(m)
}

func TestTextFromParts(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		parts []prompty.ContentPart
		want  string
	}{
		{"empty slice", []prompty.ContentPart{}, ""},
		{"nil slice", nil, ""},
		{"single text", []prompty.ContentPart{prompty.TextPart{Text: "hello"}}, "hello"},
		{"multiple text", []prompty.ContentPart{
			prompty.TextPart{Text: "a"},
			prompty.TextPart{Text: "b"},
			prompty.TextPart{Text: "c"},
		}, "abc"},
		{"mixed parts", []prompty.ContentPart{
			prompty.TextPart{Text: "x"},
			prompty.MediaPart{MediaType: "image", URL: "https://x"},
			prompty.TextPart{Text: "y"},
			prompty.ToolCallPart{ID: "1", Name: "f", Args: "{}"},
		}, "xy"},
		{"no text", []prompty.ContentPart{
			prompty.MediaPart{MediaType: "image", URL: "https://x"},
		}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := prompty.TextFromParts(tt.parts)
			assert.Equal(t, tt.want, got)
		})
	}
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
	assert.Equal(t, "resp-req", resp.Text())
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
	assert.Equal(t, "chunk", prompty.TextFromParts(chunks[0].Content))
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
