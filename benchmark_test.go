package prompty

import "testing"

// Run with: go test -bench=BenchmarkRenderPlanExecute -benchmem to verify allocs/op and B/op.
func BenchmarkRenderPlanExecute(b *testing.B) {
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("You are {{ .Input.bot_name }}.")},
		{Role: RoleUser, Content: TextContent("{{ .Input.query }}")},
	}, WithPartialVariables(map[string]any{"bot_name": "Helper"}))
	if err != nil {
		b.Fatal(err)
	}
	type P struct {
		Query string `prompt:"query"`
	}
	payload := &P{Query: "What is 2+2?"}
	b.ResetTimer()
	for range b.N {
		_, _ = executeTemplatePlan(tpl, payload)
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
	for range b.N {
		_, _, _, _ = extractStructPayload(payload)
	}
}

func BenchmarkRenderToolsAsXML(b *testing.B) {
	tools := []ToolDefinition{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		{Name: "search", Description: "Search", Parameters: nil},
	}
	b.ResetTimer()
	for range b.N {
		_, _ = renderToolsAsXML(tools)
	}
}

func BenchmarkRenderToolsAsJSON(b *testing.B) {
	tools := []ToolDefinition{
		{Name: "get_weather", Description: "Get weather", Parameters: map[string]any{"type": "object"}},
		{Name: "search", Description: "Search", Parameters: nil},
	}
	b.ResetTimer()
	for range b.N {
		_, _ = renderToolsAsJSON(tools)
	}
}
