package gemini

import (
	"errors"
	"slices"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// CapabilityIncrementalToolArguments declares endpoint/model knowledge for
// Gemini's path/value function-call protocol, separately from native streaming.
const CapabilityIncrementalToolArguments = "gemini.partial_args"

func (a *Adapter) applyPartialControl(
	config *genai.GenerateContentConfig,
	exec *prompty.PromptExecution,
	settings map[string]any,
) error {
	required := slices.Contains(exec.Requirements.Capabilities, CapabilityIncrementalToolArguments)
	enabled := required
	if raw, present := settings["stream_function_call_arguments"]; present {
		value, ok := raw.(bool)
		if !ok || required && !value {
			return adapter.ProviderSettingError(
				"stream_function_call_arguments",
				errors.New("invalid or conflicting boolean control"),
			)
		}
		enabled = value
	}
	if !enabled {
		return nil
	}
	if !required || len(exec.Tools) == 0 {
		return adapter.ProviderSettingError("stream_function_call_arguments", adapter.ErrCapabilityRequirement)
	}
	// The installed SDK has no MLDev request mapper for this field. Profile
	// knowledge/overrides cannot invent one; inspect only public SDK config.
	if a.client != nil && a.client.ClientConfig().Backend == genai.BackendGeminiAPI {
		return adapter.ProviderSettingError("stream_function_call_arguments", adapter.ErrUnsupportedCapability)
	}
	if config.ToolConfig == nil {
		config.ToolConfig = &genai.ToolConfig{} //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
	}
	if config.ToolConfig.FunctionCallingConfig == nil {
		config.ToolConfig.FunctionCallingConfig = &genai.FunctionCallingConfig{} //nolint:exhaustruct_v5 // SDK DTO: omitted optional fields and other union arms retain their zero values.
	}
	config.ToolConfig.FunctionCallingConfig.StreamFunctionCallArguments = new(true)
	return nil
}

func requestsPartialArguments(req *Request) bool {
	return req != nil && req.Config != nil && req.Config.ToolConfig != nil &&
		req.Config.ToolConfig.FunctionCallingConfig != nil &&
		req.Config.ToolConfig.FunctionCallingConfig.StreamFunctionCallArguments != nil &&
		*req.Config.ToolConfig.FunctionCallingConfig.StreamFunctionCallArguments
}
