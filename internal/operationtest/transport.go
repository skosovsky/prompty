// Package operationtest supplies deterministic SDK fault-injection transport fixtures.
package operationtest

import (
	"context"
	"io"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"
)

// RoundTripFunc adapts a local fixture without changing the provider SDK request path.
type RoundTripFunc func(*http.Request) (*http.Response, error)

func (f RoundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

// Body emits a prefix, then blocks until cancellation or Close. Blocked proves
// the caller is reading an accepted transport body, not merely preflighting.
type Body struct {
	ctx     context.Context
	prefix  *strings.Reader
	blocked chan struct{}
	closed  chan struct{}
	read    sync.Once
	close   sync.Once
	closes  atomic.Int32
}

func NewBody(ctx context.Context, prefix string) *Body {
	return &Body{
		ctx: ctx, prefix: strings.NewReader(prefix), blocked: make(chan struct{}), closed: make(chan struct{}),
		read: sync.Once{}, close: sync.Once{}, closes: atomic.Int32{},
	}
}

func (b *Body) Read(destination []byte) (int, error) {
	if len(destination) == 0 {
		return 0, nil
	}
	if b.prefix.Len() != 0 {
		return b.prefix.Read(destination)
	}
	b.read.Do(func() { close(b.blocked) })
	select {
	case <-b.ctx.Done():
		return 0, b.ctx.Err()
	case <-b.closed:
		return 0, io.ErrClosedPipe
	}
}

func (b *Body) Close() error {
	b.closes.Add(1)
	b.close.Do(func() { close(b.closed) })
	return nil
}

func (b *Body) Blocked() <-chan struct{} { return b.blocked }
func (b *Body) Closes() int32            { return b.closes.Load() }
