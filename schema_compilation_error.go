package prompty

import (
	"cmp"
	"errors"
	"net/url"
	"slices"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

const schemaSourceURI = "https://prompty.invalid/source-schema"

// SchemaSourceLocation identifies a position in a schema input document.
// Resource is empty for the supplied source, otherwise the explicit resource URI.
// Pointer is RFC 6901; empty denotes the document root, not an unknown position.
type SchemaSourceLocation struct {
	Resource string
	Pointer  string
}

// SchemaCompilationError preserves the original compiler cause and known positions.
// Empty Locations means the compiler did not expose an authoritative source position.
// Error does not print schema values; Cause may contain caller data and needs host policy.
type SchemaCompilationError struct {
	Locations []SchemaSourceLocation
	Cause     error
}

func (*SchemaCompilationError) Error() string { return "prompty: JSON Schema compilation failed" }

func (e *SchemaCompilationError) Unwrap() error {
	return errors.Join(ErrSchemaUnsupported, e.Cause)
}

func newSchemaCompilationError(cause error) *SchemaCompilationError {
	result := &SchemaCompilationError{Locations: nil, Cause: cause}
	if failure, ok := errors.AsType[*jsonschema.SchemaValidationError](cause); ok {
		base, valid := schemaCompilationLocation(failure.URL)
		if validation, ok := errors.AsType[*jsonschema.ValidationError](failure.Err); ok && valid {
			collectSchemaValidationLocations(validation, base, &result.Locations)
		}
	}
	sortSchemaSourceLocations(result.Locations)
	result.Locations = slices.Compact(result.Locations)
	return result
}

func sortSchemaSourceLocations(locations []SchemaSourceLocation) {
	slices.SortFunc(locations, func(a, b SchemaSourceLocation) int {
		if order := cmp.Compare(a.Resource, b.Resource); order != 0 {
			return order
		}
		return cmp.Compare(a.Pointer, b.Pointer)
	})
}

func schemaCompilationLocation(location string) (SchemaSourceLocation, bool) {
	u, err := url.Parse(location)
	if err != nil || (u.Fragment != "" && !strings.HasPrefix(u.Fragment, "/")) {
		return SchemaSourceLocation{Resource: "", Pointer: ""}, false
	}
	pointer := u.Fragment
	u.Fragment, u.RawFragment = "", ""
	resource := u.String()
	if resource == schemaSourceURI {
		resource = ""
	}
	return SchemaSourceLocation{Resource: resource, Pointer: pointer}, true
}

func collectSchemaValidationLocations(
	failure *jsonschema.ValidationError,
	base SchemaSourceLocation,
	locations *[]SchemaSourceLocation,
) {
	if len(failure.Causes) != 0 {
		for _, child := range failure.Causes {
			collectSchemaValidationLocations(child, base, locations)
		}
		return
	}
	for _, token := range failure.InstanceLocation {
		base.Pointer += "/" + strings.ReplaceAll(strings.ReplaceAll(token, "~", "~0"), "/", "~1")
	}
	*locations = append(*locations, base)
}
