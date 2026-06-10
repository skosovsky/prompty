package prompty

import (
	"testing"
)

type benchInput struct {
	UserName string `prompt:"user_name"`
	Query    string `prompt:"query"`
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
		vars, _, err := bindTemplateVars(&input)
		if err != nil {
			b.Fatal(err)
		}
		releaseBoundInputMap(mergeBoundVarsWithPartials(tpl, vars))
	}
}
