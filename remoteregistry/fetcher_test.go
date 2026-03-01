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
		{"agent.prod", true},
		{"name-with-dash", true},
		{"name/with/slash", false},
		{"name\\backslash", false},
		{"..", false},
		{"name:with:colon", false},
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

func TestCandidatePaths(t *testing.T) {
	t.Parallel()
	tests := []struct {
		id   string
		want []string
	}{
		{"x", []string{"x.yaml", "x.yml"}},
		{"support_agent", []string{"support_agent.yaml", "support_agent.yml"}},
		{"agent.prod", []string{"agent.prod.yaml", "agent.prod.yml"}},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			t.Parallel()
			got := CandidatePaths(tt.id)
			assert.Equal(t, tt.want, got)
		})
	}
}
