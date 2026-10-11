package openai

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/skosovsky/prompty"
)

func ExampleNewResponses_toolResults() {
	// Offline transport; the host supplies results, not an automatic agent loop.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), "function_call_output") {
			writeResponseFixture(w, "completed", "Document found")
			return
		}
		toolResponseFixture(w, responsesFunctionFixture)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].InputMIME = []string{"image/png"}
	exec := prompty.SimplePrompt("Lookup the document")
	exec.Tools = responseToolDefinitions()
	created, err := manager.Create(context.Background(), prompty.RemoteResponseRequest{
		Execution: exec, Store: new(true),
	})
	if err != nil {
		panic(err)
	}
	call := created.Response.Content[0].(prompty.ToolCallPart)
	// Persist/restore the scoped handle as needed. Inspect errors before using calls.
	// Instructions and current tools remain explicit; this call declares none.
	next := prompty.NewExecution([]prompty.ChatMessage{{
		Role: prompty.RoleTool,
		Content: []prompty.ContentPart{prompty.ToolResultPart{
			ToolCallID: call.ID, Name: call.Name,
			Content: []prompty.ContentPart{
				prompty.TextPart{Text: "Document found"},
				prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte{1, 2, 3}},
			},
		}},
	}})
	continued, err := manager.Create(context.Background(), prompty.RemoteResponseRequest{
		Execution: next, Previous: created.Handle, Store: new(true),
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(continued.Response.Content[0].(prompty.TextPart).Text)
	// Output: Document found
}
