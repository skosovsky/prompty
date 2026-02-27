package prompty

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVariableError_Error(t *testing.T) {
	t.Parallel()
	err := &VariableError{
		Variable: "user_name",
		Template: "support_agent",
		Err:      ErrMissingVariable,
	}
	assert.Contains(t, err.Error(), "user_name")
	assert.Contains(t, err.Error(), "support_agent")
	assert.Contains(t, err.Error(), "prompty:")
}

func TestVariableError_Unwrap(t *testing.T) {
	t.Parallel()
	err := &VariableError{
		Variable: "x",
		Template: "t",
		Err:      ErrMissingVariable,
	}
	require.ErrorIs(t, err, ErrMissingVariable)
	unwrapped := errors.Unwrap(err)
	require.Error(t, unwrapped)
	assert.ErrorIs(t, unwrapped, ErrMissingVariable)
}

func TestVariableError_errorsAs(t *testing.T) {
	t.Parallel()
	wrapped := &VariableError{
		Variable: "foo",
		Template: "bar",
		Err:      ErrMissingVariable,
	}
	// Wrap again to simulate error chain
	outer := fmt.Errorf("outer: %w", wrapped)

	var ve *VariableError
	require.ErrorAs(t, outer, &ve)
	assert.Equal(t, "foo", ve.Variable)
	assert.Equal(t, "bar", ve.Template)
	assert.ErrorIs(t, ve, ErrMissingVariable)
}

func TestSentinelErrors_Is(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		err    error
		target error
		want   bool
	}{
		{"missing var", ErrMissingVariable, ErrMissingVariable, true},
		{"template render", ErrTemplateRender, ErrTemplateRender, true},
		{"invalid payload", ErrInvalidPayload, ErrInvalidPayload, true},
		{"template not found", ErrTemplateNotFound, ErrTemplateNotFound, true},
		{"template parse", ErrTemplateParse, ErrTemplateParse, true},
		{"invalid manifest", ErrInvalidManifest, ErrInvalidManifest, true},
		{"reserved variable", ErrReservedVariable, ErrReservedVariable, true},
		{"invalid name", ErrInvalidName, ErrInvalidName, true},
		{"wrapped missing", fmt.Errorf("wrap: %w", ErrMissingVariable), ErrMissingVariable, true},
		{"wrong target", ErrMissingVariable, ErrTemplateRender, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, errors.Is(tt.err, tt.target))
		})
	}
}
