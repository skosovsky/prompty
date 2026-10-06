package adapter

import "github.com/skosovsky/prompty"

const requestCachePath = "/request_cache"

// RequestCacheMapper promises validation and actual whole-request control
// translation. Profile knowledge cannot supply this optional mapper port.
type RequestCacheMapper interface {
	ValidateRequestCacheControl(*prompty.CachePolicy) error
}

func requestCacheDecision(
	adp any, profile prompty.CapabilityProfile, policy *prompty.CachePolicy,
) prompty.CacheDecision {
	decision := prompty.CacheDecision{
		Path: requestCachePath, Mechanism: policy.Mechanism,
		Disposition: prompty.CacheUnknown, Reason: "profile knowledge missing",
	}
	if policy.Provider != "" && policy.Provider != profile.Identity.Provider {
		decision.Disposition, decision.Reason = prompty.CacheNotApplicable, "provider scope differs"
		return decision
	}
	mapper, ok := adp.(RequestCacheMapper)
	if !ok {
		decision.Disposition, decision.Reason = prompty.CacheUnsupported, "adapter has no request cache mapper"
		return decision
	}
	isolated := *policy
	isolated.Controls = prompty.CloneJSONDocument(policy.Controls)
	if err := mapper.ValidateRequestCacheControl(&isolated); err != nil {
		decision.Disposition, decision.Reason = prompty.CacheUnsupported, "request cache controls unsupported"
		return decision
	}
	for _, key := range cachePolicyFeatures(policy) {
		if support := profile.Capabilities[key]; support != prompty.CapabilitySupported {
			if support == prompty.CapabilityUnsupported {
				decision.Disposition = prompty.CacheUnsupported
			}
			return decision
		}
	}
	decision.Disposition, decision.Reason = prompty.CacheAccepted, "selected request controls; reuse is not guaranteed"
	return decision
}

// PrepareCacheControls mutates an isolated working execution for operation
// adapters that do not use Prepare. It applies profile/port/disposition and
// combination checks without generation dispatch. Preserve caller source first.
func PrepareCacheControls(
	adp RuntimeDescriber, exec *prompty.PromptExecution, extraFeatures []string,
) ([]prompty.CacheDecision, error) {
	if err := ValidateExecution(exec); err != nil {
		return nil, err
	}
	desc := CloneRuntimeDescriptor(adp.RuntimeDescriptor())
	_, profile, err := resolveProfile(desc, exec)
	if err != nil {
		return nil, err
	}
	features := append(cacheCombinationFeatures(desc, exec, ""), extraFeatures...)
	return prepareCache(adp, exec, profile, features)
}

// RejectRequestCache prevents unsupported raw translators from silently mapping
// request-level cache intent. Checked advisory intent is removed before Translate.
func RejectRequestCache(exec *prompty.PromptExecution) error {
	if exec.RequestCache == nil {
		return nil
	}
	return &prompty.CacheError{Decision: prompty.CacheDecision{
		Path: requestCachePath, Mechanism: exec.RequestCache.Mechanism,
		Disposition: prompty.CacheUnsupported, Reason: "adapter has no request cache mapper",
	}, Cause: prompty.ErrCacheRequirement}
}
