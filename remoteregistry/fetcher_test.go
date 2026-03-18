package remoteregistry

import (
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
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
			require.ErrorIs(t, err, prompty.ErrInvalidName)
		})
	}
}

func TestValidatePathForFetch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id    string
		valid bool
	}{
		{"", false},
		{"internal/router", true},
		{"internal/router.prod", true},
		{"..", false},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			err := ValidatePathForFetch(tt.id)
			if tt.valid {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
		})
	}
}

func TestCandidatePaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id   string
		want []string
	}{
		{"x", []string{"x.yaml", "x.yml", "x.json"}},
		{"support_agent", []string{"support_agent.yaml", "support_agent.yml", "support_agent.json"}},
		{"internal/router", []string{"internal/router.yaml", "internal/router.yml", "internal/router.json"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			got := CandidatePaths(tt.id)
			assert.Equal(t, tt.want, got)
		})
	}
}
