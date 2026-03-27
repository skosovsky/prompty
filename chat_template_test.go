package prompty

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

func TestNewChatPromptTemplate_DefensiveCopy(t *testing.T) {
	t.Parallel()
	msgs := []MessageTemplate{{Role: "system", Content: TextContent("Hi {{ .name }}")}}
	tpl, err := NewChatPromptTemplate(msgs)
	require.NoError(t, err)
	msgs[0].Content = []TemplatePart{{Type: "text", Text: "mutated"}}
	require.Len(t, tpl.Messages[0].Content, 1)
	assert.Equal(t, "Hi {{ .name }}", tpl.Messages[0].Content[0].Text)
}

func TestNewChatPromptTemplate_DefensiveCopy_NestedState(t *testing.T) {
	t.Parallel()
	msgs := []MessageTemplate{
		{
			Role:     "system",
			Content:  TextContent("Hi"),
			Metadata: map[string]any{"nested": map[string]any{"env": "dev"}},
		},
	}
	tools := []ToolDefinition{
		{
			Name:        "lookup",
			Description: "Lookup data",
			Parameters: map[string]any{
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
			},
		},
	}
	meta := PromptMetadata{
		ID:     "tpl",
		Extras: map[string]any{"trace": map[string]any{"env": "dev"}},
	}
	tpl, err := NewChatPromptTemplate(msgs, WithTools(tools), WithMetadata(meta))
	require.NoError(t, err)

	msgs[0].Metadata["nested"].(map[string]any)["env"] = "prod"
	tools[0].Parameters["properties"].(map[string]any)["city"].(map[string]any)["type"] = "number"
	meta.Extras["trace"].(map[string]any)["env"] = "prod"

	assert.Equal(t, "dev", tpl.Messages[0].Metadata["nested"].(map[string]any)["env"])
	assert.Equal(t, "string", tpl.Tools[0].Parameters["properties"].(map[string]any)["city"].(map[string]any)["type"])
	assert.Equal(t, "dev", tpl.Metadata.Extras["trace"].(map[string]any)["env"])
}

func TestNewChatPromptTemplate_DefensiveCopy_Schemas(t *testing.T) {
	t.Parallel()
	responseSchema := &SchemaDefinition{
		Name: "response",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"answer": map[string]any{"type": "string"},
			},
		},
	}
	inputSchema := &SchemaDefinition{
		Name: "input",
		Schema: map[string]any{
			"type": "object",
			"properties": map[string]any{
				"question": map[string]any{"type": "string"},
			},
		},
	}

	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: "system", Content: TextContent("Hi")}},
		WithResponseFormat(responseSchema),
		WithInputSchema(inputSchema),
	)
	require.NoError(t, err)

	responseSchema.Name = "mutated-response"
	responseSchema.Schema["properties"].(map[string]any)["answer"].(map[string]any)["type"] = "number"
	inputSchema.Name = "mutated-input"
	inputSchema.Schema["properties"].(map[string]any)["question"].(map[string]any)["type"] = "number"

	require.NotNil(t, tpl.ResponseFormat)
	require.NotNil(t, tpl.InputSchema)
	assert.Equal(t, "response", tpl.ResponseFormat.Name)
	assert.Equal(
		t,
		"string",
		tpl.ResponseFormat.Schema["properties"].(map[string]any)["answer"].(map[string]any)["type"],
	)
	assert.Equal(t, "input", tpl.InputSchema.Name)
	assert.Equal(
		t,
		"string",
		tpl.InputSchema.Schema["properties"].(map[string]any)["question"].(map[string]any)["type"],
	)
}

func TestNewChatPromptTemplate_ParseError(t *testing.T) {
	t.Parallel()
	msgs := []MessageTemplate{
		{Role: "system", Content: TextContent("Hi {{ .name }}")},
		{Role: "user", Content: TextContent("{{ end }}")},
	}
	_, err := NewChatPromptTemplate(msgs)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateParse)
}

func TestFormatStruct_SimpleVars(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("Hello, {{ .user_name }}!")},
	})
	require.NoError(t, err)
	type Payload struct {
		UserName string `prompt:"user_name"`
	}
	exec, err := tpl.FormatStruct(&Payload{UserName: "Alice"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Len(t, exec.Messages[0].Content, 1)
	assert.Equal(t, "Hello, Alice!", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_MediaPart(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: []TemplatePart{
				{Type: "text", Text: "Analyze this:"},
				{
					Type:      "media",
					MediaType: "{{ .kind }}",
					MIMEType:  "{{ .mime }}",
					URL:       "{{ .url }}",
				},
			},
		},
	})
	require.NoError(t, err)
	type Payload struct {
		Kind string `prompt:"kind"`
		MIME string `prompt:"mime"`
		URL  string `prompt:"url"`
	}
	exec, err := tpl.FormatStruct(&Payload{
		Kind: "document",
		MIME: "application/pdf",
		URL:  "https://example.com/report.pdf",
	})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	require.Len(t, exec.Messages[0].Content, 2)

	text := exec.Messages[0].Content[0].(TextPart)
	assert.Equal(t, "Analyze this:", text.Text)

	media := exec.Messages[0].Content[1].(MediaPart)
	assert.Equal(t, "document", media.MediaType)
	assert.Equal(t, "application/pdf", media.MIMEType)
	assert.Equal(t, "https://example.com/report.pdf", media.URL)
}

func TestFormatStruct_MediaPart_InferTypeFromMIME(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: []TemplatePart{
				{Type: "text", Text: "Analyze this:"},
				{
					Type:     "media",
					MIMEType: "{{ .mime }}",
					URL:      "{{ .url }}",
				},
			},
		},
	})
	require.NoError(t, err)
	type Payload struct {
		MIME string `prompt:"mime"`
		URL  string `prompt:"url"`
	}
	exec, err := tpl.FormatStruct(&Payload{
		MIME: "application/pdf",
		URL:  "https://example.com/report.pdf",
	})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	require.Len(t, exec.Messages[0].Content, 2)

	media := exec.Messages[0].Content[1].(MediaPart)
	assert.Equal(t, "document", media.MediaType)
	assert.Equal(t, "application/pdf", media.MIMEType)
	assert.Equal(t, "https://example.com/report.pdf", media.URL)
}

func TestFormatStruct_MediaPart_MissingTypeAndUnknownMIMEReturnsError(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: []TemplatePart{
				{
					Type: "media",
					URL:  "{{ .url }}",
				},
			},
		},
	})
	require.NoError(t, err)
	type Payload struct {
		URL string `prompt:"url"`
	}
	_, err = tpl.FormatStruct(&Payload{URL: "https://example.com/file.bin"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTemplateRender)
	assert.Contains(t, err.Error(), "media_type is required")
}

func TestFormatStruct_PartialVariables(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("{{ .bot_name }}: {{ .msg }}")},
	}, WithPartialVariables(map[string]any{"bot_name": "Bot", "msg": "default"}))
	require.NoError(t, err)
	type Payload struct {
		Msg string `prompt:"msg"`
	}
	exec, err := tpl.FormatStruct(&Payload{Msg: "overridden"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Contains(t, text, "Bot")
	assert.Contains(t, text, "overridden")
}

func TestFormatStruct_MissingRequired(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "user", Content: TextContent("{{ .user_name }}")},
	})
	require.NoError(t, err)
	type Payload struct {
		Other string `prompt:"other"`
	}
	_, err = tpl.FormatStruct(&Payload{Other: "x"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMissingVariable)
	var ve *VariableError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "user_name", ve.Variable)
}

func TestValidateVariables_MediaPart(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: []TemplatePart{
				{Type: "media", MediaType: "{{ .kind }}", MIMEType: "{{ .mime }}", URL: "{{ .url }}"},
			},
		},
	})
	require.NoError(t, err)
	err = tpl.ValidateVariables(map[string]any{
		"kind": "image",
		"mime": "image/png",
		"url":  "https://example.com/img.png",
	})
	require.NoError(t, err)

	err = tpl.ValidateVariables(map[string]any{
		"kind": "image",
		"mime": "image/png",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTemplateRender)
}

func TestValidateVariables_MediaPart_MissingTypeAndUnknownMIME(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{
			Role: RoleUser,
			Content: []TemplatePart{
				{Type: "media", MIMEType: "{{ .mime }}", URL: "{{ .url }}"},
			},
		},
	})
	require.NoError(t, err)
	err = tpl.ValidateVariables(map[string]any{
		"mime": "application/octet-stream",
		"url":  "https://example.com/file.bin",
	})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrTemplateRender)
	assert.Contains(t, err.Error(), "media_type is required")
}

// TestFormatStruct_ManifestRequiredVars ensures manifest-derived RequiredVars (e.g. input_schema.required) are enforced.
func TestFormatStruct_ManifestRequiredVars(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleSystem, Content: TextContent("Hi")}},
		WithRequiredVars([]string{"must_have"}),
	)
	require.NoError(t, err)
	type P struct {
		Other string `prompt:"other"`
	}
	_, err = tpl.FormatStruct(&P{Other: "x"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMissingVariable)
}

func TestFormatStruct_ReservedToolsKey(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: TextContent("Hi")}})
	require.NoError(t, err)
	type PayloadWithTools struct {
		Tools string `prompt:"Tools"` // reserved
	}
	_, err = tpl.FormatStruct(&PayloadWithTools{Tools: "x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReservedVariable)
}

// TestFormatStruct_ResponseFormatClone verifies that mutating exec.ResponseFormat does not affect the template.
// FormatStruct must return an execution that is an independent snapshot (thread-safety, no shared pointers).
func TestFormatStruct_ResponseFormatClone(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "remove_me": true}
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: "system", Content: TextContent("Hi")}},
		WithResponseFormat(&SchemaDefinition{Name: "original", Description: "desc", Schema: schema}),
	)
	require.NoError(t, err)
	type emptyPayload struct {
		X string `prompt:"x"` // unused by template; needed for getPayloadFields
	}
	exec, err := tpl.FormatStruct(&emptyPayload{})
	require.NoError(t, err)
	require.NotNil(t, exec.ResponseFormat)
	require.NotNil(t, tpl.ResponseFormat)

	origName := tpl.ResponseFormat.Name
	_, origHasKey := tpl.ResponseFormat.Schema["remove_me"]

	// Mutate execution's ResponseFormat (e.g. middleware normalizing schema).
	exec.ResponseFormat.Name = "other"
	delete(exec.ResponseFormat.Schema, "remove_me")

	// Template must be unchanged.
	assert.Equal(t, origName, tpl.ResponseFormat.Name, "template ResponseFormat.Name must not be mutated")
	assert.True(t, origHasKey, "template ResponseFormat.Schema must not be mutated")
	_, stillHasKey := tpl.ResponseFormat.Schema["remove_me"]
	assert.True(t, stillHasKey, "template ResponseFormat.Schema must not share map with execution")
}

// TestCloneTemplate_ResponseFormatDoesNotMutateOriginal verifies that mutating the clone's ResponseFormat
// does not affect the original template. Registries rely on this so callers cannot mutate the cached template.
func TestCloneTemplate_ResponseFormatDoesNotMutateOriginal(t *testing.T) {
	t.Parallel()
	schema := map[string]any{"type": "object", "key": "v"}
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: "system", Content: TextContent("Hi")}},
		WithResponseFormat(&SchemaDefinition{Name: "orig", Description: "d", Schema: schema}),
	)
	require.NoError(t, err)
	clone := CloneTemplate(tpl)
	require.NotNil(t, clone)
	require.NotNil(t, clone.ResponseFormat)

	origName := tpl.ResponseFormat.Name
	_, origHasKey := tpl.ResponseFormat.Schema["key"]

	// Mutate clone's ResponseFormat (and its Schema).
	clone.ResponseFormat.Name = "mutated"
	delete(clone.ResponseFormat.Schema, "key")
	clone.ResponseFormat.Schema["new"] = "x"

	// Original template must be unchanged.
	assert.Equal(t, origName, tpl.ResponseFormat.Name, "original ResponseFormat.Name must not be mutated")
	assert.True(t, origHasKey, "original ResponseFormat.Schema must not be mutated")
	_, stillHasKey := tpl.ResponseFormat.Schema["key"]
	assert.True(t, stillHasKey)
	_, hasNew := tpl.ResponseFormat.Schema["new"]
	assert.False(t, hasNew, "original Schema must not share map with clone")
}

func TestCloneTemplate_SchemaDefinitionsAreIndependent(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: "system", Content: TextContent("Hi")}},
		WithResponseFormat(&SchemaDefinition{
			Name: "response",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"answer": map[string]any{"type": "string"},
				},
			},
		}),
		WithInputSchema(&SchemaDefinition{
			Name: "input",
			Schema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"question": map[string]any{"type": "string"},
				},
			},
		}),
	)
	require.NoError(t, err)

	clone := CloneTemplate(tpl)
	require.NotNil(t, clone)
	require.NotNil(t, clone.ResponseFormat)
	require.NotNil(t, clone.InputSchema)

	clone.ResponseFormat.Name = "mutated-response"
	clone.ResponseFormat.Schema["properties"].(map[string]any)["answer"].(map[string]any)["type"] = "number"
	clone.InputSchema.Name = "mutated-input"
	clone.InputSchema.Schema["properties"].(map[string]any)["question"].(map[string]any)["type"] = "number"

	assert.Equal(t, "response", tpl.ResponseFormat.Name)
	assert.Equal(
		t,
		"string",
		tpl.ResponseFormat.Schema["properties"].(map[string]any)["answer"].(map[string]any)["type"],
	)
	assert.Equal(t, "input", tpl.InputSchema.Name)
	assert.Equal(
		t,
		"string",
		tpl.InputSchema.Schema["properties"].(map[string]any)["question"].(map[string]any)["type"],
	)
}

func TestCloneTemplate_DoesNotMutateOriginalNestedState(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{
			Role:     "system",
			Content:  TextContent("Hi"),
			Metadata: map[string]any{"nested": map[string]any{"env": "dev"}},
		}},
		WithTools([]ToolDefinition{{
			Name:        "lookup",
			Description: "Lookup data",
			Parameters: map[string]any{
				"properties": map[string]any{
					"city": map[string]any{"type": "string"},
				},
			},
		}}),
		WithMetadata(PromptMetadata{
			ID:     "tpl",
			Extras: map[string]any{"trace": map[string]any{"env": "dev"}},
		}),
	)
	require.NoError(t, err)

	clone := CloneTemplate(tpl)
	require.NotNil(t, clone)

	clone.Messages[0].Metadata["nested"].(map[string]any)["env"] = "prod"
	clone.Tools[0].Parameters["properties"].(map[string]any)["city"].(map[string]any)["type"] = "number"
	clone.Metadata.Extras["trace"].(map[string]any)["env"] = "prod"

	assert.Equal(t, "dev", tpl.Messages[0].Metadata["nested"].(map[string]any)["env"])
	assert.Equal(t, "string", tpl.Tools[0].Parameters["properties"].(map[string]any)["city"].(map[string]any)["type"])
	assert.Equal(t, "dev", tpl.Metadata.Extras["trace"].(map[string]any)["env"])
}

func TestFormatStruct_PointerToPointerPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("Hello, {{ .user_name }}!")},
	})
	require.NoError(t, err)
	type Payload struct {
		UserName string `prompt:"user_name"`
	}
	p := &Payload{UserName: "Alice"}
	exec, err := tpl.FormatStruct(&p) // pass **Payload
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "Hello, Alice!", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_InvalidPayload(t *testing.T) {
	t.Parallel()
	type NoTags struct {
		X string
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: TextContent("Hi")}})
	require.NoError(t, err)
	_, err = tpl.FormatStruct(&NoTags{X: "y"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

// TestFormatStruct_JsonTagFallback ensures payload uses json tag when prompt tag is missing (strings.Split(tag, ",")[0]).
func TestFormatStruct_JsonTagFallback(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: RoleSystem, Content: TextContent("Hello, {{ .user_name }}!")},
	})
	require.NoError(t, err)
	type Payload struct {
		UserName string `json:"user_name,omitempty"` // no prompt tag; fallback to json first part
	}
	exec, err := tpl.FormatStruct(&Payload{UserName: "Bob"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "Hello, Bob!", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_NilPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: TextContent("Hi")}})
	require.NoError(t, err)
	_, err = tpl.FormatStruct(nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func TestFormatStruct_NilPointerPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: TextContent("Hi {{ .x }}")}})
	require.NoError(t, err)
	type P struct {
		X string `prompt:"x"`
	}
	var p *P // nil pointer
	_, err = tpl.FormatStruct(p)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
}

func TestFormatStruct_NonStructPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: TextContent("Hi")}})
	require.NoError(t, err)
	_, err = tpl.FormatStruct(42)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
	_, err = tpl.FormatStruct("string")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
}

func TestValidateVariables_Ok(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("Hello, {{ .user_name }}!")},
	})
	require.NoError(t, err)
	err = tpl.ValidateVariables(map[string]any{"user_name": "Alice"})
	require.NoError(t, err)
}

func TestValidateVariables_MissingVar(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("Hello, {{ .user_name }}!")},
	})
	require.NoError(t, err)
	err = tpl.ValidateVariables(map[string]any{})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateRender)
}

func TestFormatStruct_OptionalMessage(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("System")},
		{Role: "user", Content: TextContent("{{ .extra }}"), Optional: true},
	})
	require.NoError(t, err)
	type Payload struct {
		Extra string `prompt:"extra"`
	}
	exec, err := tpl.FormatStruct(&Payload{Extra: ""})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "System", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_ChatHistory_Splice(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("You are a helper.")},
		{Role: "user", Content: TextContent("{{ .query }}")},
	})
	require.NoError(t, err)
	history := []ChatMessage{
		{Role: "user", Content: []ContentPart{TextPart{Text: "first"}}},
		{Role: "assistant", Content: []ContentPart{TextPart{Text: "second"}}},
	}
	type Payload struct {
		Query   string        `prompt:"query"`
		History []ChatMessage `prompt:"history"`
	}
	exec, err := tpl.FormatStruct(&Payload{Query: "last", History: history})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 4) // system, history[0], history[1], user
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, RoleUser, exec.Messages[1].Role)
	assert.Equal(t, RoleAssistant, exec.Messages[2].Role)
	assert.Equal(t, RoleUser, exec.Messages[3].Role)
	assert.Equal(t, "last", exec.Messages[3].Content[0].(TextPart).Text)
}

// TestFormatStruct_ChatHistory_SpliceAfterDeveloper ensures history is inserted after all system/developer anchors.
func TestFormatStruct_ChatHistory_SpliceAfterDeveloper(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("System.")},
		{Role: "developer", Content: TextContent("Developer.")},
		{Role: "user", Content: TextContent("{{ .query }}")},
	})
	require.NoError(t, err)
	history := []ChatMessage{
		{Role: RoleUser, Content: []ContentPart{TextPart{Text: "hist_user"}}},
	}
	type Payload struct {
		Query   string        `prompt:"query"`
		History []ChatMessage `prompt:"history"`
	}
	exec, err := tpl.FormatStruct(&Payload{Query: "last", History: history})
	require.NoError(t, err)
	// Expected: system, developer, history[0], user
	require.Len(t, exec.Messages, 4)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, RoleDeveloper, exec.Messages[1].Role)
	assert.Equal(t, RoleUser, exec.Messages[2].Role)
	assert.Equal(t, "hist_user", exec.Messages[2].Content[0].(TextPart).Text)
	assert.Equal(t, RoleUser, exec.Messages[3].Role)
	assert.Equal(t, "last", exec.Messages[3].Content[0].(TextPart).Text)
}

func TestFormatStruct_ToolsInjection(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("Tools: {{ render_tools_as_json .Tools }}")},
	}, WithTools([]ToolDefinition{{Name: "foo", Description: "bar", Parameters: nil}}))
	require.NoError(t, err)
	type Payload struct {
		X string `prompt:"x"` // not referenced in template; payload must have at least one prompt tag
	}
	exec, err := tpl.FormatStruct(&Payload{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Contains(t, text, "foo")
	assert.Contains(t, text, "bar")
}

// failingTokenCounter implements TokenCounter and returns an error for Count (to trigger ErrTemplateRender path).
type failingTokenCounter struct{}

func (failingTokenCounter) Count(string) (int, error) {
	return 0, errors.New("token counter failure")
}

func (failingTokenCounter) CountMessage(ChatMessage) (int, error) {
	return 0, errors.New("token counter failure")
}

func TestFormatStruct_ErrTemplateRender(t *testing.T) {
	t.Parallel()
	// Template parses but Execute fails because truncate_tokens uses a TokenCounter that returns error.
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("{{ truncate_tokens .text 10 }}")},
	}, WithTokenCounter(failingTokenCounter{}))
	require.NoError(t, err)
	type P struct {
		Text string `prompt:"text"`
	}
	_, err = tpl.FormatStruct(&P{Text: "hello"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateRender)
}

func TestWithTokenCounter_Nil(t *testing.T) {
	t.Parallel()
	// WithTokenCounter(nil) should fall back to CharFallbackCounter in truncate_tokens.
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("{{ truncate_tokens .text 2 }}")}, // 2 tokens, default ~4 chars/token
	}, WithTokenCounter(nil))
	require.NoError(t, err)
	type P struct {
		Text string `prompt:"text"`
	}
	exec, err := tpl.FormatStruct(&P{Text: "12345678"}) // 8 chars -> 2 tokens, no truncation
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "12345678", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestNewChatPromptTemplate_WithPartialsGlob(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "partials"), 0755))
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(dir, "partials", "safety.tmpl"),
			[]byte(`{{ define "safety" }}Never give medical advice.{{ end }}`),
			0600,
		),
	)
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{
			{Role: RoleSystem, Content: TextContent("You are a doctor.\n{{ template \"safety\" }}")},
			{Role: RoleUser, Content: TextContent("Hi")},
		},
		WithPartialsGlob(filepath.Join(dir, "partials", "*.tmpl")),
	)
	require.NoError(t, err)
	require.NotNil(t, tpl)
	exec, err := tpl.FormatStruct(&struct {
		X string `json:"x"`
	}{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 2)
	require.Len(t, exec.Messages[0].Content, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Contains(t, text, "Never give medical advice.")
	assert.Contains(t, text, "You are a doctor.")
}

func TestFormatStruct_ConcurrentUse(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: TextContent("{{ .x }}")},
	})
	require.NoError(t, err)
	type P struct {
		X string `prompt:"x"`
	}
	const n = 100
	errCh := make(chan error, n)
	for range n {
		go func() {
			exec, err := tpl.FormatStruct(&P{X: "v"})
			if err != nil {
				errCh <- err
				return
			}
			if len(exec.Messages) != 1 || exec.Messages[0].Content[0].(TextPart).Text != "v" {
				errCh <- errors.New("unexpected result")
				return
			}
			errCh <- nil
		}()
	}
	for range n {
		require.NoError(t, <-errCh)
	}
}

type mockFetcher struct {
	data []byte
	mime string
	err  error
}

func (m mockFetcher) Fetch(_ context.Context, _ string) ([]byte, string, error) {
	if m.err != nil {
		return nil, "", m.err
	}
	return m.data, m.mime, nil
}

type typedNilFetcher struct{}

func (f *typedNilFetcher) Fetch(_ context.Context, _ string) ([]byte, string, error) {
	if f == nil {
		panic("typed nil fetcher called")
	}
	return nil, "", nil
}

func TestPromptExecution_ResolvedMedia(t *testing.T) {
	t.Parallel()
	imageBytes := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a} // PNG magic
	fetcher := mockFetcher{data: imageBytes, mime: "image/png"}

	exec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "image", URL: "https://example.com/img.png"},
			}},
		},
	}
	resolved, err := exec.ResolvedMedia(context.Background(), fetcher)
	require.NoError(t, err)
	require.NotNil(t, resolved)

	origPart := exec.Messages[0].Content[0].(MediaPart)
	assert.Empty(t, origPart.Data)
	assert.Empty(t, origPart.MIMEType)

	part := resolved.Messages[0].Content[0].(MediaPart)
	assert.Equal(t, imageBytes, part.Data)
	assert.Equal(t, "image/png", part.MIMEType)
}

func TestPromptExecution_ResolvedMedia_GenericMedia(t *testing.T) {
	t.Parallel()
	audioExec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "audio", URL: "https://example.com/audio.mp3"},
			}},
		},
	}
	audioResolved, err := audioExec.ResolvedMedia(context.Background(), mockFetcher{
		data: []byte{0x01, 0x02, 0x03},
		mime: "audio/mpeg",
	})
	require.NoError(t, err)
	require.NotNil(t, audioResolved)
	audio := audioResolved.Messages[0].Content[0].(MediaPart)
	assert.Equal(t, "audio", audio.MediaType)
	assert.Equal(t, "audio/mpeg", audio.MIMEType)
	assert.Equal(t, []byte{0x01, 0x02, 0x03}, audio.Data)

	docExec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "document", URL: "https://example.com/file.pdf"},
			}},
		},
	}
	docResolved, err := docExec.ResolvedMedia(context.Background(), mockFetcher{
		data: []byte("%PDF-1.7"),
		mime: "application/pdf",
	})
	require.NoError(t, err)
	require.NotNil(t, docResolved)
	doc := docResolved.Messages[0].Content[0].(MediaPart)
	assert.Equal(t, "document", doc.MediaType)
	assert.Equal(t, "application/pdf", doc.MIMEType)
	assert.Equal(t, []byte("%PDF-1.7"), doc.Data)
}

func TestPromptExecution_ResolvedMedia_NilFetcherReturnsErrNoFetcher(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "image", URL: "https://example.com/image.png"},
			}},
		},
	}
	resolved, err := exec.ResolvedMedia(context.Background(), nil)
	require.Error(t, err)
	assert.Nil(t, resolved)
	require.ErrorIs(t, err, ErrNoFetcher)

	part := exec.Messages[0].Content[0].(MediaPart)
	assert.Empty(t, part.Data)
	assert.Empty(t, part.MIMEType)
}

func TestPromptExecution_ResolvedMedia_NilFetcherOnNoOpPath(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "image", URL: "https://example.com/image.png", Data: []byte("already-resolved")},
			}},
		},
	}
	resolved, err := exec.ResolvedMedia(context.Background(), nil)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.NotSame(t, exec, resolved)
	assert.Equal(t, []byte("already-resolved"), resolved.Messages[0].Content[0].(MediaPart).Data)
}

func TestPromptExecution_ResolvedMedia_TypedNilFetcherReturnsErrNoFetcher(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "image", URL: "https://example.com/image.png"},
			}},
		},
	}
	var fetcher *typedNilFetcher
	resolved, err := exec.ResolvedMedia(context.Background(), fetcher)
	require.Error(t, err)
	assert.Nil(t, resolved)
	require.ErrorIs(t, err, ErrNoFetcher)
}

func TestPromptExecution_ResolvedMedia_TypedNilFetcherOnNoOpPath(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{Role: RoleUser, Content: []ContentPart{
				MediaPart{MediaType: "image", URL: "https://example.com/image.png", Data: []byte("already-resolved")},
			}},
		},
	}
	var fetcher *typedNilFetcher
	resolved, err := exec.ResolvedMedia(context.Background(), fetcher)
	require.NoError(t, err)
	require.NotNil(t, resolved)
	assert.Equal(t, []byte("already-resolved"), resolved.Messages[0].Content[0].(MediaPart).Data)
}

func TestPromptExecution_Normalize(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{
				Role:         RoleSystem,
				Content:      []ContentPart{TextPart{Text: "First system."}},
				CacheControl: &CacheControl{Type: "ephemeral"},
			},
			{
				Role:         RoleSystem,
				Content:      []ContentPart{TextPart{Text: "Second system."}},
				CacheControl: &CacheControl{Type: "ephemeral"},
			},
			{Role: RoleUser, Content: []ContentPart{TextPart{Text: "User query"}}},
		},
	}
	out := exec.Normalize()
	require.Len(t, out.Messages, 2, "two consecutive system messages must merge into one")
	assert.Equal(t, RoleSystem, out.Messages[0].Role)
	assert.Equal(t, RoleUser, out.Messages[1].Role)
	require.Len(t, out.Messages[0].Content, 3)
	assert.Equal(t, "First system.", out.Messages[0].Content[0].(TextPart).Text)
	assert.Equal(t, "\n\n", out.Messages[0].Content[1].(TextPart).Text)
	assert.Equal(t, "Second system.", out.Messages[0].Content[2].(TextPart).Text)
	require.NotNil(t, out.Messages[0].CacheControl)
	assert.Equal(t, "ephemeral", out.Messages[0].CacheControl.Type)
	assert.Equal(t, "User query", out.Messages[1].Content[0].(TextPart).Text)
}

func TestPromptExecution_Normalize_CacheControlNilWins(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{
				Role:    RoleSystem,
				Content: []ContentPart{TextPart{Text: "First system."}},
			},
			{
				Role:         RoleDeveloper,
				Content:      []ContentPart{TextPart{Text: "Second system."}},
				CacheControl: &CacheControl{Type: "ephemeral"},
			},
			{
				Role:         RoleDeveloper,
				Content:      []ContentPart{TextPart{Text: "Third system."}},
				CacheControl: &CacheControl{Type: "ephemeral"},
			},
		},
	}
	out := exec.Normalize()
	require.Len(t, out.Messages, 1)
	assert.Nil(t, out.Messages[0].CacheControl)
}

func TestPromptExecution_Normalize_DoesNotAliasSource(t *testing.T) {
	t.Parallel()
	exec := &PromptExecution{
		Messages: []ChatMessage{
			{
				Role: RoleSystem,
				Content: []ContentPart{
					TextPart{Text: "First system."},
					MediaPart{MediaType: "image", URL: "https://example.com/first.png", Data: []byte("one")},
				},
				Metadata: map[string]any{"scope": "first"},
			},
			{
				Role: RoleDeveloper,
				Content: []ContentPart{
					TextPart{Text: "Second system."},
					MediaPart{MediaType: "image", URL: "https://example.com/second.png", Data: []byte("two")},
				},
				Metadata: map[string]any{"scope": "second"},
			},
			{
				Role:     RoleUser,
				Content:  []ContentPart{TextPart{Text: "User query"}},
				Metadata: map[string]any{"origin": "user"},
			},
		},
		Metadata: PromptMetadata{
			Extras: map[string]any{"trace": map[string]any{"env": "dev"}},
		},
	}

	out := exec.Normalize()
	require.Len(t, out.Messages, 2)

	out.Messages[0].Metadata["scope"] = "changed"
	mergedMedia := out.Messages[0].Content[1].(MediaPart)
	mergedMedia.Data[0] = 'X'
	out.Messages[0].Content[1] = mergedMedia

	out.Messages[1].Metadata["origin"] = "changed"
	out.Messages[1].Content[0] = TextPart{Text: "Changed user query"}
	out.Metadata.Extras["trace"].(map[string]any)["env"] = "prod"

	assert.Equal(t, "first", exec.Messages[0].Metadata["scope"])
	assert.Equal(t, byte('o'), exec.Messages[0].Content[1].(MediaPart).Data[0])
	assert.Equal(t, "user", exec.Messages[2].Metadata["origin"])
	assert.Equal(t, "User query", exec.Messages[2].Content[0].(TextPart).Text)
	assert.Equal(t, "dev", exec.Metadata.Extras["trace"].(map[string]any)["env"])
}
