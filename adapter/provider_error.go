package adapter

import (
	"context"
	"errors"
)

// SafeProviderError projects an untrusted SDK/transport failure without retaining
// its body, credentials, URL or arbitrary host error text. It does not catch panics.
// Adapter callback validation errors must be handled separately by their owner.
func SafeProviderError(ctx context.Context, err error) error {
	if err == nil {
		return nil
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	for _, cause := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, cause) {
			return cause
		}
	}
	return ErrInvalidResponse
}
