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

func ExampleNewResponses_hostedHistory() {
	// Offline fixture. The host owns history persistence and search resources.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"type":"web_search_call"`) {
			writeResponseFixture(w, "completed", "Clarified answer")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, hostedOutputFixture)
	}))
	defer server.Close()
	manager := hostedFixtureAdapter(server)
	created, err := manager.Create(context.Background(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("Search"), Store: new(false), HostedTools: []prompty.HostedToolRequest{
			{Kind: HostedWebSearch}, {Kind: HostedFileSearch, Settings: []byte(`{"vector_store_ids":["vs_fixture"]}`)},
		},
	})
	if err != nil {
		panic(err)
	}
	// Keep content, annotations, ordered manifest and required opaque state together.
	history, err := created.Response.AssistantMessage()
	if err != nil {
		panic(err)
	}
	next := prompty.NewExecution([]prompty.ChatMessage{history, {
		Role: prompty.RoleUser, Content: []prompty.ContentPart{prompty.TextPart{Text: "Clarify"}},
	}})
	// No tools selected for this turn; history does not inherit declarations.
	result, err := manager.Create(
		context.Background(),
		prompty.RemoteResponseRequest{Execution: next, Store: new(false)},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Response.Content[0].(prompty.TextPart).Text)
	// Output: Clarified answer
}
