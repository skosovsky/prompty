package adapter

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/skosovsky/prompty"
)

// ErrCapabilityRequirement indicates unknown or unsupported required guarantees.
var ErrCapabilityRequirement = errors.New("adapter: execution requirement not satisfied")

var (
	ErrUnknownProfile         = errors.New("adapter: unknown required capability profile")
	ErrUnsupportedCapability  = errors.New("adapter: unsupported required capability")
	ErrUnsupportedCombination = errors.New("adapter: unsupported capability combination")
	ErrInvalidProfileOverride = errors.New("adapter: invalid capability profile override")
	ErrInvalidProfile         = errors.New("adapter: invalid or ambiguous effective capability profile")
)

// CapabilityError carries the actual checked profile and each failing requirement.
type CapabilityError struct {
	Diagnostics prompty.ExecutionDiagnostics
	Cause       error
}

func (e *CapabilityError) Error() string {
	var failures []string
	for _, check := range e.Diagnostics.Checks {
		if check.Status != prompty.CapabilitySupported {
			failures = append(failures, check.Requirement+": "+string(check.Status)+" ("+check.Reason+")")
		}
	}
	return fmt.Sprintf("%v: %s", ErrCapabilityRequirement, strings.Join(failures, "; "))
}

func (e *CapabilityError) Unwrap() []error { return []error{ErrCapabilityRequirement, e.Cause} }

// Preflight checks the effective model, combinations and MIME without network access.
// Native implementation is established by the actual adapter method set, not an override.
func Preflight(
	desc RuntimeDescriptor,
	exec *prompty.PromptExecution,
	mode prompty.StreamMode,
	hasNative bool,
) (prompty.ExecutionDiagnostics, error) {
	report, profile, err := resolveProfile(desc, exec)
	if err != nil {
		return report, err
	}
	reasoning := checkModelOptions(&report, desc, profile, exec)
	required := requiredCapabilities(exec, mode)
	for _, capability := range required {
		report.Checks = append(report.Checks, capabilityCheck(profile, capability, mode, hasNative))
	}
	if reasoning {
		// The options schema checks individual reasoning intent; the same
		// profile's combination rules can forbid tools/output/stream with it.
		required = append(required, prompty.CapabilityReasoning)
	}
	checkCombinations(&report, profile, required)
	checkInputMIME(&report, profile, exec)
	for _, mime := range exec.Requirements.OutputMIME {
		report.Checks = append(report.Checks, mimeCheck("output_mime", mime, profile.OutputMIME, profile.Provenance))
	}
	return report, requirementError(report)
}

func resolveProfile(
	desc RuntimeDescriptor,
	exec *prompty.PromptExecution,
) (prompty.ExecutionDiagnostics, prompty.CapabilityProfile, error) {
	var report prompty.ExecutionDiagnostics
	if err := ValidateExecution(exec); err != nil {
		return report, prompty.CapabilityProfile{}, err
	}
	if err := prompty.ValidateEndpointIdentity(desc.Endpoint); err != nil {
		return report, prompty.CapabilityProfile{}, errors.Join(ErrInvalidProfile, err)
	}
	report.Profile = prompty.ProfileIdentity{
		Provider:    desc.Name,
		Endpoint:    desc.Endpoint,
		Model:       desc.DefaultModel,
		ModelFamily: "",
	}
	if exec.ModelOptions != nil && exec.ModelOptions.Model != "" {
		report.Profile.Model = exec.ModelOptions.Model
	}
	profile, err := selectProfile(desc, exec.Requirements.Override, &report)
	if err != nil {
		return report, profile, err
	}
	report.Provenance = profile.Provenance
	if profile.Identity.ModelFamily != "" {
		report.Profile.ModelFamily = profile.Identity.ModelFamily
	}
	snapshot, err := json.Marshal(struct {
		Identity prompty.ProfileIdentity   `json:"identity"`
		Profile  prompty.CapabilityProfile `json:"profile"`
	}{report.Profile, profile})
	if err != nil {
		return report, profile, fmt.Errorf("%w: snapshot: %w", ErrCapabilityRequirement, err)
	}
	digest := sha256.Sum256(snapshot)
	report.Snapshot = hex.EncodeToString(digest[:])
	return report, profile, nil
}

func selectProfile(
	desc RuntimeDescriptor,
	override *prompty.CapabilityProfile,
	report *prompty.ExecutionDiagnostics,
) (prompty.CapabilityProfile, error) {
	if override != nil {
		if !sameDestination(override.Identity, report.Profile) {
			return prompty.CapabilityProfile{}, invalidProfileError(
				report,
				ErrInvalidProfileOverride,
				"profile_override",
				"destination mismatch",
			)
		}
		if err := prompty.ValidateCapabilityProfile(*override); err != nil {
			return prompty.CapabilityProfile{}, invalidProfileError(report, errors.Join(ErrInvalidProfileOverride, err),
				"profile_override", "malformed profile knowledge")
		}
		report.Override = true
		return prompty.CloneCapabilityProfile(*override), nil
	}
	var profile prompty.CapabilityProfile
	found := false
	for _, candidate := range desc.Profiles {
		if !sameDestination(candidate.Identity, report.Profile) {
			continue
		}
		if found {
			return profile, invalidProfileError(report, ErrInvalidProfile, "profile", "ambiguous destination profiles")
		}
		profile, found = prompty.CloneCapabilityProfile(candidate), true
	}
	if found {
		if err := prompty.ValidateCapabilityProfile(profile); err != nil {
			return profile, invalidProfileError(report, errors.Join(ErrInvalidProfile, err), "profile",
				"malformed profile knowledge")
		}
	}
	return profile, nil
}

func invalidProfileError(report *prompty.ExecutionDiagnostics, cause error, requirement, reason string) error {
	report.Checks = []prompty.RequirementCheck{{
		Requirement: requirement, Status: prompty.CapabilityUnsupported, Reason: reason,
	}}
	return &CapabilityError{Diagnostics: prompty.CloneExecutionDiagnostics(*report), Cause: cause}
}

func requiredCapabilities(exec *prompty.PromptExecution, mode prompty.StreamMode) []string {
	required := slices.Clone(exec.Requirements.Capabilities)
	if exec.CacheResource != nil {
		required = append(required, prompty.CapabilityCacheReuse)
	}
	if len(exec.Tools) != 0 {
		required = append(required, prompty.CapabilityTools)
	}
	if exec.ForcedTool != "" {
		required = append(required, prompty.CapabilityForcedToolSelection)
	}
	if exec.ResponseFormat != nil {
		if exec.Requirements.OutputMode == prompty.OutputSyntheticTool {
			required = append(required, prompty.CapabilitySyntheticOutput,
				prompty.CapabilityTools, prompty.CapabilityForcedToolSelection)
		} else {
			required = append(required, prompty.CapabilityStructuredOutput)
		}
	}
	if mode == prompty.StreamNative {
		required = append(required, prompty.CapabilityNativeStream)
	}
	for _, message := range exec.Messages {
		if len(message.ProviderState) != 0 || message.ContinuationUnavailable {
			required = append(required, prompty.CapabilityProviderState)
			break
		}
	}
	if historyNeedsNameCorrelation(exec.Messages) {
		required = append(required, prompty.CapabilityToolNameCorrelation)
	}
	slices.Sort(required)
	return slices.Compact(required)
}

func capabilityCheck(
	profile prompty.CapabilityProfile,
	capability string,
	mode prompty.StreamMode,
	hasNative bool,
) prompty.RequirementCheck {
	status := profile.Capabilities[capability]
	if status == "" {
		status = prompty.CapabilityUnknown
	}
	reason := ""
	if capability == prompty.CapabilityNativeStream {
		if !hasNative {
			status, reason = prompty.CapabilityUnsupported, "adapter has no native stream implementation"
		} else if mode != prompty.StreamNative {
			status, reason = prompty.CapabilityUnsupported, "execution mode is not native streaming"
		}
	}
	return prompty.RequirementCheck{
		Requirement: capability, Status: status, Reason: reason,
	}
}

func checkCombinations(report *prompty.ExecutionDiagnostics, profile prompty.CapabilityProfile, required []string) {
	for _, combination := range profile.ForbiddenCombinations {
		if len(combination) != 0 && containsAll(required, combination) {
			sorted := slices.Clone(combination)
			slices.Sort(sorted)
			report.Checks = append(report.Checks, prompty.RequirementCheck{
				Requirement: strings.Join(sorted, "+"), Status: prompty.CapabilityUnsupported,
				Reason: "profile forbids capability combination",
			})
		}
	}
}

func requirementError(report prompty.ExecutionDiagnostics) error {
	for _, check := range report.Checks {
		if check.Status != prompty.CapabilitySupported {
			cause := ErrUnsupportedCapability
			if check.Status == prompty.CapabilityUnknown {
				cause = ErrUnknownProfile
			} else if check.Reason == "profile forbids capability combination" {
				cause = ErrUnsupportedCombination
			}
			return &CapabilityError{Diagnostics: prompty.CloneExecutionDiagnostics(report), Cause: cause}
		}
	}
	return nil
}

func sameDestination(a, b prompty.ProfileIdentity) bool {
	return a.Provider == b.Provider && a.Endpoint == b.Endpoint && a.Model == b.Model
}

func containsAll(values, required []string) bool {
	for _, value := range required {
		if !slices.Contains(values, value) {
			return false
		}
	}
	return true
}

func checkInputMIME(
	report *prompty.ExecutionDiagnostics,
	profile prompty.CapabilityProfile,
	exec *prompty.PromptExecution,
) {
	var visit func([]prompty.ContentPart)
	visit = func(parts []prompty.ContentPart) {
		for _, part := range parts {
			switch value := part.(type) {
			case prompty.MediaPart:
				report.Checks = append(
					report.Checks,
					mimeCheck("input_mime", value.MIMEType, profile.InputMIME, profile.Provenance),
				)
			case *prompty.MediaPart:
				if value != nil {
					report.Checks = append(
						report.Checks,
						mimeCheck("input_mime", value.MIMEType, profile.InputMIME, profile.Provenance),
					)
				}
			case prompty.ToolResultPart:
				visit(value.Content)
			case *prompty.ToolResultPart:
				if value != nil {
					visit(value.Content)
				}
			}
		}
	}
	for _, message := range exec.Messages {
		visit(message.Content)
	}
}

func mimeCheck(direction, mime string, allowed []string, provenance string) prompty.RequirementCheck {
	status := prompty.CapabilityUnsupported
	if provenance == "" {
		status = prompty.CapabilityUnknown
	} else if slices.Contains(allowed, mime) && mime != "" {
		status = prompty.CapabilitySupported
	}
	return prompty.RequirementCheck{Requirement: direction + ":" + mime, Status: status, Reason: ""}
}

func historyNeedsNameCorrelation(messages []prompty.ChatMessage) bool {
	return slices.ContainsFunc(messages, func(message prompty.ChatMessage) bool {
		return slices.ContainsFunc(message.Content, partNeedsNameCorrelation)
	})
}
func partNeedsNameCorrelation(part prompty.ContentPart) bool {
	switch value := part.(type) {
	case prompty.ToolCallPart:
		return value.ID == ""
	case *prompty.ToolCallPart:
		return value != nil && value.ID == ""
	case prompty.ToolResultPart:
		return value.ToolCallID == ""
	case *prompty.ToolResultPart:
		return value != nil && value.ToolCallID == ""
	default:
		return false
	}
}
