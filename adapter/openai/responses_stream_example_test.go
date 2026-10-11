package openai

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func ExampleResponses_CreateStream() {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeResponseSSEFixture(w, mixedResponseSSEFixture(true))
	}))
	defer server.Close()
	manager := localToolsFixtureAdapter(server)
	manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = responseToolDefinitions()
	input := prompty.RemoteResponseRequest{Execution: withResponseHostedTools(exec,
		[]prompty.HostedToolRequest{{Kind: HostedWebSearch}}), Store: new(false)}
	stream := manager.CreateStream(context.Background(), input)
	for _, err := range stream.Events() {
		if err != nil {
			panic(err)
		}
	}
	result := stream.RemoteResult()
	fmt.Println(result.Handle.Status, result.Handle.ProviderID, len(result.Response.Content))
	input.Background = true
	rejected := manager.CreateStream(context.Background(), input)
	var rejectedErr error
	for _, err := range rejected.Events() {
		if err != nil {
			rejectedErr = err
		}
	}
	fmt.Printf(
		"background rejected: %t; requests: %d\n",
		errors.Is(rejectedErr, adapter.ErrUnsupportedCombination),
		calls.Load(),
	)
	// Output:
	// completed resp_fixture 3
	// background rejected: true; requests: 1
}
