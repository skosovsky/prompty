package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalize_SkipsMergeWhenLayerIDDiffers(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "a"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "layer_a"},
		},
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "b"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "layer_b"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 2)
	assert.Equal(t, "layer_a", normalized.Messages[0].Provenance.LayerID)
	assert.Equal(t, "layer_b", normalized.Messages[1].Provenance.LayerID)
}

func TestNormalize_MergesWhenSameLayerIDOrEmpty(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "a"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
		{
			Role:       RoleDeveloper,
			Content:    []ContentPart{TextPart{Text: "b"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 1)
	assert.Equal(t, "same", normalized.Messages[0].Provenance.LayerID)
	assert.Contains(t, mustTextFromParts(t, normalized.Messages[0].Content), "a")
	assert.Contains(t, mustTextFromParts(t, normalized.Messages[0].Content), "b")
}

func TestNormalize_SkipsMergeWhenOneLayerIDEmpty(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "a"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: ""},
		},
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "b"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "base_system"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 2)
}

func TestNormalize_MergeSystemMessages_FirstWinsMetadata(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "a"}},
			Metadata:   MustJSONDocumentFromMap(map[string]any{"keep": true}),
			LayerKind:  LayerKind("policy"),
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "b"}},
			Metadata:   MustJSONDocumentFromMap(map[string]any{"drop": true}),
			LayerKind:  LayerKind("rules"),
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 1)
	doc, err := JSONDocumentAsMap(normalized.Messages[0].Metadata)
	require.NoError(t, err)
	assert.True(t, doc["keep"].(bool))
	_, hasDrop := doc["drop"]
	assert.False(t, hasDrop)
	assert.Equal(t, LayerKind("policy"), normalized.Messages[0].LayerKind)
}

func TestNormalize_CachePolicyMergeSemantics_PartialNil(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:        RoleSystem,
			Content:     []ContentPart{TextPart{Text: "a"}},
			CachePolicy: &CachePolicy{Type: "ephemeral"},
			Provenance:  &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "b"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 1)
	assert.Nil(t, normalized.Messages[0].CachePolicy)
}

func TestNormalize_CachePolicyMergeSemantics_MatchingTypes(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:        RoleSystem,
			Content:     []ContentPart{TextPart{Text: "a"}},
			CachePolicy: &CachePolicy{Type: "ephemeral"},
			Provenance:  &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
		{
			Role:        RoleSystem,
			Content:     []ContentPart{TextPart{Text: "b"}},
			CachePolicy: &CachePolicy{Type: "ephemeral"},
			Provenance:  &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 1)
	require.NotNil(t, normalized.Messages[0].CachePolicy)
	assert.Equal(t, "ephemeral", normalized.Messages[0].CachePolicy.Type)
}

func TestNormalize_ProvenanceNilVsEmptyLayerID(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:    RoleSystem,
			Content: []ContentPart{TextPart{Text: "a"}},
		},
		{
			Role:       RoleSystem,
			Content:    []ContentPart{TextPart{Text: "b"}},
			Provenance: &MessageProvenance{ManifestID: "m", LayerID: ""},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 1)
}

func TestNormalize_DropsCachePolicyOnMismatch(t *testing.T) {
	t.Parallel()
	exec := NewExecution([]ChatMessage{
		{
			Role:        RoleSystem,
			Content:     []ContentPart{TextPart{Text: "a"}},
			CachePolicy: &CachePolicy{Type: "ephemeral"},
			Provenance:  &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
		{
			Role:        RoleSystem,
			Content:     []ContentPart{TextPart{Text: "b"}},
			CachePolicy: &CachePolicy{Type: "persistent"},
			Provenance:  &MessageProvenance{ManifestID: "m", LayerID: "same"},
		},
	})
	normalized := exec.Normalize()
	require.Len(t, normalized.Messages, 1)
	assert.Nil(t, normalized.Messages[0].CachePolicy)
}
