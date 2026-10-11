// This offline example uses a local invoker; it makes no provider request.
package main

import (
	"context"
	"fmt"
	"iter"
	"log"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/ext/truncate"
	"github.com/skosovsky/prompty/middleware/contextx"
)

const exampleMessageBudget = 6

type localInvoker struct{}

func (localInvoker) Execute(_ context.Context, execution *prompty.PromptExecution) (*prompty.Response, error) {
	fmt.Printf("retained messages: %d\n", len(execution.Messages))
	return prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "offline result"}}), nil
}

func (localInvoker) ExecuteStream(
	ctx context.Context,
	execution *prompty.PromptExecution,
	mode prompty.StreamMode,
) *prompty.Stream {
	return prompty.NewStream(ctx, mode, func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
		return func(yield func(*prompty.ResponseChunk, error) bool) {
			fmt.Printf("retained messages: %d\n", len(execution.Messages))
			yield(&prompty.ResponseChunk{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeCompleted}, nil)
		}
	})
}

func main() {
	execution := prompty.NewExecution(
		[]prompty.ChatMessage{
			prompty.NewSystemMessage("policy"),
			prompty.NewUserMessage("old explanation with substantial detail"),
			prompty.NewUserMessage("current question"),
		},
	)
	options := contextx.Options{
		Budget: truncate.Options{
			Limit:     exampleMessageBudget,
			Estimator: truncate.MessageEstimator{Counter: &prompty.CharFallbackCounter{}},
		},
		Observe: func(_ context.Context, report truncate.Report) error {
			fmt.Printf(
				"coverage=%s before=%d after=%d state=%s removed=%v\n",
				report.After.Scope,
				report.Before.Tokens,
				report.After.Tokens,
				report.State,
				report.Removed,
			)
			return nil
		},
	}
	invoker := contextx.WithTokenBudget(options)(localInvoker{})
	if _, err := invoker.Execute(context.Background(), execution); err != nil {
		log.Fatal(err)
	}
	// The same policy is applied before the lazy native handle dispatches.
	stream := invoker.ExecuteStream(context.Background(), execution, prompty.StreamNative)
	for _, err := range stream.Events() {
		if err != nil {
			log.Fatal(err)
		}
	}
}
