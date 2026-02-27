package prompty

import (
	"errors"
	"fmt"
	"strings"
)

// Sentinel errors for template and registry operations.
// All use prefix "prompty:" for identification. Callers should use errors.Is/errors.As.
var (
	ErrMissingVariable  = errors.New("prompty: required template variable not provided")
	ErrTemplateRender   = errors.New("prompty: template rendering failed")
	ErrTemplateParse    = errors.New("prompty: template parsing failed")
	ErrInvalidPayload   = errors.New("prompty: payload struct is invalid or missing prompt tags")
	ErrTemplateNotFound = errors.New("prompty: template not found in registry")
	ErrInvalidManifest  = errors.New("prompty: manifest file is malformed")
	ErrReservedVariable = errors.New("prompty: reserved variable name in payload (use a different prompt tag than Tools)")
	// ErrInvalidName indicates template name or env contains invalid characters (e.g. ':', path separators).
	ErrInvalidName = errors.New("prompty: invalid template name")
)

// VariableError wraps a sentinel error with variable and template context.
// Use errors.Is(err, ErrMissingVariable) and errors.As(err, &variableErr) to inspect.
type VariableError struct {
	Variable string
	Template string
	Err      error
}

// Error implements error.
func (e *VariableError) Error() string {
	return fmt.Sprintf("prompty: variable %q in template %q: %v", e.Variable, e.Template, e.Err)
}

// Unwrap returns the wrapped error for errors.Is/errors.As.
func (e *VariableError) Unwrap() error { return e.Err }

// Compile-time check that VariableError implements error.
var _ error = (*VariableError)(nil)

// ValidateName checks that name and env are safe for use in paths and cache keys.
// Rejects empty name and names containing '/', '\\', "..", or ':'. Call before registry GetTemplate or path resolution.
func ValidateName(name, env string) error {
	if name == "" {
		return fmt.Errorf("%w: name must not be empty", ErrInvalidName)
	}
	invalid := []string{"/", "\\", "..", ":"}
	for _, s := range invalid {
		if strings.Contains(name, s) || strings.Contains(env, s) {
			return fmt.Errorf("%w: name and env must not contain %q", ErrInvalidName, s)
		}
	}
	return nil
}
