package openai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/skosovsky/prompty"
)

func ExampleNewResponses_localFunctions() {
	// Offline SDK fixture; no function handler or agent loop is installed.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		toolResponseFixture(w, responsesFunctionFixture)
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	exec := prompty.SimplePrompt("Lookup the document")
	exec.Tools, exec.ForcedTool = responseToolDefinitions(), "lookup"
	created, err := manager.Create(
		context.Background(),
		prompty.RemoteResponseRequest{Execution: exec, Store: new(true)},
	)
	if err != nil {
		panic(err)
	}
	// The host may persist the handle, then rebind original schemas for inspection.
	observed, err := manager.Get(
		context.Background(),
		*created.Handle,
		prompty.RemoteResponseOperationOptions{Tools: exec.Tools},
	)
	if err != nil {
		panic(err)
	}
	call := observed.Response.Content[0].(prompty.ToolCallPart)
	fmt.Println(call.ID, call.Name, call.Args)
	// Output:
	// call_fixture lookup {"id":9007199254740993}
}
