package prompty

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClonePromptExecution_DeepCopy(t *testing.T) {
	t.Parallel()

	exec := &PromptExecution{
		Messages: []ChatMessage{
			{
				Role: RoleUser,
				Content: []ContentPart{
					&MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte("img")},
					&ToolResultPart{
						ToolCallID: "tool-1",
						Name:       "lookup",
						Content: []ContentPart{
							&TextPart{Text: "ok"},
						},
						IsError: false,
					},
				},
				Metadata: mustJSONDocument(map[string]any{
					"nested": map[string]any{
						"items": []any{"a", map[string]any{"x": "y"}},
					},
				}),
			},
		},
		Tools: []ToolDefinition{
			{
				Name: "lookup",
				Parameters: mustJSONDocument(map[string]any{
					"properties": map[string]any{
						"city": map[string]any{"type": "string"},
					},
				}),
			},
		},
		ModelOptions: &ModelOptions{
			Stop: []string{"STOP"},
			ProviderSettings: mustJSONDocument(map[string]any{
				"nested": map[string]any{
					"values": []any{"one", "two"},
				},
			}),
		},
		Metadata: PromptMetadata{
			ID:     "original",
			Tags:   []string{"tag-1"},
			Extras: mustJSONDocument(map[string]any{"trace": map[string]any{"env": "dev"}}),
		},
		ResponseFormat: &SchemaDefinition{
			Name: "schema",
			Schema: mustJSONDocument(map[string]any{
				"properties": map[string]any{
					"answer": map[string]any{"type": "string"},
				},
			}),
		},
	}

	clone := clonePromptExecution(exec)
	require.NotNil(t, clone)

	mustJSONDocumentMap(clone.Messages[0].Metadata)["nested"].(map[string]any)["items"].([]any)[0] = "changed"
	clone.Messages[0].Content[0].(*MediaPart).Data[0] = 'X'
	clone.Messages[0].Content[1].(*ToolResultPart).Content[0].(*TextPart).Text = "changed"
	mustJSONDocumentMap(clone.Tools[0].Parameters)["properties"].(map[string]any)["city"].(map[string]any)["type"] = "number"
	clone.ModelOptions.Stop[0] = "END"
	mustJSONDocumentMap(clone.ModelOptions.ProviderSettings)["nested"].(map[string]any)["values"].([]any)[1] = "changed"
	clone.Metadata.Tags[0] = "tag-2"
	mustJSONDocumentMap(clone.Metadata.Extras)["trace"].(map[string]any)["env"] = "prod"
	mustJSONDocumentMap(clone.ResponseFormat.Schema)["properties"].(map[string]any)["answer"].(map[string]any)["type"] = "integer"

	assert.Equal(t, "a", mustJSONDocumentMap(exec.Messages[0].Metadata)["nested"].(map[string]any)["items"].([]any)[0])
	assert.Equal(t, byte('i'), exec.Messages[0].Content[0].(*MediaPart).Data[0])
	assert.Equal(t, "ok", exec.Messages[0].Content[1].(*ToolResultPart).Content[0].(*TextPart).Text)
	assert.Equal(
		t,
		"string",
		mustJSONDocumentMap(exec.Tools[0].Parameters)["properties"].(map[string]any)["city"].(map[string]any)["type"],
	)
	assert.Equal(t, "STOP", exec.ModelOptions.Stop[0])
	assert.Equal(
		t,
		"two",
		mustJSONDocumentMap(exec.ModelOptions.ProviderSettings)["nested"].(map[string]any)["values"].([]any)[1],
	)
	assert.Equal(t, "tag-1", exec.Metadata.Tags[0])
	assert.Equal(t, "dev", mustJSONDocumentMap(exec.Metadata.Extras)["trace"].(map[string]any)["env"])
	assert.Equal(
		t,
		"string",
		mustJSONDocumentMap(exec.ResponseFormat.Schema)["properties"].(map[string]any)["answer"].(map[string]any)["type"],
	)
}
