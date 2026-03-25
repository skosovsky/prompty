package prompty

import (
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Sentinel errors for template and registry operations.
// All use prefix "prompty:" for identification. Callers should use [errors.Is] and [errors.As].
var (
	ErrMissingVariable  = errors.New("prompty: required template variable not provided")
	ErrTemplateRender   = errors.New("prompty: template rendering failed")
	ErrTemplateParse    = errors.New("prompty: template parsing failed")
	ErrInvalidPayload   = errors.New("prompty: payload struct is invalid or missing prompt tags")
	ErrTemplateNotFound = errors.New("prompty: template not found in registry")
	ErrInvalidManifest  = errors.New("prompty: manifest file is malformed")
	ErrNoFetcher        = errors.New("prompty: fetcher is required but not provided")
	ErrReservedVariable = errors.New(
		"prompty: reserved variable name in payload (use a different prompt tag than Tools)",
	)
	// ErrInvalidName indicates template name or env contains invalid characters (e.g. ':', path separators).
	ErrInvalidName = errors.New("prompty: invalid template name")
	// ErrNoParser indicates that a registry was created without a manifest parser (use WithParser when creating the registry).
	ErrNoParser = errors.New("prompty: parser is required but not provided")
	// ErrConflictingDirectives indicates conflicting execution directives (e.g. Tools and ResponseFormat together).
	ErrConflictingDirectives = errors.New(
		"prompty: conflicting directives (e.g. Tools and ResponseFormat cannot be used together)",
	)
)

// VariableError wraps a sentinel error with variable and template context.
// Use [errors.Is](err, ErrMissingVariable) and [errors.As](err, &variableErr) to inspect.
type VariableError struct {
	Variable string
	Template string
	Err      error
}

// ValidationError is returned when model output is invalid JSON or violates semantic validation.
type ValidationError struct {
	RawAssistantMessage *ChatMessage
	FeedbackPrompt      string
	Err                 error
}

// ToolCallError is returned when model-generated tool arguments fail validation.
type ToolCallError struct {
	RawAssistantMessage *ChatMessage
	ToolResults         []ContentPart
	Err                 error
}

// Error implements error.
func (e *VariableError) Error() string {
	return fmt.Sprintf("prompty: variable %q in template %q: %v", e.Variable, e.Template, e.Err)
}

// Unwrap returns the wrapped error for [errors.Is] and [errors.As].
func (e *VariableError) Unwrap() error { return e.Err }

// Error implements error.
func (e *ValidationError) Error() string {
	if e == nil || e.Err == nil {
		return "prompty: validation error"
	}
	return fmt.Sprintf("prompty: validation error: %v", e.Err)
}

// Unwrap returns the wrapped error for [errors.Is] and [errors.As].
func (e *ValidationError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Error implements error.
func (e *ToolCallError) Error() string {
	if e == nil || e.Err == nil {
		return "prompty: tool call error"
	}
	return fmt.Sprintf("prompty: tool call error: %v", e.Err)
}

// Unwrap returns the wrapped error for [errors.Is] and [errors.As].
func (e *ToolCallError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// Compile-time check that VariableError implements error.
var _ error = (*VariableError)(nil)
var _ error = (*ValidationError)(nil)
var _ error = (*ToolCallError)(nil)

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

// ValidateID checks that id is a valid io/fs-style path (slash-separated, no extensions).
// Canonical ID format: slash-only (e.g. "internal/router"). Rejects dotted IDs (Clean Break).
// Rejects ids containing ':', '\', ".", ".." for cross-platform safety.
// Use [fs.ValidPath] for path safety; rejects IDs with file extensions.
func ValidateID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: id must not be empty", ErrInvalidName)
	}
	if strings.Contains(id, ":") || strings.Contains(id, "\\") {
		return fmt.Errorf("%w: id must not contain : or \\", ErrInvalidName)
	}
	if id == "." || id == ".." {
		return fmt.Errorf("%w: id %q is invalid", ErrInvalidName, id)
	}
	if !fs.ValidPath(id) {
		return fmt.Errorf("%w: id %q is not a valid path", ErrInvalidName, id)
	}
	if ext := filepath.Ext(id); ext != "" {
		return fmt.Errorf("%w: id must not contain file extension (got %q)", ErrInvalidName, ext)
	}
	return nil
}
