//go:build integration

package prompty

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegrationFuncMap_HasTool(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent(`{{ if hasTool .Input.allowed "my_tool" }}Use my tool!{{ end }}`)},
	})
	require.NoError(t, err)
	type payload struct {
		Allowed []string `prompt:"allowed"`
	}
	exec, err := executeTemplatePlan(tpl, &payload{Allowed: []string{"my_tool"}})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Equal(t, "Use my tool!", text)
	exec2, err := executeTemplatePlan(tpl, &payload{Allowed: []string{"other"}})
	require.NoError(t, err)
	text2 := exec2.Messages[0].Content[0].(TextPart).Text
	assert.Empty(t, text2)
}

func TestIntegrationFuncMap_EscapeXML(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("Input: {{ .Input.UserInput | escapeXML }}")},
	})
	require.NoError(t, err)
	type Payload struct {
		UserInput string `prompt:"UserInput"`
	}
	exec, err := executeTemplatePlan(tpl, &Payload{UserInput: "x</tag>y"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Contains(t, text, "&lt;/tag&gt;", "escapeXML must be applied in template render")
	assert.NotContains(t, text, "</tag>")
}

// TestFuncMap_RandomHex_Integration verifies randomHex is available in the template pipeline and yields same delimiter in one render.

func TestIntegrationFuncMap_RandomHex(t *testing.T) {
	t.Parallel()
	content := "{{ $d := randomHex 8 }}<data_{{ $d }}>body</data_{{ $d }}>"
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent(content)},
	})
	require.NoError(t, err)
	type Payload struct {
		X string `prompt:"x"`
	}
	exec, err := executeTemplatePlan(tpl, &Payload{X: "ok"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	// Opening and closing tags must use the same 16-char hex.
	openIdx := strings.Index(text, "<data_")
	closeIdx := strings.Index(text, "</data_")
	require.Greater(t, openIdx, -1, "opening tag must be present")
	require.Greater(t, closeIdx, openIdx, "closing tag must be present")
	delimOpen := text[openIdx+6 : openIdx+6+16]
	delimClose := text[closeIdx+7 : closeIdx+7+16]
	assert.Len(t, delimOpen, 16)
	assert.Len(t, delimClose, 16)
	assert.Equal(t, delimOpen, delimClose, "same randomHex value must appear in both tags")
	assert.Regexp(t, `^[0-9a-f]+$`, delimOpen)
}
