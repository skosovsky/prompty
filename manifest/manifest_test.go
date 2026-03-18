package manifest

import (
	"context"
	"embed"
	"strings"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

//go:embed testdata/*.json
var testdataFS embed.FS

var jsonParser = NewJSONParser()

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestParse_ValidSimple(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"simple_prompt","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Hello, {{ .user_name }}."}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "simple_prompt", tpl.Metadata.ID)
	assert.Equal(t, "1", tpl.Metadata.Version)
	require.Len(t, tpl.Messages, 1)
	assert.Equal(t, prompty.RoleSystem, tpl.Messages[0].Role)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "text", tpl.Messages[0].Content[0].Type)
	assert.Equal(t, "Hello, {{ .user_name }}.", tpl.Messages[0].Content[0].Text)
}

func TestParse_NilParser(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"x","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Hi"}]}]}`)
	_, err := Parse(data, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrNoParser)
}

func TestParse_ValidFull(t *testing.T) {
	t.Parallel()
	data, err := testdataFS.ReadFile("testdata/valid_full.json")
	require.NoError(t, err)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "support_agent", tpl.Metadata.ID)
	assert.Equal(t, "1.2", tpl.Metadata.Version)
	assert.Equal(t, "Customer support agent", tpl.Metadata.Description)
	assert.Equal(t, []string{"user_query"}, tpl.RequiredVars)
	assert.Equal(t, "SupportBot", tpl.PartialVariables["bot_name"])
	require.Len(t, tpl.Tools, 1)
	assert.Equal(t, "get_order_status", tpl.Tools[0].Name)
	require.Len(t, tpl.Messages, 2)
	assert.True(t, tpl.Messages[1].Optional)
}

func TestParse_InvalidMissingId(t *testing.T) {
	t.Parallel()
	data, err := testdataFS.ReadFile("testdata/invalid_missing_id.json")
	require.NoError(t, err)
	_, err = Parse(data, jsonParser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestParse_InvalidMissingMessages(t *testing.T) {
	t.Parallel()
	data, err := testdataFS.ReadFile("testdata/invalid_missing_messages.json")
	require.NoError(t, err)
	_, err = Parse(data, jsonParser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestParse_InvalidJSON(t *testing.T) {
	t.Parallel()
	data := []byte(`{invalid json}`)
	_, err := Parse(data, jsonParser)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestParse_MetadataTagsAndExtras(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"x","version":"1","metadata":{"tags":["a","b"],"domain":"medical","version":"2"},"messages":[{"role":"system","content":[{"type":"text","text":"Hi"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, []string{"a", "b"}, tpl.Metadata.Tags)
	require.NotNil(t, tpl.Metadata.Extras)
	assert.Equal(t, "medical", tpl.Metadata.Extras["domain"])
	assert.Equal(t, "2", tpl.Metadata.Extras["version"])
}

func TestParse_MetadataEnvironmentTyped(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"x","version":"1","metadata":{"environment":"prod","tags":["a"],"custom":"val"},"messages":[{"role":"system","content":[{"type":"text","text":"Hi"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "prod", tpl.Metadata.Environment)
	assert.Equal(t, []string{"a"}, tpl.Metadata.Tags)
	require.NotNil(t, tpl.Metadata.Extras)
	assert.Equal(t, "val", tpl.Metadata.Extras["custom"])
	assert.NotContains(t, tpl.Metadata.Extras, "environment")
	assert.NotContains(t, tpl.Metadata.Extras, "tags")
}

func TestParse_AcceptsCustomRole(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"x","version":"1","messages":[{"role":"custom_alien","content":[{"type":"text","text":"Hi"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages, 1)
	assert.Equal(t, prompty.Role("custom_alien"), tpl.Messages[0].Role)
}

func TestParse_ContentScalar(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"scalar_content","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"Ты ассистент"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.Len(t, tpl.Messages, 1)
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "text", tpl.Messages[0].Content[0].Type)
	assert.Equal(t, "Ты ассистент", tpl.Messages[0].Content[0].Text)
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.Len(t, exec.Messages[0].Content, 1)
	assert.Equal(t, "Ты ассистент", exec.Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestParse_ContentMultimodalArray(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"multimodal","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"Look: {{ .x }}"},{"type":"image_url","url":"{{ .img }}"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.Len(t, tpl.Messages, 1)
	require.Len(t, tpl.Messages[0].Content, 2)
	assert.Equal(t, "text", tpl.Messages[0].Content[0].Type)
	assert.Equal(t, "Look: {{ .x }}", tpl.Messages[0].Content[0].Text)
	assert.Equal(t, "image_url", tpl.Messages[0].Content[1].Type)
	assert.Equal(t, "{{ .img }}", tpl.Messages[0].Content[1].URL)
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X   string `json:"x"`
		Img string `json:"img"`
	}{X: "done", Img: "https://example.com/photo.png"})
	require.NoError(t, err)
	require.Len(t, exec.Messages[0].Content, 2)
	assert.Equal(t, "Look: done", exec.Messages[0].Content[0].(prompty.TextPart).Text)
	mp := exec.Messages[0].Content[1].(prompty.MediaPart)
	assert.Equal(t, "image", mp.MediaType)
	assert.Equal(t, "https://example.com/photo.png", mp.URL)
}

func TestParseFile(t *testing.T) {
	t.Parallel()
	tpl, err := ParseFile("testdata/valid_simple.json", jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "simple_prompt", tpl.Metadata.ID)
}

func TestParseFS(t *testing.T) {
	t.Parallel()
	tpl, err := ParseFS(testdataFS, "testdata/valid_simple.json", jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "simple_prompt", tpl.Metadata.ID)
}

func TestParse_ResponseFormat(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"with_schema","version":"1","messages":[{"role":"user","content":[{"type":"text","text":"Return JSON"}]}],"response_format":{"name":"my_schema","schema":{"type":"object","properties":{"key":{"type":"string"}}}}}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.NotNil(t, tpl.ResponseFormat)
	assert.Equal(t, "my_schema", tpl.ResponseFormat.Name)
	require.NotNil(t, tpl.ResponseFormat.Schema)
	assert.Equal(t, "object", tpl.ResponseFormat.Schema["type"])
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.NotNil(t, exec.ResponseFormat)
	assert.Equal(t, "my_schema", exec.ResponseFormat.Name)
}

func TestParse_MetadataPassThrough_ArbitraryKeys(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"with_metadata_arbitrary","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"You are a helper."}],"metadata":{"custom_user_id":"u-123"}},{"role":"user","content":[{"type":"text","text":"Hi"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages, 2)
	require.NotNil(t, tpl.Messages[0].Metadata)
	assert.Equal(t, "u-123", tpl.Messages[0].Metadata["custom_user_id"])
	assert.Nil(t, tpl.Messages[1].Metadata)
}

func TestParse_MetadataPassThrough(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"with_metadata_pass","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"You are a helper. {{ .x }}"}],"metadata":{"gemini_search_grounding":true}},{"role":"user","content":[{"type":"text","text":"Hi"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{X: "ok"})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.Len(t, exec.Messages, 2)
	require.NotNil(t, exec.Messages[0].Metadata)
	assert.Equal(t, true, exec.Messages[0].Metadata["gemini_search_grounding"], "metadata from manifest must reach PromptExecution")
}

func TestParse_CacheTrueAndMetadata(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"with_cache_and_metadata","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"You are a helper."}],"cache":true,"metadata":{"gemini_search_grounding":true}},{"role":"user","content":[{"type":"text","text":"Hi"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages, 2)
	assert.True(t, tpl.Messages[0].CachePoint)
	require.NotNil(t, tpl.Messages[0].Metadata)
	assert.Equal(t, true, tpl.Messages[0].Metadata["gemini_search_grounding"])
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.Len(t, exec.Messages, 2)
	assert.True(t, exec.Messages[0].CachePoint)
	require.NotNil(t, exec.Messages[0].Metadata)
	assert.Equal(t, true, exec.Messages[0].Metadata["gemini_search_grounding"])
}

func TestParse_FuncMapHelpersManifestPath(t *testing.T) {
	t.Parallel()
	data := []byte(`{"id":"secure","version":"1","messages":[{"role":"system","content":[{"type":"text","text":"{{ $d := randomHex 8 }}\\n<data_{{ $d }}>{{ .user_input | escapeXML }}</data_{{ $d }}"}]}]}`)
	tpl, err := Parse(data, jsonParser)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		UserInput string `prompt:"user_input"`
	}{UserInput: "</patient_input>"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(prompty.TextPart).Text
	assert.Contains(t, text, "&lt;/patient_input&gt;", "escapeXML must work for manifest-driven templates")
	assert.NotContains(t, text, "</patient_input>")

	openIdx := strings.Index(text, "<data_")
	closeIdx := strings.Index(text, "</data_")
	require.Greater(t, openIdx, -1)
	require.Greater(t, closeIdx, openIdx)
	delimOpen := text[openIdx+6 : openIdx+6+16]
	delimClose := text[closeIdx+7 : closeIdx+7+16]
	assert.Equal(t, delimOpen, delimClose, "same randomHex value must appear in both tags")
	assert.Regexp(t, `^[0-9a-f]{16}$`, delimOpen)
}
