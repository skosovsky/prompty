package prompty

import (
	"maps"
	"slices"
)

// CapabilitySupport is knowledge about a profile, not a quality or availability promise.
type CapabilitySupport string

const (
	CapabilitySupported           CapabilitySupport = "supported"
	CapabilityUnsupported         CapabilitySupport = "unsupported"
	CapabilityUnknown             CapabilitySupport = "unknown"
	CapabilityTools                                 = "tools"
	CapabilityForcedToolSelection                   = "forced_tool_selection"
	// CapabilityToolNameCorrelation permits complete ID-less calls with unique
	// names in one batch, when the adapter preserves those names in tool results.
	CapabilityToolNameCorrelation = "tool_name_correlation"
	CapabilityNativeStream        = "native_stream"
	CapabilityStructuredOutput    = "native_json_schema"
	CapabilitySyntheticOutput     = "synthetic_tool_output"
	CapabilityUsage               = "usage"
	CapabilityProviderState       = "provider_state"
	CapabilityReasoning           = "reasoning"
)

// ProfileIdentity describes the effective provider destination without SDK types.
type ProfileIdentity struct {
	Provider    string `json:"provider"`
	Endpoint    string `json:"endpoint"`
	Model       string `json:"model"`
	ModelFamily string `json:"model_family,omitempty"`
}

// CapabilityProfile is a plain-data, provenance-bearing contract snapshot.
type CapabilityProfile struct {
	Identity              ProfileIdentity              `json:"identity"`
	Provenance            string                       `json:"provenance"`
	Capabilities          map[string]CapabilitySupport `json:"capabilities"`
	ForbiddenCombinations [][]string                   `json:"forbidden_combinations,omitempty"`
	InputMIME             []string                     `json:"input_mime,omitempty"`
	OutputMIME            []string                     `json:"output_mime,omitempty"`
	ModelOptionsSchema    JSONDocument                 `json:"model_options_schema,omitempty"`
}

// ExecutionRequirements contains caller guarantees and an explicit checked profile override.
type ExecutionRequirements struct {
	OutputMode   StructuredOutputMode `json:"output_mode,omitempty"`
	Capabilities []string             `json:"capabilities,omitempty"`
	OutputMIME   []string             `json:"output_mime,omitempty"`
	Override     *CapabilityProfile   `json:"override,omitempty"`
	Schema       SchemaPolicy         `json:"schema,omitzero"`
	Capture      CaptureRequirement   `json:"capture,omitzero"`
}

// RequirementCheck explains one profile decision without prompt or credentials.
type RequirementCheck struct {
	Requirement string            `json:"requirement"`
	Status      CapabilitySupport `json:"status"`
	Reason      string            `json:"reason,omitempty"`
}

// ExecutionDiagnostics preserves the effective checked profile and provenance.
type ExecutionDiagnostics struct {
	Profile    ProfileIdentity           `json:"profile"`
	Snapshot   string                    `json:"snapshot"`
	Provenance string                    `json:"provenance,omitempty"`
	Override   bool                      `json:"override,omitempty"`
	Checks     []RequirementCheck        `json:"checks,omitempty"`
	Schemas    []SchemaCompilationReport `json:"schemas,omitempty"`
	Capture    *CaptureReport            `json:"capture,omitempty"`
	Cache      []CacheDecision           `json:"cache,omitempty"`
}

// CloneExecutionDiagnostics isolates the mutable checks of a prepared snapshot.
func CloneExecutionDiagnostics(report ExecutionDiagnostics) ExecutionDiagnostics {
	out := report
	out.Checks = slices.Clone(report.Checks)
	out.Cache = slices.Clone(report.Cache)
	out.Schemas = slices.Clone(report.Schemas)
	if report.Capture != nil {
		capture := CloneCaptureReport(*report.Capture)
		out.Capture = &capture
	}
	for i := range out.Schemas {
		out.Schemas[i].Transformations = slices.Clone(report.Schemas[i].Transformations)
	}
	return out
}

// CloneCapabilityProfile isolates all mutable snapshot data.
func CloneCapabilityProfile(profile CapabilityProfile) CapabilityProfile {
	out := profile
	out.Capabilities = maps.Clone(profile.Capabilities)
	out.ForbiddenCombinations = slices.Clone(profile.ForbiddenCombinations)
	for i, combination := range profile.ForbiddenCombinations {
		out.ForbiddenCombinations[i] = slices.Clone(combination)
	}
	out.InputMIME = slices.Clone(profile.InputMIME)
	out.OutputMIME = slices.Clone(profile.OutputMIME)
	out.ModelOptionsSchema = CloneJSONDocument(profile.ModelOptionsSchema)
	return out
}

func cloneRequirements(requirements ExecutionRequirements) ExecutionRequirements {
	out := requirements
	out.Capabilities = slices.Clone(requirements.Capabilities)
	out.OutputMIME = slices.Clone(requirements.OutputMIME)
	out.Schema.AllowedTransforms = slices.Clone(requirements.Schema.AllowedTransforms)
	out.Capture.Stages = slices.Clone(requirements.Capture.Stages)
	if requirements.Override != nil {
		profile := CloneCapabilityProfile(*requirements.Override)
		out.Override = &profile
	}
	return out
}
