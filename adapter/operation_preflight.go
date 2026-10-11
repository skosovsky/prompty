package adapter

import (
	"encoding/json"
	"slices"

	"github.com/skosovsky/prompty"
)

// PreflightOperation checks a caller-driven remote operation, not generation.
// Model selects the destination; no generation options are synthesized or replayed.
// The snapshot retains the full effective profile, including generation contracts.
// Actual operation ports, handle bindings and original result validation are still
// the adapter's responsibility; profile knowledge cannot create implementation.
func PreflightOperation(desc RuntimeDescriptor, model string, capabilities []string,
	override *prompty.CapabilityProfile,
) (prompty.ExecutionDiagnostics, error) {
	exec := prompty.SimplePrompt("")
	exec.ModelOptions = &prompty.ModelOptions{
		Temperature:      nil,
		MaxTokens:        nil,
		TopP:             nil,
		Stop:             nil,
		ProviderSettings: nil,
		Model:            model,
	}
	exec.Requirements.Override = override
	report, profile, err := resolveProfile(desc, exec)
	if err != nil {
		return report, err
	}
	checkOperationCapabilities(&report, profile, capabilities)
	return report, requirementError(report)
}

// PreflightInputOperation checks canonical input for a non-generation resource
// operation. It retains inferred tools/state and MIME checks, but accepts only a
// model selector in ModelOptions, never generation settings or output intent.
// Actual resource mapping and original schema/state validation remain adapter-owned.
func PreflightInputOperation(
	desc RuntimeDescriptor,
	exec *prompty.PromptExecution,
) (prompty.ExecutionDiagnostics, error) {
	report, profile, err := resolveProfile(desc, exec)
	if err != nil {
		return report, err
	}
	if !operationModelOnly(exec.ModelOptions) || exec.ResponseFormat != nil || exec.Requirements.OutputMode != "" ||
		len(exec.Requirements.OutputMIME) != 0 {
		appendOptionsCheck(
			&report,
			prompty.CapabilityUnsupported,
			"resource input does not accept generation settings or output",
		)
		return report, requirementError(report)
	}
	checkOperationCapabilities(&report, profile, requiredCapabilities(exec, ""))
	checkInputMIME(&report, profile, exec)
	return report, requirementError(report)
}

func operationModelOnly(options *prompty.ModelOptions) bool {
	if options == nil {
		return true
	}
	encoded, err := json.Marshal(options)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(encoded, &fields) != nil {
		return false
	}
	delete(fields, "model")
	return len(fields) == 0
}

func checkOperationCapabilities(report *prompty.ExecutionDiagnostics, profile prompty.CapabilityProfile,
	capabilities []string,
) {
	required := slices.Clone(capabilities)
	slices.Sort(required)
	required = slices.Compact(required)
	for _, capability := range required {
		report.Checks = append(report.Checks, capabilityCheck(profile, capability, "", false))
	}
	checkCombinations(report, profile, required)
}
