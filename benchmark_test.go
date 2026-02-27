package prompty

import (
	"context"
	"testing"
)

func BenchmarkFormatStruct(b *testing.B) {
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: "You are {{ .bot_name }}."},
		{Role: RoleUser, Content: "{{ .query }}"},
	}, WithPartialVariables(map[string]any{"bot_name": "Helper"}))
	if err != nil {
		b.Fatal(err)
	}
	type P struct {
		Query string `prompt:"query"`
	}
	ctx := context.Background()
	payload := &P{Query: "What is 2+2?"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = tpl.FormatStruct(ctx, payload)
	}
}

func BenchmarkGetPayloadFields(b *testing.B) {
	type P struct {
		A string `prompt:"a"`
		B string `prompt:"b"`
		C string `prompt:"c"`
	}
	payload := &P{A: "x", B: "y", C: "z"}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _, _ = getPayloadFields(payload)
	}
}

func BenchmarkRenderToolsAsXML(b *testing.B) {
	tools := []ToolDefinition{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		{Name: "search", Description: "Search", Parameters: nil},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = renderToolsAsXML(tools)
	}
}

func BenchmarkRenderToolsAsJSON(b *testing.B) {
	tools := []ToolDefinition{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		{Name: "search", Description: "Search", Parameters: nil},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = renderToolsAsJSON(tools)
	}
}
