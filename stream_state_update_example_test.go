package prompty_test

import (
	"context"
	"fmt"
	"iter"

	"github.com/skosovsky/prompty"
)

// A provider source explicitly binds each opaque state snapshot to the current
// tool part. Payload interpretation/continuation validation stays in its adapter.
func ExampleResponseChunk_stateUpdates() {
	envelope := prompty.ProviderEnvelope{
		Format: prompty.ProviderStateFormat,
		Scope:  prompty.ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "fixture"},
		Codec:  "fixture.function", Required: true,
	}
	stream := prompty.NewStream(context.Background(), prompty.StreamNative,
		func(context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
			return func(yield func(*prompty.ResponseChunk, error) bool) {
				for _, snapshot := range []struct{ args, state string }{
					{"", "pending"}, {`{"n":1}`, "complete"},
				} {
					state := envelope
					state.Payload = []byte(snapshot.state)
					if !yield(&prompty.ResponseChunk{
						Kind:     prompty.StreamDelta,
						Identity: prompty.StreamIdentity{PartIDs: []string{"tool:0"}},
						Content: []prompty.ContentPart{
							prompty.ToolCallPart{ID: "call", Name: "lookup", Args: snapshot.args},
						},
						StateUpdates: []prompty.PartState{{PartIndex: 0, Envelope: state}},
					}, nil) {
						return
					}
				}
				yield(&prompty.ResponseChunk{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeToolCalls}, nil)
			}
		})
	defer stream.Close()
	for _, err := range stream.Events() {
		if err != nil {
			fmt.Println(err)
			return
		}
	}
	status := stream.Status()
	fmt.Println(status.State)
	fmt.Println(status.Result.Content[0].(prompty.ToolCallPart).Args)
	fmt.Println(string(status.Result.ProviderState[0].Envelope.Payload))
	// Output:
	// completed
	// {"n":1}
	// complete
}
