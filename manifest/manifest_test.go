package manifest

import (
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

func TestParseBytes_InvalidRole(t *testing.T) {
	t.Parallel()
	data := []byte(`
id: x
version: "1"
messages:
  - role: invalid_role
    content: "Hi"
`)
	_, err := ParseBytes(data)
	require.Error(t, err)
	assert.ErrorIs(t, err, prompty.ErrInvalidManifest)
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
