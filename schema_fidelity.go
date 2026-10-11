package prompty

import "fmt"

// SchemaFidelityError identifies a source-schema construct that a provider mapper
// cannot preserve. Pointer is an RFC 6901 JSON pointer; the empty string is the root.
// It contains no schema values or request data and unwraps to ErrSchemaUnsupported.
type SchemaFidelityError struct {
	Pointer string
	Reason  string
}

func (e *SchemaFidelityError) Error() string {
	return fmt.Sprintf("%s at %q: %s", ErrSchemaUnsupported, e.Pointer, e.Reason)
}

func (*SchemaFidelityError) Unwrap() error { return ErrSchemaUnsupported }
