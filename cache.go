package prompty

import (
	"bytes"
	"errors"
	"fmt"
	"slices"
)

// CacheMechanism identifies request controls, not a cache hit or remote resource.
type CacheMechanism string

const (
	CacheImplicitPrefix     CacheMechanism = "implicit_prefix"
	CacheBlockMarker        CacheMechanism = "block_marker"
	CacheAdvisory                          = "advisory"
	CacheRequired                          = "required"
	CapabilityCacheImplicit                = "cache_implicit_prefix"
	CapabilityCacheMarker                  = "cache_block_marker"
)

// CacheOmit suppresses inherited block markers, not implicit caching or remote reuse.
const CacheOmit CacheMechanism = "omit"

// CachePolicy replaces unscoped Type hints. Controls are provider-specific JSON
// (for example {"ttl":"1h"}), never portable retention or deletion guarantees.
// Empty Mode is advisory. A provider is mandatory when Controls are present.
// CacheOmit has no Mode, Provider or Controls and suppresses block inheritance only.
type CachePolicy struct {
	Mechanism CacheMechanism `json:"mechanism"          yaml:"mechanism"`
	Mode      string         `json:"mode,omitempty"     yaml:"mode,omitempty"`
	Provider  string         `json:"provider,omitempty" yaml:"provider,omitempty"`
	Controls  JSONDocument   `json:"controls,omitempty" yaml:"controls,omitempty"`
}

// CacheDisposition describes selected request controls, never observed reuse.
type CacheDisposition string

const (
	CacheAccepted      CacheDisposition = "accepted"
	CacheUnsupported   CacheDisposition = "unsupported"
	CacheNotApplicable CacheDisposition = "not_applicable"
	CacheUnknown       CacheDisposition = "unknown"
)

// CacheDecision is a safe preflight diagnostic, with no raw control values.
type CacheDecision struct {
	Path        string           `json:"path"`
	Mechanism   CacheMechanism   `json:"mechanism"`
	Disposition CacheDisposition `json:"disposition"`
	Reason      string           `json:"reason"`
}

var (
	ErrCachePolicy      = errors.New("prompty: invalid cache policy")
	ErrCacheRequirement = errors.New("prompty: cache requirement not satisfied")
)

// CacheError identifies a rejected policy without leaking provider options.
type CacheError struct {
	Decision CacheDecision
	Cause    error
}

func (e *CacheError) Error() string {
	return fmt.Sprintf("prompty: cache policy at %s: %s", e.Decision.Path, e.Decision.Disposition)
}

func (e *CacheError) Unwrap() error { return e.Cause }

// EqualCachePolicy compares all intent and scoped controls. Different JSON
// encodings conservatively prevent merging, even if semantically equivalent.
func EqualCachePolicy(a, b *CachePolicy) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Mechanism == b.Mechanism && a.Mode == b.Mode && a.Provider == b.Provider &&
		bytes.Equal(a.Controls, b.Controls)
}

// MessageInheritsCachePolicy reports whether any top-level block uses the message default.
// Empty content or an explicit policy on every block means false. A tool-result
// block itself inherits independently of its children. This does not establish
// provider support for a part type and does not mutate the message.
func MessageInheritsCachePolicy(message ChatMessage) (bool, error) {
	if message.CachePolicy == nil {
		return false, nil
	}
	parts, err := contentPartsToWire(message.Content)
	if err != nil {
		return false, err
	}
	for _, part := range parts {
		if part.CachePolicy == nil {
			return true, nil
		}
	}
	return false, nil
}

// ValidateCachePolicy checks portable shape without interpreting provider keys.
func ValidateCachePolicy(policy *CachePolicy) error {
	if policy == nil {
		return nil
	}
	if policy.Mechanism == CacheOmit {
		if policy.Mode != "" || policy.Provider != "" || len(policy.Controls) != 0 {
			return ErrCachePolicy
		}
		return nil
	}
	if policy.Mechanism != CacheImplicitPrefix && policy.Mechanism != CacheBlockMarker {
		return ErrCachePolicy
	}
	if policy.Mode != "" && policy.Mode != CacheAdvisory && policy.Mode != CacheRequired {
		return ErrCachePolicy
	}
	if len(policy.Controls) > 0 {
		if policy.Provider == "" {
			return ErrCachePolicy
		}
		controls, err := JSONDocumentAsMap(policy.Controls)
		if err != nil || controls == nil {
			return ErrCachePolicy
		}
	}
	return nil
}

// ValidateRequestCachePolicy checks whole-request shape, not provider knob support.
func ValidateRequestCachePolicy(policy *CachePolicy) error {
	if err := ValidateCachePolicy(policy); err != nil {
		return err
	}
	if policy != nil && policy.Mechanism != CacheImplicitPrefix {
		return ErrCachePolicy
	}
	return nil
}

// ValidateCachePolicies checks every message/part policy without mutating input.
func ValidateCachePolicies(messages []ChatMessage) error {
	return RewriteCachePolicies(slices.Clone(messages), func(_ string, _ *CachePolicy) (bool, error) {
		return true, nil
	})
}

// RewriteCachePolicies visits message and nested part policies at stable JSON
// pointer paths. It mutates caller-owned messages; use a clone for preflight.
// Returned part values use the canonical value representation, preserving order
// and associations. The visitor returns true to keep a policy, false to omit it,
// and must not retain or mutate unrelated content.
func RewriteCachePolicies(
	messages []ChatMessage,
	visit func(string, *CachePolicy) (bool, error),
) error {
	for i := range messages {
		message := &messages[i]
		path := fmt.Sprintf("/messages/%d", i)
		policy, err := visitCachePolicy(path+"/cache_policy", message.CachePolicy, visit)
		if err != nil {
			return err
		}
		message.CachePolicy = policy
		parts, err := contentPartsToWire(message.Content)
		if err != nil {
			return err
		}
		if err = rewritePartCache(parts, path+"/content", visit); err != nil {
			return err
		}
		message.Content, err = contentPartsFromWire(parts)
		if err != nil {
			return err
		}
	}
	return nil
}

func rewritePartCache(
	parts []ContentPartWire,
	path string,
	visit func(string, *CachePolicy) (bool, error),
) error {
	for i := range parts {
		part := &parts[i]
		partPath := fmt.Sprintf("%s/%d", path, i)
		policy, err := visitCachePolicy(partPath+"/cache_policy", part.CachePolicy, visit)
		if err != nil {
			return err
		}
		part.CachePolicy = policy
		if err = rewritePartCache(part.Content, partPath+"/content", visit); err != nil {
			return err
		}
	}
	return nil
}

func visitCachePolicy(
	path string,
	policy *CachePolicy,
	visit func(string, *CachePolicy) (bool, error),
) (*CachePolicy, error) {
	if policy == nil {
		return policy, nil
	}
	if err := ValidateCachePolicy(policy); err != nil {
		return nil, &CacheError{Decision: CacheDecision{
			Path: path, Mechanism: policy.Mechanism, Disposition: CacheUnsupported, Reason: "invalid policy shape",
		}, Cause: err}
	}
	selected := cloneCachePolicy(policy)
	keep, err := visit(path, selected)
	if !keep {
		selected = nil
	}
	return selected, err
}
