package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesFunctionResultsSDKOrderedMedia(t *testing.T) {
	t.Parallel()
	// Arrange: actual remote call, then host-supplied heterogeneous content.
	requests := make(chan []byte, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- body
		if strings.Contains(string(body), "function_call_output") {
			writeResponseFixture(w, "completed", "observed")
			return
		}
		toolResponseFixture(w, responsesFunctionFixture)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].InputMIME = []string{"image/png", "application/pdf", "text/plain"}
	exec := prompty.SimplePrompt("lookup")
	exec.Tools = responseToolDefinitions()
	created, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(true)})
	require.NoError(t, err)
	content := []prompty.ContentPart{
		prompty.TextPart{Text: "start"},
		prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://example.invalid/image.png"},
		&prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1, 2, 3}},
		prompty.MediaPart{MediaType: "document", MIMEType: "application/pdf", Data: []byte("%PDF-fixture")},
		prompty.MediaPart{MediaType: "document", MIMEType: "text/plain", URL: "https://example.invalid/report.txt"},
		prompty.TextPart{Text: `{"sequence":9007199254740993}`},
	}
	next := prompty.NewExecution([]prompty.ChatMessage{{Role: prompty.RoleTool, Content: []prompty.ContentPart{
		&prompty.ToolResultPart{ToolCallID: "call_fixture", Content: content},
	}}})
	// Act: no URL fetch, upload, handler, flattening or inherited declaration.
	result, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: next, Previous: created.Handle, Store: new(true),
	})
	// Assert.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeCompleted, result.Response.Outcome)
	<-requests
	var actual struct {
		Input []struct {
			CallID string            `json:"call_id"`
			Output []json.RawMessage `json:"output"`
		} `json:"input"`
		Tools []json.RawMessage `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &actual))
	require.Len(t, actual.Input, 1)
	require.Equal(t, "call_fixture", actual.Input[0].CallID)
	output := actual.Input[0].Output
	require.Len(t, output, 6)
	require.JSONEq(t, `{"type":"input_text","text":"start"}`, string(output[0]))
	require.JSONEq(t, `{"type":"input_image","image_url":"https://example.invalid/image.png","detail":"auto"}`,
		string(output[1]))
	require.Contains(t, string(output[2]), "data:image/png;base64,AQID")
	require.Contains(t, string(output[3]), "data:application/pdf;base64,JVBERi1maXh0dXJl")
	require.Contains(t, string(output[3]), `"filename":"input.pdf"`)
	require.Contains(t, string(output[4]), `"file_url":"https://example.invalid/report.txt"`)
	require.Contains(t, string(output[5]), "9007199254740993")
	require.Empty(t, actual.Tools)
	require.Equal(t, []byte{1, 2, 3}, content[2].(*prompty.MediaPart).Data)
}

func TestResponsesFunctionResultsUnsupportedBeforeIO(t *testing.T) {
	t.Parallel()
	// Arrange: known call reference and an allowed image/PDF profile.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseFixture(w, "completed", "unexpected")
	}))
	t.Cleanup(server.Close)
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].InputMIME = []string{"image/png", "application/pdf", "audio/wav"}
	handle := remoteFixtureHandle(manager)
	handle.Status = prompty.RemoteResponseCompleted
	handle.ToolSchemaDigests = map[string]string{"lookup": strings.Repeat("a", 64)}
	handle.ToolCalls = map[string]string{"call_fixture": "lookup"}
	for _, test := range []struct {
		name   string
		result prompty.ToolResultPart
	}{
		{"is_error", prompty.ToolResultPart{IsError: true}},
		{"mime_not_allowed", prompty.ToolResultPart{Content: []prompty.ContentPart{
			prompty.MediaPart{MediaType: "image", MIMEType: "image/jpeg", Data: []byte{1}},
		}}},
		{"ambiguous_source", prompty.ToolResultPart{Content: []prompty.ContentPart{
			prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://example.invalid", Data: []byte{1}},
		}}},
		{"url_credentials", prompty.ToolResultPart{Content: []prompty.ContentPart{
			prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://user:secret@example.invalid"},
		}}},
		{"unsupported_audio", prompty.ToolResultPart{Content: []prompty.ContentPart{
			prompty.MediaPart{MediaType: "audio", MIMEType: "audio/wav", Data: []byte{1}},
		}}},
		{"nonpdf_inline", prompty.ToolResultPart{Content: []prompty.ContentPart{
			prompty.MediaPart{MediaType: "document", MIMEType: "image/png", Data: []byte{1}},
		}}},
		{"unknown_nested_kind", prompty.ToolResultPart{Content: []prompty.ContentPart{prompty.ReasoningPart{Text: "hidden"}}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			part := test.result
			part.ToolCallID = "call_fixture"
			exec := prompty.NewExecution([]prompty.ChatMessage{{Role: prompty.RoleTool,
				Content: []prompty.ContentPart{part}}})
			// Act.
			_, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{
				Execution: exec, Previous: &handle, Store: new(true),
			})
			// Assert.
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}
