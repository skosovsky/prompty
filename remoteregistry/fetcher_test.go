package remoteregistry

import (
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		env   string
		valid bool
	}{
		{"", "", false},
		{"", "prod", false},
		{"x", "", true},
		{"support_agent", "production", true},
		{"agent", "staging", true},
		{"name-with-dash", "prod", true},
		{"x", "path/to/env", false},
		{"name/with/slash", "", false},
		{"name\\backslash", "", false},
		{"..", "", false},
		{"name:with:colon", "", false},
		{"ok", "env:with:colon", false},
		{"ok", "env/with/slash", false},
		{"ok", "..", false},
	}
	for _, tt := range tests {
		t.Run(tt.name+":"+tt.env, func(t *testing.T) {
			t.Parallel()
			err := ValidateName(tt.name, tt.env)
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
		name string
		env  string
		want []string
	}{
		{"x", "", []string{"x.yaml", "x.yml"}},
		{"x", "prod", []string{"x.prod.yaml", "x.prod.yml", "x.yaml", "x.yml"}},
		{"support_agent", "staging", []string{"support_agent.staging.yaml", "support_agent.staging.yml", "support_agent.yaml", "support_agent.yml"}},
	}
	for _, tt := range tests {
		t.Run(tt.name+":"+tt.env, func(t *testing.T) {
			t.Parallel()
			got := CandidatePaths(tt.name, tt.env)
			assert.Equal(t, tt.want, got)
		})
	}
}
