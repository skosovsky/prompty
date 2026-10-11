package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/skosovsky/prompty"
)

func ExampleNewResponses_reasoningState() {
	// Offline SDK fixture; the host owns storage and the model-specific profile.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		writeReasoningResponse(w, responseReasoningFixture)
	}))
	defer server.Close()
	manager := reasoningFixtureAdapter(server)
	exec := prompty.SimplePrompt("First question")
	exec.Requirements.Capabilities = []string{prompty.CapabilityProviderState}
	first, err := manager.Create(
		context.Background(),
		prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
	)
	if err != nil {
		panic(err)
	}
	// Preserve required state with the exact assistant content, not summary alone.
	next := prompty.SimplePrompt("Follow-up question")
	next.Messages = append([]prompty.ChatMessage{{
		Role: prompty.RoleAssistant, Content: first.Response.Content,
		ProviderState:           first.Response.ProviderState,
		ContinuationUnavailable: first.Response.ContinuationUnavailable,
	}}, next.Messages...)
	result, err := manager.Create(
		context.Background(),
		prompty.RemoteResponseRequest{Execution: next, Store: new(false)},
	)
	if err != nil {
		panic(err)
	}
	fmt.Println(result.Response.Content[1].(prompty.TextPart).Text)
	// Output:
	// answer
}
