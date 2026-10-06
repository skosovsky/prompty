package adapter

import (
	"errors"
	"fmt"
	"slices"

	"github.com/skosovsky/prompty"
)

// RejectUnmappedCache protects low-level translators without block controls.
// Advisory hints are outside the raw transport contract; required intent cannot
// be silently ignored. Checked preflight reports and removes advisory policies.
func RejectUnmappedCache(exec *prompty.PromptExecution) error {
	return prompty.RewriteCachePolicies(
		exec.Clone().Messages,
		func(path string, policy *prompty.CachePolicy) (bool, error) {
			if policy.Mode == prompty.CacheRequired {
				return false, &prompty.CacheError{Decision: prompty.CacheDecision{
					Path: path, Mechanism: policy.Mechanism, Disposition: prompty.CacheUnknown,
					Reason: "required cache intent needs checked preflight",
				}, Cause: prompty.ErrCacheRequirement}
			}
			return true, nil
		},
	)
}

// CacheControlMapper validates provider controls without network activity. The
// provider's Translate must map every accepted control. Profile knowledge alone
// cannot implement this port.
type CacheControlMapper interface {
	ValidateCacheControl(policy *prompty.CachePolicy) error
}

func prepareCache(
	adp any,
	exec *prompty.PromptExecution,
	profile prompty.CapabilityProfile,
	features []string,
) ([]prompty.CacheDecision, error) {
	shadowed, err := shadowedMessageCachePolicies(exec.Messages)
	if err != nil {
		return nil, err
	}
	var decisions []prompty.CacheDecision
	var selected []selectedCachePolicy
	if policy := exec.RequestCache; policy != nil {
		decision := requestCacheDecision(adp, profile, policy)
		decisions = append(decisions, decision)
		switch {
		case decision.Disposition == prompty.CacheAccepted:
			selected = append(selected, selectedCachePolicy{
				path:     requestCachePath,
				required: policy.Mode == prompty.CacheRequired,
				features: cachePolicyFeatures(policy),
			})
		case policy.Mode == prompty.CacheRequired:
			return decisions, &prompty.CacheError{Decision: decision, Cause: prompty.ErrCacheRequirement}
		default:
			exec.RequestCache = nil
		}
	}
	err = prompty.RewriteCachePolicies(
		exec.Messages,
		func(path string, policy *prompty.CachePolicy) (bool, error) {
			if shadowed[path] {
				decisions = append(decisions, prompty.CacheDecision{
					Path: path, Mechanism: policy.Mechanism, Disposition: prompty.CacheNotApplicable,
					Reason: "no content block inherits this message default",
				})
				omitCachePolicy(policy)
				return true, nil
			}
			decision := cacheDecision(adp, profile, path, policy)
			decisions = append(decisions, decision)
			if decision.Disposition == prompty.CacheAccepted {
				selected = append(selected, selectedCachePolicy{
					path: path, required: policy.Mode == prompty.CacheRequired, features: cachePolicyFeatures(policy),
				})
				if policy.Mechanism == prompty.CacheImplicitPrefix {
					omitCachePolicy(policy) // No block field; do not reactivate an inherited marker.
				}
				return true, nil
			}
			if policy.Mode == prompty.CacheRequired {
				return false, &prompty.CacheError{Decision: decision, Cause: prompty.ErrCacheRequirement}
			}
			omitCachePolicy(policy)
			return true, nil
		},
	)
	if err != nil {
		return decisions, err
	}
	return reconcileCacheCombinations(exec, profile, features, decisions, selected)
}

func shadowedMessageCachePolicies(messages []prompty.ChatMessage) (map[string]bool, error) {
	shadowed := make(map[string]bool)
	for index, message := range messages {
		if message.CachePolicy == nil {
			continue
		}
		inherited, err := prompty.MessageInheritsCachePolicy(message)
		if err != nil {
			return nil, err
		}
		if !inherited {
			shadowed[fmt.Sprintf("/messages/%d/cache_policy", index)] = true
		}
	}
	return shadowed, nil
}

func cacheDecision(
	adp any,
	profile prompty.CapabilityProfile,
	path string,
	policy *prompty.CachePolicy,
) prompty.CacheDecision {
	decision := prompty.CacheDecision{
		Path: path, Mechanism: policy.Mechanism, Disposition: prompty.CacheUnknown, Reason: "profile knowledge missing",
	}
	if policy.Mechanism == prompty.CacheOmit {
		decision.Disposition, decision.Reason = prompty.CacheNotApplicable, "block marker explicitly omitted"
		return decision
	}
	if policy.Provider != "" && policy.Provider != profile.Identity.Provider {
		decision.Disposition, decision.Reason = prompty.CacheNotApplicable, "provider scope differs"
		return decision
	}
	if policy.Mechanism == prompty.CacheBlockMarker {
		mapper, ok := adp.(CacheControlMapper)
		if !ok {
			decision.Disposition, decision.Reason = prompty.CacheUnsupported, "adapter has no block marker mapper"
			return decision
		}
		isolated := *policy
		isolated.Controls = prompty.CloneJSONDocument(policy.Controls)
		if err := mapper.ValidateCacheControl(&isolated); err != nil {
			decision.Disposition, decision.Reason = prompty.CacheUnsupported, "provider controls unsupported"
			return decision
		}
	} else if len(policy.Controls) != 0 {
		decision.Disposition, decision.Reason = prompty.CacheUnsupported, "implicit controls have no request mapper"
		return decision
	}
	keys := cachePolicyFeatures(policy)
	for _, key := range keys {
		support := profile.Capabilities[key]
		if support != prompty.CapabilitySupported {
			if support == prompty.CapabilityUnsupported {
				decision.Disposition = prompty.CacheUnsupported
			}
			decision.Reason = "profile knowledge missing or unsupported for requested cache controls"
			return decision
		}
	}
	decision.Disposition, decision.Reason = prompty.CacheAccepted, "selected request controls; reuse is not guaranteed"
	return decision
}

type selectedCachePolicy struct {
	path     string
	required bool
	features []string
}

func cachePolicyFeatures(policy *prompty.CachePolicy) []string {
	capability := prompty.CapabilityCacheImplicit
	if policy.Mechanism == prompty.CacheBlockMarker {
		capability = prompty.CapabilityCacheMarker
	}
	keys := []string{capability}
	controls, _ := prompty.JSONDocumentAsMap(policy.Controls) // Shape was checked before this call.
	for key := range controls {
		keys = append(keys, "cache_control:"+key)
	}
	slices.Sort(keys)
	return keys
}

func reconcileCacheCombinations(
	exec *prompty.PromptExecution,
	profile prompty.CapabilityProfile,
	features []string,
	decisions []prompty.CacheDecision,
	selected []selectedCachePolicy,
) ([]prompty.CacheDecision, error) {
	blocked := reconcileSelectedCachePolicies(profile.ForbiddenCombinations, features, selected)
	if len(blocked) == 0 {
		return decisions, nil
	}
	for index := range decisions {
		if blocked[decisions[index].Path] {
			decisions[index].Disposition = prompty.CacheUnsupported
			decisions[index].Reason = "profile forbids requested cache combination"
		}
	}
	for _, policy := range selected {
		if policy.required && blocked[policy.path] {
			for _, decision := range decisions {
				if decision.Path == policy.path {
					return decisions, &prompty.CacheError{
						Decision: decision, Cause: errors.Join(prompty.ErrCacheRequirement, ErrUnsupportedCombination),
					}
				}
			}
		}
	}
	if blocked[requestCachePath] {
		exec.RequestCache = nil
	}
	err := prompty.RewriteCachePolicies(exec.Messages, func(path string, policy *prompty.CachePolicy) (bool, error) {
		if blocked[path] {
			omitCachePolicy(policy)
		}
		return true, nil
	})
	return decisions, err
}

func omitCachePolicy(policy *prompty.CachePolicy) {
	*policy = prompty.CachePolicy{Mechanism: prompty.CacheOmit, Mode: "", Provider: "", Controls: nil}
}

func reconcileSelectedCachePolicies(
	combinations [][]string,
	features []string,
	selected []selectedCachePolicy,
) map[string]bool {
	active := cacheActiveFeatures(features, selected)
	initial := blockedCachePolicies(combinations, active, selected)
	blocked := make(map[string]bool)
	var remaining []selectedCachePolicy
	for _, policy := range selected {
		if !policy.required && initial[policy.path] {
			blocked[policy.path] = true
		} else {
			remaining = append(remaining, policy)
		}
	}
	active = cacheActiveFeatures(features, remaining)
	for path := range blockedCachePolicies(combinations, active, remaining) {
		blocked[path] = true
	}
	return blocked
}

func cacheActiveFeatures(features []string, selected []selectedCachePolicy) []string {
	active := slices.Clone(features)
	for _, policy := range selected {
		active = append(active, policy.features...)
	}
	return active
}

func blockedCachePolicies(combinations [][]string, active []string, selected []selectedCachePolicy) map[string]bool {
	blocked := make(map[string]bool)
	for _, combination := range combinations {
		if len(combination) == 0 || !containsAll(active, combination) {
			continue
		}
		for _, policy := range selected {
			for _, feature := range policy.features {
				if slices.Contains(combination, feature) {
					blocked[policy.path] = true
				}
			}
		}
	}
	return blocked
}

func cacheCombinationFeatures(desc RuntimeDescriptor, exec *prompty.PromptExecution, mode prompty.StreamMode) []string {
	features := requiredCapabilities(exec, mode)
	settings, _ := prompty.ProviderSettingsMap(exec.ModelOptions) // Already checked by Preflight.
	for key := range settings {
		if slices.Contains(desc.ReasoningSettings, key) {
			return append(features, prompty.CapabilityReasoning)
		}
	}
	return features
}
