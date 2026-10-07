package openai

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestChatDeveloperAndPDFWireMapping(t *testing.T) {
	t.Parallel()
	// Arrange: native SDK serialization must preserve policy roles and inline PDF metadata.
	pdf := []byte("%PDF-fixture")
	execution := &prompty.PromptExecution{Messages: []prompty.ChatMessage{
		{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "system"}}},
		{Role: prompty.RoleDeveloper, Content: []prompty.ContentPart{prompty.TextPart{Text: "developer"}}},
		{
			Role:    prompty.RoleUser,
			Content: []prompty.ContentPart{prompty.MediaPart{MIMEType: "application/pdf", Data: pdf}},
		},
	}}
	before := execution.Clone()
	// Act.
	params, err := New().Translate(execution)
	require.NoError(t, err)
	encoded, err := json.Marshal(params)
	require.NoError(t, err)
	var body struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(encoded, &body))
	// Assert.
	require.Equal(t, "system", body.Messages[0].Role)
	require.Equal(t, "developer", body.Messages[1].Role)
	require.Equal(t, before.Messages, execution.Messages)
	var parts []struct {
		File struct {
			Filename string `json:"filename"`
			Data     string `json:"file_data"`
		} `json:"file"`
	}
	require.NoError(t, json.Unmarshal(body.Messages[2].Content, &parts))
	require.Equal(t, "input.pdf", parts[0].File.Filename)
	require.Equal(t, "data:application/pdf;base64,"+base64.StdEncoding.EncodeToString(pdf), parts[0].File.Data)
	for _, mime := range []string{"", "application/octet-stream", "text/plain"} {
		_, err = mediaPartToOpenAI(prompty.MediaPart{MIMEType: mime, Data: []byte("data")})
		require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	}
}

func TestChatCheckedHTTPRolesAndPDF(t *testing.T) {
	t.Parallel()
	// Arrange: receive the actual SDK request body, with explicit profile MIME support.
	var calls atomic.Int32
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(request.Body)
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, captureResponseFixture)
	}))
	t.Cleanup(server.Close)
	client := sdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "actual local SDK body",
		InputMIME:  []string{"application/pdf", "text/plain"},
	}
	checked := adapter.NewClient(New(WithClient(&client), WithModel("fixture"), WithProfiles(server.URL, profile)))
	pdf := []byte("%PDF-fixture")
	execution := prompty.NewExecution([]prompty.ChatMessage{
		{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "system"}}},
		{Role: prompty.RoleDeveloper, Content: []prompty.ContentPart{prompty.TextPart{Text: "developer"}}},
		{
			Role: prompty.RoleUser,
			Content: []prompty.ContentPart{
				prompty.MediaPart{MediaType: "document", MIMEType: "application/pdf", Data: pdf},
			},
		},
	})
	before := execution.Clone()
	// Act.
	_, err := checked.Execute(t.Context(), execution)
	// Assert: exact native roles, reversible document and unchanged caller data.
	require.NoError(t, err)
	var received struct {
		Messages []struct {
			Role    string          `json:"role"`
			Content json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &received))
	require.Len(t, received.Messages, 3)
	require.Equal(t, "system", received.Messages[0].Role)
	require.Equal(t, "developer", received.Messages[1].Role)
	var content []struct {
		File struct {
			Filename string `json:"filename"`
			Data     string `json:"file_data"`
		} `json:"file"`
	}
	require.NoError(t, json.Unmarshal(received.Messages[2].Content, &content))
	require.Equal(t, "input.pdf", content[0].File.Filename)
	require.Equal(t, "data:application/pdf;base64,"+base64.StdEncoding.EncodeToString(pdf), content[0].File.Data)
	require.Equal(t, before.Messages, execution.Messages)
	for _, media := range []prompty.MediaPart{
		{MediaType: "document", MIMEType: "text/plain", Data: []byte("unsupported")},
		{MediaType: "document", MIMEType: "application/pdf"},
	} {
		invalid := prompty.NewExecution(
			[]prompty.ChatMessage{{Role: prompty.RoleUser, Content: []prompty.ContentPart{media}}},
		)
		_, err = checked.Execute(t.Context(), invalid)
		require.Error(t, err)
		require.EqualValues(t, 1, calls.Load(), "invalid media never dispatches or uploads")
	}
}
