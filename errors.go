package prompty

import (
	"errors"
	"fmt"
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
