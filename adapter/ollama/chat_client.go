package ollama

import (
	"context"
	"errors"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty/adapter"
)

// ChatClient is the host-owned official SDK Chat operation port.
// Production hosts inject *api.Client; the interface also permits offline doubles.
// SDK-OLLAMA-NUM and SDK-OLLAMA-FRAME in docs/official-sdk-migration.md
// describe decoding/scanner limits; this port makes no raw fidelity guarantee.
type ChatClient interface {
	Chat(context.Context, *api.ChatRequest, api.ChatResponseFunc) error
}

// SDK errors may contain provider bodies or host URLs. Preserve only safe
// cancellation and adapter callback categories, without parsing provider payloads.
type chatCallbackError struct{ cause error }

func (e *chatCallbackError) Error() string { return e.cause.Error() }
func (e *chatCallbackError) Unwrap() error { return e.cause }

func safeChatError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if callback, ok := errors.AsType[*chatCallbackError](err); ok {
		return callback.cause
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded,
		adapter.ErrMalformedArgs, adapter.ErrInvalidResponse} {
		if errors.Is(err, cause) {
			return cause
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return adapter.ErrInvalidResponse
}
