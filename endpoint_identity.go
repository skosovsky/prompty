package prompty

import (
	"fmt"
	"net/url"
	"strings"
)

// ValidateEndpointIdentity rejects authentication-bearing URL components without
// echoing values. Empty optional identity is allowed; required fields are checked
// by their owner. Opaque references and URI paths remain the host's responsibility.
func ValidateEndpointIdentity(endpoint string) error {
	parsed, err := url.Parse(endpoint)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		strings.Contains(endpoint, "#") {
		return fmt.Errorf("%w: /identity/endpoint must be a credential-free reference", ErrCapabilityProfile)
	}
	return nil
}

func validateDiagnosticsEndpointIdentities(diagnostics ExecutionDiagnostics) error {
	if err := ValidateEndpointIdentity(diagnostics.Profile.Endpoint); err != nil {
		return err
	}
	for _, report := range diagnostics.Schemas {
		if err := ValidateEndpointIdentity(report.Profile.Endpoint); err != nil {
			return err
		}
	}
	return nil
}
