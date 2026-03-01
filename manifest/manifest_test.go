package manifest

import (
	"context"
	"embed"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

//go:embed testdata/*.yaml
var testdataFS embed.FS

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestParseBytes_ValidSimple(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: simple_prompt
version: "1"
messages:
  - role: system
    content: "Hello, {{ .user_name }}."
`)
	tpl, err := ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "simple_prompt", tpl.Metadata.ID)
	assert.Equal(t, "1", tpl.Metadata.Version)
	require.Len(t, tpl.Messages, 1)
	assert.Equal(t, prompty.RoleSystem, tpl.Messages[0].Role)
	assert.Equal(t, "Hello, {{ .user_name }}.", tpl.Messages[0].Content)
}

func TestParseBytes_ValidFull(t *testing.T) {
	t.Parallel()
	data, err := testdataFS.ReadFile("testdata/valid_full.yaml")
	require.NoError(t, err)
	tpl, err := ParseBytes(data)
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

func TestParseBytes_InvalidMissingId(t *testing.T) {
	t.Parallel()
	data, err := testdataFS.ReadFile("testdata/invalid_missing_id.yaml")
	require.NoError(t, err)
	_, err = ParseBytes(data)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestParseBytes_InvalidMissingMessages(t *testing.T) {
	t.Parallel()
	data, err := testdataFS.ReadFile("testdata/invalid_missing_messages.yaml")
	require.NoError(t, err)
	_, err = ParseBytes(data)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestParseBytes_InvalidBadYAML(t *testing.T) {
	t.Parallel()
	data := []byte("id: x\nmessages:\n  - role: system\n  content: [unclosed")
	_, err := ParseBytes(data)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
}

func TestParseBytes_AcceptsCustomRole(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: x
version: "1"
messages:
  - role: custom_alien
    content: "Hi"
`)
	tpl, err := ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages, 1)
	assert.Equal(t, prompty.Role("custom_alien"), tpl.Messages[0].Role)
}

func TestParseFile(t *testing.T) {
	t.Parallel()
	tpl, err := ParseFile("testdata/valid_simple.yaml")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "simple_prompt", tpl.Metadata.ID)
}

func TestParseFS(t *testing.T) {
	t.Parallel()
	tpl, err := ParseFS(testdataFS, "testdata/valid_simple.yaml")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	assert.Equal(t, "simple_prompt", tpl.Metadata.ID)
}

func TestParseBytes_ResponseFormat(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: with_schema
version: "1"
messages:
  - role: user
    content: "Return JSON"
response_format:
  name: my_schema
  schema:
    type: object
    properties:
      key:
        type: string
`)
	tpl, err := ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.NotNil(t, tpl.ResponseFormat)
	assert.Equal(t, "my_schema", tpl.ResponseFormat.Name)
	require.NotNil(t, tpl.ResponseFormat.Schema)
	assert.Equal(t, "object", tpl.ResponseFormat.Schema["type"])
	// Payload must have at least one prompt/json tag for getPayloadFields to accept it
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.NotNil(t, exec.ResponseFormat)
	assert.Equal(t, "my_schema", exec.ResponseFormat.Name)
}

func TestParseBytes_MetadataAnthropicCache(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: with_metadata_cache
version: "1"
messages:
  - role: system
    content: "You are a helper."
    metadata:
      anthropic_cache: true
  - role: user
    content: "Hi"
`)
	tpl, err := ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages, 2)
	require.NotNil(t, tpl.Messages[0].Metadata)
	assert.Equal(t, true, tpl.Messages[0].Metadata["anthropic_cache"])
	assert.Nil(t, tpl.Messages[1].Metadata)
}

func TestParseBytes_MetadataPassThrough(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: with_metadata_pass
version: "1"
messages:
  - role: system
    content: "You are a helper. {{ .x }}"
    metadata:
      anthropic_cache: true
  - role: user
    content: "Hi"
`)
	tpl, err := ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{X: "ok"})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.Len(t, exec.Messages, 2)
	require.NotNil(t, exec.Messages[0].Metadata)
	assert.Equal(t, true, exec.Messages[0].Metadata["anthropic_cache"], "metadata from manifest must reach PromptExecution")
}

func TestParseBytes_CacheTrueAndMetadata(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: with_cache_and_metadata
version: "1"
messages:
  - role: system
    content: "You are a helper."
    cache: true
    metadata:
      gemini_search_grounding: true
  - role: user
    content: "Hi"
`)
	tpl, err := ParseBytes(data)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Len(t, tpl.Messages, 2)
	// cache: true is converted to metadata["anthropic_cache"] for backward compatibility.
	require.NotNil(t, tpl.Messages[0].Metadata)
	assert.Equal(t, true, tpl.Messages[0].Metadata["anthropic_cache"])
	assert.Equal(t, true, tpl.Messages[0].Metadata["gemini_search_grounding"])
	exec, err := tpl.FormatStruct(context.Background(), &struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.NotNil(t, exec)
	require.Len(t, exec.Messages, 2)
	require.NotNil(t, exec.Messages[0].Metadata)
	assert.Equal(t, true, exec.Messages[0].Metadata["anthropic_cache"])
	assert.Equal(t, true, exec.Messages[0].Metadata["gemini_search_grounding"])
}
