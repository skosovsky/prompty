package prompty

import (
	"testing"
)

type benchInput struct {
	UserName string `json:"user_name" prompt:"user_name"`
	Query    string `json:"query"     prompt:"query"`
}

func BenchmarkStructBindingCacheHit(b *testing.B) {
	input := benchInput{UserName: "alice", Query: "hello"}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.user_name }}")},
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		val, binding, _, err := extractStructPayload(&input)
		if err != nil {
			b.Fatal(err)
		}
		releaseBoundInputMap(buildStructTemplateInput(tpl, val, binding))
	}
}
