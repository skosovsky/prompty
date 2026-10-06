package prompty

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ErrCapabilityProfile identifies malformed caller-owned profile knowledge.
var ErrCapabilityProfile = errors.New("prompty: invalid capability profile")

// ValidateCapabilityProfile checks shape, not the truth of provider knowledge.
// Extension capability names remain caller-owned. Errors never echo values.
func ValidateCapabilityProfile(profile CapabilityProfile) error {
	if err := ValidateEndpointIdentity(profile.Identity.Endpoint); err != nil {
		return err
	}
	for _, field := range []struct{ path, value string }{
		{"/identity/provider", profile.Identity.Provider},
		{"/identity/endpoint", profile.Identity.Endpoint},
		{"/identity/model", profile.Identity.Model},
		{"/provenance", profile.Provenance},
	} {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("%w: %s must be nonblank", ErrCapabilityProfile, field.path)
		}
	}
	for capability, status := range profile.Capabilities {
		if strings.TrimSpace(capability) == "" || !validCapabilitySupport(status) {
			return fmt.Errorf("%w: /capabilities has an invalid key or support state", ErrCapabilityProfile)
		}
	}
	for _, combination := range profile.ForbiddenCombinations {
		if len(combination) == 0 || !validProfileNames(combination) {
			return fmt.Errorf("%w: /forbidden_combinations has an empty or invalid combination", ErrCapabilityProfile)
		}
	}
	if !validProfileNames(profile.InputMIME) || !validProfileNames(profile.OutputMIME) {
		return fmt.Errorf("%w: MIME lists have blank or duplicate entries", ErrCapabilityProfile)
	}
	if len(profile.ModelOptionsSchema) != 0 && !validProfileSchema(profile.ModelOptionsSchema) {
		return fmt.Errorf("%w: /model_options_schema must be a JSON Schema object or boolean", ErrCapabilityProfile)
	}
	return nil
}

func validProfileSchema(document JSONDocument) bool {
	trimmed := bytes.TrimSpace(document)
	return json.Valid(trimmed) && (trimmed[0] == '{' || bytes.Equal(trimmed, []byte("true")) ||
		bytes.Equal(trimmed, []byte("false")))
}

func validCapabilitySupport(status CapabilitySupport) bool {
	switch status {
	case CapabilitySupported, CapabilityUnsupported, CapabilityUnknown:
		return true
	default:
		return false
	}
}

func validProfileNames(names []string) bool {
	seen := make(map[string]struct{}, len(names))
	for _, name := range names {
		if strings.TrimSpace(name) == "" {
			return false
		}
		if _, exists := seen[name]; exists {
			return false
		}
		seen[name] = struct{}{}
	}
	return true
}
