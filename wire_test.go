package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPromptExecutionWire_RoundTrip(t *testing.T) {
	t.Parallel()
	temp := 0.2
	maxTokens := int64(256)
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{
				Role: RoleUser,
				Content: []ContentPart{
					TextPart{Text: "hello"},
					ToolCallPart{ID: "call-1", Name: "lookup", Args: `{"q":"x"}`},
				},
				Metadata: MustJSONDocumentFromMap(map[string]any{"trace": "m1"}),
			},
			{
				Role: RoleTool,
				Content: []ContentPart{
					ToolResultPart{
						ToolCallID: "call-1",
						Name:       "lookup",
						Content:    []ContentPart{TextPart{Text: "result"}},
					},
				},
			},
		},
		Tools: []ToolDefinition{{
			Name:         "lookup",
			Description:  "Lookup data",
			Parameters:   MustJSONDocumentFromMap(map[string]any{"type": "object"}),
			Capabilities: []string{"read"},
		}},
		RequiredTools: []string{"lookup"},
		ForcedTool:    "lookup",
		ModelOptions: &ModelOptions{
			Model:            "model",
			Temperature:      &temp,
			MaxTokens:        &maxTokens,
			ProviderSettings: MustJSONDocumentFromMap(map[string]any{"provider": "value"}),
		},
		Metadata: PromptMetadata{
			ID:     "agent",
			Extras: MustJSONDocumentFromMap(map[string]any{"owner": "team"}),
		},
		ResponseFormat: &SchemaDefinition{
			Name:   "result",
			Schema: MustJSONDocumentFromMap(map[string]any{"type": "object"}),
		},
	}

	wire, err := MarshalExecution(exec)
	require.NoError(t, err)
	data, err := json.Marshal(wire)
	require.NoError(t, err)
	var decoded PromptExecutionWire
	require.NoError(t, json.Unmarshal(data, &decoded))
	restored, err := UnmarshalExecution(decoded)

	require.NoError(t, err)
	assert.Equal(t, exec.RequiredTools, restored.RequiredTools)
	assert.Equal(t, exec.Tools[0].Capabilities, restored.Tools[0].Capabilities)
	assert.JSONEq(t, string(exec.Tools[0].Parameters), string(restored.Tools[0].Parameters))
	assert.JSONEq(t, string(exec.ModelOptions.ProviderSettings), string(restored.ModelOptions.ProviderSettings))
	assert.JSONEq(t, string(exec.ResponseFormat.Schema), string(restored.ResponseFormat.Schema))
	require.Len(t, restored.Messages, 2)
	assert.Equal(t, "hello", restored.Messages[0].Content[0].(TextPart).Text)
	assert.Equal(t, "lookup", restored.Messages[0].Content[1].(ToolCallPart).Name)
}

func TestPromptExecutionWire_MarshalsPointerContentParts(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{{
			Role: RoleTool,
			Content: []ContentPart{
				&ToolResultPart{
					ToolCallID: "call-1",
					Name:       "lookup",
					Content:    []ContentPart{&TextPart{Text: "ok"}},
				},
			},
		}},
	}

	wire, err := MarshalExecution(exec)
	require.NoError(t, err)
	restored, err := UnmarshalExecution(wire)

	require.NoError(t, err)
	require.Len(t, restored.Messages, 1)
	result, ok := restored.Messages[0].Content[0].(ToolResultPart)
	require.True(t, ok)
	require.Len(t, result.Content, 1)
	assert.Equal(t, "ok", result.Content[0].(TextPart).Text)
}

func TestPromptExecutionWire_RejectsInvalidContentUnion(t *testing.T) {
	t.Parallel()
	wire := PromptExecutionWire{
		Messages: []MessageWire{{
			Role:    RoleUser,
			Content: []ContentPartWire{{Type: "unknown"}},
		}},
	}

	_, err := UnmarshalExecution(wire)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported content part wire type")
}
