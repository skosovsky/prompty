package adapter

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRejectUnknownProviderSettingKeys(t *testing.T) {
	t.Parallel()
	err := RejectUnknownProviderSettingKeys(
		map[string]any{"known": 1, "unknown": 2},
		[]string{"known"},
	)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProviderSettings)
}

func TestValidateFloat32Range(t *testing.T) {
	t.Parallel()
	err := ValidateFloat32Range("presence_penalty", 3, -2, 2)
	require.Error(t, err)
	assert.ErrorIs(t, err, ErrInvalidProviderSettings)
}
