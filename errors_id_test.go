package prompty

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestValidateID(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id    string
		valid bool
	}{
		{"", false},
		{"x", true},
		{"support_agent", true},
		{"internal/router", true},
		{"name-with-dash", true},
		{"name/with/slash", true},
		{"name\\backslash", false},
		{".", false},
		{"..", false},
		{"name:with:colon", false},
		{"internal/router.yaml", false},
		{"agent.prod", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			err := ValidateID(tt.id)
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			require.ErrorIs(t, err, ErrInvalidName)
		})
	}
}
