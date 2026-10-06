package openai

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestResponsesActualSDKOrderedMediaInputWithoutFetchOrUpload(t *testing.T) {
	t.Parallel()
	// Arrange: one native Responses endpoint; referenced URLs must not be fetched locally.
	var calls atomic.Int32
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.URL.Path != "/v1/responses" {
			t.Errorf("unexpected media fetch/upload: %s", r.URL.Path)
			http.Error(w, "fixture", http.StatusBadRequest)
			return
		}
		body, _ := io.ReadAll(r.Body)
		requests <- body
		writeResponseFixture(w, "completed", "answer")
	}))
	defer server.Close()
	manager := responseFixtureAdapter(server)
	manager.base.profiles[0].InputMIME = []string{"image/png", "application/pdf"}
	image, pdf := []byte{0, 1, 255}, []byte("%PDF-fixture")
	exec := prompty.NewExecution([]prompty.ChatMessage{
		{Role: prompty.RoleSystem, Content: []prompty.ContentPart{prompty.TextPart{Text: "extract"}}},
		{Role: prompty.RoleUser, Content: []prompty.ContentPart{
			prompty.TextPart{
				Text: "before",
			}, &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: image},
			prompty.TextPart{
				Text: "between",
			}, prompty.MediaPart{MediaType: "document", MIMEType: "application/pdf", URL: server.URL + "/document?host_token=private"},
			prompty.MediaPart{MediaType: "document", MIMEType: "application/pdf", Data: pdf},
			prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: server.URL + "/image"},
			&prompty.TextPart{Text: "after"}}},
	})
	original := exec.Clone()
	// Act.
	result, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(false)})
	// Assert: actual SDK request preserves order, exact bytes and explicit caller intent.
	require.NoError(t, err)
	require.Equal(t, prompty.RemoteResponseCompleted, result.Handle.Status)
	require.Equal(t, original, exec)
	require.EqualValues(t, 1, calls.Load())
	var request struct {
		Instructions string `json:"instructions"`
		Input        []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"input"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.Equal(t, "extract", request.Instructions)
	require.Len(t, request.Input, 1)
	content := request.Input[0].Content
	require.Len(t, content, 7)
	require.Equal(
		t,
		[]any{"input_text", "input_image", "input_text", "input_file", "input_file", "input_image", "input_text"},
		[]any{
			content[0]["type"],
			content[1]["type"],
			content[2]["type"],
			content[3]["type"],
			content[4]["type"],
			content[5]["type"],
			content[6]["type"],
		},
	)
	require.Equal(t, "before", content[0]["text"])
	require.Equal(t, "between", content[2]["text"])
	require.Equal(t, "after", content[6]["text"])
	require.Equal(t, "data:image/png;base64,"+base64.StdEncoding.EncodeToString(image), content[1]["image_url"])
	require.Equal(t, "auto", content[1]["detail"])
	require.Equal(t, server.URL+"/document?host_token=private", content[3]["file_url"])
	require.Equal(t, "input.pdf", content[4]["filename"])
	require.Equal(t, "data:application/pdf;base64,"+base64.StdEncoding.EncodeToString(pdf), content[4]["file_data"])
	require.Equal(t, server.URL+"/image", content[5]["image_url"])
}

func TestResponsesMediaInvalidMappingOrProfileRejectsBeforeIO(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		role    prompty.Role
		media   *prompty.MediaPart
		support prompty.CapabilitySupport
	}{
		{name: "unknown MIME", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1}}, support: prompty.CapabilityUnknown},
		{name: "unsupported MIME", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1}}, support: prompty.CapabilityUnsupported},
		{name: "two sources", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://fixture.invalid/image", Data: []byte{1}}, support: prompty.CapabilitySupported},
		{name: "no source", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png"}, support: prompty.CapabilitySupported},
		{name: "mismatched kind", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "image", MIMEType: "application/pdf", Data: []byte{1}}, support: prompty.CapabilitySupported},
		{name: "unsupported audio", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "audio", MIMEType: "audio/wav", Data: []byte{1}}, support: prompty.CapabilitySupported},
		{name: "unsupported inline document", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "document", MIMEType: "text/plain", Data: []byte{1}}, support: prompty.CapabilitySupported},
		{name: "credentials in URL", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", URL: "https://private:secret@fixture.invalid/image"}, support: prompty.CapabilitySupported},
		{name: "non HTTP URL", role: prompty.RoleUser, media: &prompty.MediaPart{MediaType: "document", MIMEType: "application/pdf", URL: "file:///private.pdf"}, support: prompty.CapabilitySupported},
		{name: "assistant media", role: prompty.RoleAssistant, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1}}, support: prompty.CapabilitySupported},
		{name: "instructions media", role: prompty.RoleSystem, media: &prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1}}, support: prompty.CapabilitySupported},
		{name: "nil part", role: prompty.RoleUser, support: prompty.CapabilitySupported},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: even claimed MIME knowledge cannot invent an unimplemented mapping.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeResponseFixture(w, "completed", "answer")
			}))
			defer server.Close()
			manager := responseFixtureAdapter(server)
			switch tc.support {
			case prompty.CapabilityUnknown:
				manager.base.profiles[0].InputMIME = nil
			case prompty.CapabilityUnsupported:
				manager.base.profiles[0].InputMIME = []string{"image/jpeg"}
			default:
				manager.base.profiles[0].InputMIME = []string{"image/png", "application/pdf", "audio/wav", "text/plain"}
			}
			exec := prompty.NewExecution(
				[]prompty.ChatMessage{{Role: tc.role, Content: []prompty.ContentPart{tc.media}}},
			)
			// Act.
			_, err := manager.Create(t.Context(), prompty.RemoteResponseRequest{Execution: exec, Store: new(false)})
			// Assert: no provider/upload/fetch and no sensitive URL in the failure.
			require.Error(t, err)
			require.Zero(t, calls.Load())
			require.NotContains(t, err.Error(), "private")
			require.NotContains(t, err.Error(), "secret")
		})
	}
}
