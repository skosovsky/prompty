package prompty

import (
	"context"
	"fmt"
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
	msgs := []MessageTemplate{{Role: "system", Content: "Hi {{ .name }}"}}
	tpl, err := NewChatPromptTemplate(msgs)
	require.NoError(t, err)
	msgs[0].Content = "mutated"
	assert.Equal(t, "Hi {{ .name }}", tpl.Messages[0].Content)
}

func TestNewChatPromptTemplate_ParseError(t *testing.T) {
	t.Parallel()
	msgs := []MessageTemplate{{Role: "system", Content: "Hi {{ .name }}"}, {Role: "user", Content: "{{ end }}"}}
	_, err := NewChatPromptTemplate(msgs)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateParse)
}

func TestFormatStruct_SimpleVars(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "Hello, {{ .user_name }}!"},
	})
	require.NoError(t, err)
	type Payload struct {
		UserName string `prompt:"user_name"`
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{UserName: "Alice"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Len(t, exec.Messages[0].Content, 1)
	assert.Equal(t, "Hello, Alice!", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_PartialVariables(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "{{ .bot_name }}: {{ .msg }}"},
	}, WithPartialVariables(map[string]any{"bot_name": "Bot", "msg": "default"}))
	require.NoError(t, err)
	type Payload struct {
		Msg string `prompt:"msg"`
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{Msg: "overridden"})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Contains(t, text, "Bot")
	assert.Contains(t, text, "overridden")
}

func TestFormatStruct_MissingRequired(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "user", Content: "{{ .user_name }}"},
	})
	require.NoError(t, err)
	type Payload struct {
		Other string `prompt:"other"`
	}
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, &Payload{Other: "x"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMissingVariable)
	var ve *VariableError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "user_name", ve.Variable)
}

// TestFormatStruct_ManifestRequiredVars ensures manifest-derived RequiredVars (e.g. variables.required) are enforced.
func TestFormatStruct_ManifestRequiredVars(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate(
		[]MessageTemplate{{Role: RoleSystem, Content: "Hi"}},
		WithRequiredVars([]string{"must_have"}),
	)
	require.NoError(t, err)
	type P struct {
		Other string `prompt:"other"`
	}
	_, err = tpl.FormatStruct(context.Background(), &P{Other: "x"})
	require.Error(t, err)
	require.ErrorIs(t, err, ErrMissingVariable)
}

func TestFormatStruct_ReservedToolsKey(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: "Hi"}})
	require.NoError(t, err)
	type PayloadWithTools struct {
		Tools string `prompt:"Tools"` // reserved
	}
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, &PayloadWithTools{Tools: "x"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrReservedVariable)
}

func TestFormatStruct_PointerToPointerPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "Hello, {{ .user_name }}!"},
	})
	require.NoError(t, err)
	type Payload struct {
		UserName string `prompt:"user_name"`
	}
	p := &Payload{UserName: "Alice"}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &p) // pass **Payload
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "Hello, Alice!", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_InvalidPayload(t *testing.T) {
	t.Parallel()
	type NoTags struct {
		X string
	}
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: "Hi"}})
	require.NoError(t, err)
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, &NoTags{X: "y"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func TestFormatStruct_NilPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: "Hi"}})
	require.NoError(t, err)
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidPayload)
}

func TestFormatStruct_NilPointerPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: "Hi {{ .x }}"}})
	require.NoError(t, err)
	type P struct {
		X string `prompt:"x"`
	}
	var p *P // nil pointer
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, p)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
}

func TestFormatStruct_NonStructPayload(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: "Hi"}})
	require.NoError(t, err)
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, 42)
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
	_, err = tpl.FormatStruct(ctx, "string")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidPayload)
}

func TestFormatStruct_CancelledContext(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{{Role: "system", Content: "Hi"}})
	require.NoError(t, err)
	type P struct {
		X string `prompt:"x"`
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = tpl.FormatStruct(ctx, &P{X: "v"})
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFormatStruct_OptionalMessage(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "System"},
		{Role: "user", Content: "{{ .extra }}", Optional: true},
	})
	require.NoError(t, err)
	type Payload struct {
		Extra string `prompt:"extra"`
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{Extra: ""})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "System", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_ChatHistory_Splice(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "You are a helper."},
		{Role: "user", Content: "{{ .query }}"},
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
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{Query: "last", History: history})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 4) // system, history[0], history[1], user
	assert.Equal(t, RoleSystem, exec.Messages[0].Role)
	assert.Equal(t, RoleUser, exec.Messages[1].Role)
	assert.Equal(t, RoleAssistant, exec.Messages[2].Role)
	assert.Equal(t, RoleUser, exec.Messages[3].Role)
	assert.Equal(t, "last", exec.Messages[3].Content[0].(TextPart).Text)
}

func TestFormatStruct_ToolsInjection(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "Tools: {{ render_tools_as_json .Tools }}"},
	}, WithTools([]ToolDefinition{{Name: "foo", Description: "bar", Parameters: nil}}))
	require.NoError(t, err)
	type Payload struct {
		X string `prompt:"x"` // not referenced in template; payload must have at least one prompt tag
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &Payload{})
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	text := exec.Messages[0].Content[0].(TextPart).Text
	assert.Contains(t, text, "foo")
	assert.Contains(t, text, "bar")
}

// failingTokenCounter implements TokenCounter and returns an error for Count (to trigger ErrTemplateRender path).
type failingTokenCounter struct{}

func (failingTokenCounter) Count(string) (int, error) {
	return 0, fmt.Errorf("token counter failure")
}

func TestFormatStruct_ErrTemplateRender(t *testing.T) {
	t.Parallel()
	// Template parses but Execute fails because truncate_tokens uses a TokenCounter that returns error.
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "{{ truncate_tokens .text 10 }}"},
	}, WithTokenCounter(failingTokenCounter{}))
	require.NoError(t, err)
	type P struct {
		Text string `prompt:"text"`
	}
	ctx := context.Background()
	_, err = tpl.FormatStruct(ctx, &P{Text: "hello"})
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrTemplateRender)
}

func TestWithTokenCounter_Nil(t *testing.T) {
	t.Parallel()
	// WithTokenCounter(nil) should fall back to CharFallbackCounter in truncate_tokens.
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "{{ truncate_tokens .text 2 }}"}, // 2 tokens, default ~4 chars/token
	}, WithTokenCounter(nil))
	require.NoError(t, err)
	type P struct {
		Text string `prompt:"text"`
	}
	ctx := context.Background()
	exec, err := tpl.FormatStruct(ctx, &P{Text: "12345678"}) // 8 chars -> 2 tokens, no truncation
	require.NoError(t, err)
	require.Len(t, exec.Messages, 1)
	assert.Equal(t, "12345678", exec.Messages[0].Content[0].(TextPart).Text)
}

func TestFormatStruct_ConcurrentUse(t *testing.T) {
	t.Parallel()
	tpl, err := NewChatPromptTemplate([]MessageTemplate{
		{Role: "system", Content: "{{ .x }}"},
	})
	require.NoError(t, err)
	type P struct {
		X string `prompt:"x"`
	}
	ctx := context.Background()
	const n = 100
	errCh := make(chan error, n)
	for range n {
		go func() {
			exec, err := tpl.FormatStruct(ctx, &P{X: "v"})
			if err != nil {
				errCh <- err
				return
			}
			if len(exec.Messages) != 1 || exec.Messages[0].Content[0].(TextPart).Text != "v" {
				errCh <- fmt.Errorf("unexpected result")
				return
			}
			errCh <- nil
		}()
	}
	for range n {
		require.NoError(t, <-errCh)
	}
}
