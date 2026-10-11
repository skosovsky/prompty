package adapter

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSafeProviderError(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name        string
		cause, want error
	}{
		{"private", errors.New("credential secret provider-body"), ErrInvalidResponse},
		{"cancel", errors.Join(errors.New("private"), context.Canceled), context.Canceled},
		{"deadline", errors.Join(errors.New("private"), context.DeadlineExceeded), context.DeadlineExceeded},
		{"success", nil, nil},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK/host causes are untrusted data.
			ctx := t.Context()
			// Act.
			got := SafeProviderError(ctx, fixture.cause)
			// Assert: only safe identities survive.
			require.Equal(t, fixture.want, got)
			if got != nil {
				require.NotContains(t, got.Error(), "private")
				require.NotContains(t, got.Error(), "secret")
			}
		})
	}
}
