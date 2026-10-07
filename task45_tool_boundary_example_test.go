package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

type exampleLookupArgs struct {
	Name string `json:"name"`
}

// An application (or toolsy integration) supplies this port. It need not use
// TypedToolRegistry, and owns authorization, scheduling and downstream effects.
type exampleCallerToolPort struct{}

func (exampleCallerToolPort) ValidateToolCall(name, argsJSON string) error {
	if name != "lookup" {
		return errors.New("unknown caller tool")
	}
	args, err := DecodeToolArgs[exampleLookupArgs](argsJSON)
	if err != nil {
		return err
	}
	if args.Name == "" {
		return errors.New("lookup name is required")
	}
	return nil
}

func (port exampleCallerToolPort) InvokeTool(ctx context.Context, name, argsJSON string) (json.RawMessage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := port.ValidateToolCall(name, argsJSON); err != nil {
		return nil, err
	}
	args, err := DecodeToolArgs[exampleLookupArgs](argsJSON)
	if err != nil {
		return nil, err
	}
	// Substitute the caller-owned context-aware service operation here.
	return json.Marshal(map[string]string{"found": args.Name})
}

func ExampleToolInvoker() {
	var tools ToolInvoker = exampleCallerToolPort{}
	// ExecuteWithTypedTools accepts this same port after original-schema/scope
	// whole-batch preflight. This local example exercises only the caller port.
	result, err := tools.InvokeTool(context.Background(), "lookup", `{"name":"known"}`)
	if err != nil {
		panic(err)
	}
	fmt.Println(string(result))
	// Output: {"found":"known"}
}
