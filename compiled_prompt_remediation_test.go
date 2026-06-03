package prompty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type compileStubRegistry struct{}

func (compileStubRegistry) Plan(_ context.Context, _ string, input RegistryPlanInput) (*RenderPlan, error) {
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "stub"}))
	if err != nil {
		return nil, err
	}
	return NewRenderPlanFromRegistryInput(tpl, input)
}

func (compileStubRegistry) ReadManifestBytes(context.Context, string) ([]byte, error) {
	return nil, ErrManifestBytesUnavailable
}

func TestCompileFromRegistry_ManifestBytesUnavailableFails(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "t"}))
	require.NoError(t, err)

	plan, err := newRenderPlanFromMap(tpl, map[string]any{"q": "x"})
	require.NoError(t, err)
	_, err = plan.CompileFromRegistry(context.Background(), compileStubRegistry{}, "t")
	require.ErrorIs(t, err, ErrManifestBytesUnavailable)
}

func TestCompileFromRegistryWithCanonicalSnapshot_UsesCanonicalDigest(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "t"}))
	require.NoError(t, err)

	plan, err := newRenderPlanFromMap(tpl, map[string]any{"q": "x"})
	require.NoError(t, err)
	cp, err := plan.CompileFromRegistryWithCanonicalSnapshot(context.Background(), "t")
	require.NoError(t, err)
	assert.Equal(t, DigestSourceCanonicalSnapshot, cp.DigestSource())
	assert.NotEmpty(t, cp.ManifestDigest())
}

func TestCompiledPrompt_MetadataCapabilitiesRoundTrip(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.Metadata.Capabilities = []string{"text", "vision"}

	cp, err := NewCompiledPrompt(exec, "agent", []byte("id: agent\n"))
	require.NoError(t, err)

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var restored CompiledPrompt
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.Equal(t, []string{"text", "vision"}, restored.PromptExecution().Metadata.Capabilities)
}

func TestCompiledPrompt_UnmarshalUnknownContentTypeFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 1,
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "canonical_snapshot",
		"execution": {
			"messages": [{
				"role": "user",
				"content": [{"type": "bogus", "text": "hi"}]
			}],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown content part type")
}

func TestCompiledPrompt_UnmarshalInvalidBase64Fails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 1,
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "canonical_snapshot",
		"execution": {
			"messages": [{
				"role": "user",
				"content": [{"type": "media", "data": "!!!not-base64!!!"}]
			}],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid base64")
}

func TestCompiledPrompt_UnmarshalMissingFormatVersionFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "canonical_snapshot",
		"execution": {
			"messages": [],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format_version")
}

func TestCompiledPrompt_MarshalUnknownPartTypeFails(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	exec.Messages[0].Content = append(exec.Messages[0].Content, nil)
	tplSrc, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("hi")},
	}, WithMetadata(PromptMetadata{ID: "agent"}))
	require.NoError(t, err)
	cp, err := NewCompiledPromptWithCanonicalSnapshot(exec, "agent", tplSrc)
	require.NoError(t, err)
	_, err = json.Marshal(cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil content part")
}

func TestCompiledPrompt_UnmarshalUnsupportedFormatVersionFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 99,
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "canonical_snapshot",
		"execution": {
			"messages": [],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported format_version")
}

func TestCompiledPrompt_UnmarshalUnknownTopLevelFieldFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 1,
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "canonical_snapshot",
		"unexpected": true,
		"execution": {
			"messages": [],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown field")
}

func TestCompiledPrompt_UnmarshalInvalidDigestSourceFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 1,
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "legacy",
		"execution": {
			"messages": [],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "digest_source")
}

func TestCompiledPrompt_UnmarshalEmptyManifestDigestFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 1,
		"manifest_id": "x",
		"manifest_digest": "",
		"digest_source": "canonical_snapshot",
		"execution": {
			"messages": [],
			"metadata": {}
		}
	}`
	var cp CompiledPrompt
	err := json.Unmarshal([]byte(raw), &cp)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "manifest_digest")
}

func TestCompiledPrompt_UnmarshalTrailingJSONFails(t *testing.T) {
	t.Parallel()
	raw := `{
		"format_version": 1,
		"manifest_id": "x",
		"manifest_digest": "abc",
		"digest_source": "canonical_snapshot",
		"execution": {
			"messages": [],
			"metadata": {}
		}
	}
	{"extra":1}`
	_, err := unmarshalCompiledPromptJSON([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing JSON")

	var cp CompiledPrompt
	err = (&cp).UnmarshalJSON([]byte(raw))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "trailing JSON")
}

func TestCompiledPrompt_MarshalIncludesFormatVersion(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{NewUserMessage("hi")})
	tplSrc, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleUser, Content: TextContent("{{ .Input.q }}")},
	}, WithMetadata(PromptMetadata{ID: "agent"}))
	require.NoError(t, err)
	cp, err := NewCompiledPromptWithCanonicalSnapshot(exec, "agent", tplSrc)
	require.NoError(t, err)

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	fv, ok := wire["format_version"].(float64)
	require.True(t, ok)
	assert.Equal(t, compiledPromptFormatVersion, int(fv))
}
