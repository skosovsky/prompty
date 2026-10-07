package hostdispatch

import (
	"github.com/skosovsky/routery"

	"github.com/skosovsky/prompty"
)

// StreamOwner binds lifetime cleanup to consumption completion. Caller must run
// Consume exactly once on its chosen goroutine. Callbacks use Stream.Close only;
// Lifetime.Close joins and must never be called from the consumption callback.
type StreamOwner struct {
	Stream   *prompty.Stream
	Lifetime *routery.Lifetime
	done     chan struct{}
}

// OwnStream creates an owner without starting a producer or consuming events.
// Even a closed-before-consumption handle must be consumed to close done.
func OwnStream(stream *prompty.Stream) *StreamOwner {
	done := make(chan struct{})
	return &StreamOwner{Stream: stream, done: done, Lifetime: routery.NewLifetime(func() error {
		if err := stream.Close(); err != nil {
			return err
		}
		<-done
		return nil
	})}
}

// Consume runs on the caller goroutine and signals completion after iterator unwind.
func (owner *StreamOwner) Consume(onEvent func(*prompty.ResponseChunk, error)) {
	defer close(owner.done)
	for event, err := range owner.Stream.Events() {
		onEvent(event, err)
	}
}
