package prompty

import "errors"

// StructuredOutputMode selects output transport, independently of schema fidelity.
// Synthetic output validates a received result; it is not constrained decoding.
type StructuredOutputMode string

const (
	OutputNative        StructuredOutputMode = "native"
	OutputSyntheticTool StructuredOutputMode = "synthetic_tool"
)

// ErrOutputMode identifies an invalid or contradictory output selection.
var ErrOutputMode = errors.New("prompty: invalid structured output mode")

// ValidateOutputMode checks portable shape, not provider support.
func ValidateOutputMode(mode StructuredOutputMode, format *SchemaDefinition) error {
	if mode != "" && mode != OutputNative && mode != OutputSyntheticTool {
		return ErrOutputMode
	}
	if mode != "" && format == nil {
		return ErrOutputMode
	}
	return nil
}
