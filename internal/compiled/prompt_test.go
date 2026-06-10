package compiled_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/compiled"
)

func TestNew_RequiresManifestBytes(t *testing.T) {
	t.Parallel()
	exec := prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hi")})
	_, err := compiled.New(exec, "agent", nil)
	require.ErrorIs(t, err, prompty.ErrManifestBytesRequired)
}

func TestPrompt_RoundTrip(t *testing.T) {
	t.Parallel()
	exec := prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hi")})
	raw := []byte("id: agent\nversion: 1\n")
	cp, err := compiled.New(exec, "agent", raw)
	require.NoError(t, err)

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var restored compiled.Prompt
	require.NoError(t, json.Unmarshal(data, &restored))
	assert.Equal(t, "agent", restored.ManifestID())
	assert.Equal(t, prompty.ManifestDigestSHA256(raw), restored.ManifestDigest())
}

func TestFromRenderPlanCanonicalSnapshot_UsesCanonicalDigest(t *testing.T) {
	t.Parallel()
	tpl, err := prompty.NewChatPromptTemplate([]prompty.MessageTemplate{
		{Role: prompty.RoleUser, Content: prompty.TextContent("{{ .Input.q }}")},
	}, prompty.WithMetadata(prompty.PromptMetadata{ID: "t"}))
	require.NoError(t, err)

	plan, err := prompty.NewRenderPlanFromStruct(tpl, struct {
		Q string `prompt:"q"`
	}{Q: "x"})
	require.NoError(t, err)
	cp, err := compiled.FromRenderPlanCanonicalSnapshot(context.Background(), plan, "t")
	require.NoError(t, err)
	assert.Equal(t, compiled.DigestSourceCanonicalSnapshot, cp.DigestSource())
	assert.NotEmpty(t, cp.ManifestDigest())
}

func TestPrompt_UnmarshalRejectsFormatVersion1(t *testing.T) {
	t.Parallel()
	const v1Wire = `{"format_version":1,"manifest_id":"a","manifest_digest":"d",` +
		`"digest_source":"manifest_bytes","execution":{"messages":[]}}`
	err := json.Unmarshal([]byte(v1Wire), new(compiled.Prompt))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "format_version")
}

func TestPrompt_RoundTripProvenanceAndCachePolicy(t *testing.T) {
	t.Parallel()
	cache := &prompty.CachePolicy{Type: "ephemeral"}
	exec := prompty.NewExecution([]prompty.ChatMessage{{
		Role: prompty.RoleSystem,
		Content: []prompty.ContentPart{
			prompty.TextPart{Text: "hi", CachePolicy: cache},
		},
		CachePolicy: cache,
		Provenance:  &prompty.MessageProvenance{LayerID: "rules", ManifestID: "agent"},
	}})
	raw := []byte("id: agent\n")
	cp, err := compiled.New(exec, "agent", raw)
	require.NoError(t, err)

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var restored compiled.Prompt
	require.NoError(t, json.Unmarshal(data, &restored))
	restoredExec := restored.PromptExecution()
	require.Len(t, restoredExec.Messages, 1)
	require.NotNil(t, restoredExec.Messages[0].Provenance)
	assert.Equal(t, "rules", restoredExec.Messages[0].Provenance.LayerID)
	require.NotNil(t, restoredExec.Messages[0].CachePolicy)
	assert.Equal(t, "ephemeral", restoredExec.Messages[0].CachePolicy.Type)
}

func TestPrompt_ExecutionCloneIsImmutable(t *testing.T) {
	t.Parallel()
	exec := prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hi")})
	cp, err := compiled.New(exec, "agent", []byte("id: agent\n"))
	require.NoError(t, err)

	got := cp.PromptExecution()
	got.Messages[0].Content = []prompty.ContentPart{prompty.TextPart{Text: "mutated"}}
	assert.Equal(t, "hi", cp.PromptExecution().Messages[0].Content[0].(prompty.TextPart).Text)
}

func TestPrompt_MarshalIncludesFormatVersion(t *testing.T) {
	t.Parallel()
	exec := prompty.NewExecution([]prompty.ChatMessage{prompty.NewUserMessage("hi")})
	tpl, err := prompty.NewChatPromptTemplate([]prompty.MessageTemplate{
		{Role: prompty.RoleUser, Content: prompty.TextContent("{{ .Input.q }}")},
	}, prompty.WithMetadata(prompty.PromptMetadata{ID: "agent"}))
	require.NoError(t, err)
	cp, err := compiled.NewWithCanonicalSnapshot(exec, "agent", tpl)
	require.NoError(t, err)

	data, err := json.Marshal(cp)
	require.NoError(t, err)

	var wire map[string]any
	require.NoError(t, json.Unmarshal(data, &wire))
	assert.InDelta(t, 2, wire["format_version"], 0)
}
