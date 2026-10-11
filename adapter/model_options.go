package adapter

import (
	"encoding/json"
	"slices"

	"github.com/skosovsky/prompty"
)

func checkModelOptions(
	report *prompty.ExecutionDiagnostics,
	desc RuntimeDescriptor,
	profile prompty.CapabilityProfile,
	exec *prompty.PromptExecution,
) bool {
	settings, err := prompty.ProviderSettingsMap(exec.ModelOptions)
	if err != nil || exec.ModelOptions != nil && len(exec.ModelOptions.ProviderSettings) != 0 && settings == nil {
		appendOptionsCheck(report, prompty.CapabilityUnsupported, "invalid provider settings object")
		return false
	}
	reasoning := false
	for key := range settings {
		if slices.Contains(desc.ReasoningSettings, key) {
			reasoning = true
			break
		}
	}
	if len(profile.ModelOptionsSchema) == 0 {
		if reasoning {
			appendOptionsCheck(
				report,
				prompty.CapabilityUnknown,
				"reasoning settings require an effective model options schema",
			)
		}
		return reasoning
	}
	validator := exec.Validator
	if validator == nil {
		validator = prompty.NewJSONSchemaValidator(nil)
	}
	compiled, err := validator.CompileSchema(
		prompty.CloneJSONDocument(profile.ModelOptionsSchema),
		prompty.SchemaDialect2020,
	)
	if err != nil || compiled == nil {
		appendOptionsCheck(report, prompty.CapabilityUnsupported, "model options schema cannot be compiled")
		return reasoning
	}
	options := exec.ModelOptions
	if options == nil {
		options = new(prompty.ModelOptions)
	}
	document, err := json.Marshal(options)
	if err != nil || compiled.ValidateJSON(document) != nil {
		appendOptionsCheck(
			report,
			prompty.CapabilityUnsupported,
			"model options violate the effective profile contract",
		)
		return reasoning
	}
	appendOptionsCheck(report, prompty.CapabilitySupported, "model options satisfy the effective profile contract")
	return reasoning
}

func appendOptionsCheck(report *prompty.ExecutionDiagnostics, status prompty.CapabilitySupport, reason string) {
	report.Checks = append(
		report.Checks,
		prompty.RequirementCheck{Requirement: "model_options", Status: status, Reason: reason},
	)
}
