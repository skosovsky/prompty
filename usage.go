package prompty

import (
	"errors"
	"fmt"
)

// ErrUsage indicates malformed presence/mode/counters or accumulation overflow.
var ErrUsage = errors.New("prompty: invalid usage metadata")

// Validate checks portable usage shape, not provider-specific total composition.
// Unknown-presence scalar estimates are preserved without promoting them to known.
func (u Usage) Validate() error {
	if u.Mode != "" && u.Mode != UsageCumulative && u.Mode != UsageDelta {
		return fmt.Errorf("%w: unrecognized mode", ErrUsage)
	}
	for _, value := range []int{u.PromptTokens, u.CompletionTokens, u.TotalTokens,
		u.PromptTokensCached, u.PromptTokensCacheCreation, u.CompletionTokensReasoning} {
		if value < 0 {
			return fmt.Errorf("%w: negative counter", ErrUsage)
		}
	}
	seen := make(map[UsageCounter]bool)
	for _, counter := range u.Known {
		if usageCounter(&u, counter) == nil || seen[counter] {
			return fmt.Errorf("%w: invalid or duplicate known counter", ErrUsage)
		}
		seen[counter] = true
	}
	return nil
}

// ValidateResponseMetadata checks canonical result metadata without interpreting
// opaque payloads or inventing provider-specific token composition rules.
func ValidateResponseMetadata(response *Response) error {
	if response == nil || !validOutcome(response.Outcome) {
		return ErrResponseOutcome
	}
	if err := response.Usage.Validate(); err != nil {
		return err
	}
	if err := ValidateMessageAnnotations(response.MessageAnnotations); err != nil {
		return err
	}
	if err := validateDiagnosticsEndpointIdentities(response.Diagnostics); err != nil {
		return err
	}
	if response.Diagnostics.Capture != nil {
		if err := response.Diagnostics.Capture.Validate(); err != nil {
			return err
		}
	}
	return ValidatePartAssociations(response.Content, response.ProviderState, response.Annotations)
}
