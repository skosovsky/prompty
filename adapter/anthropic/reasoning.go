package anthropic

import (
	"errors"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/cast"
)

const minThinkingBudget int64 = 1024

func applyThinkingSettings(params *anthropic.MessageNewParams, settings map[string]any) error {
	mode, modeSet := settings["thinking_mode"]
	budget, budgetSet := settings["thinking_budget"]
	display, displaySet := settings["thinking_visibility"]
	if !modeSet {
		if budgetSet || displaySet {
			return adapter.ProviderSettingError("thinking_mode", errors.New("mode required with budget or visibility"))
		}
		return applyThinkingEffort(params, settings)
	}
	if mode != "enabled" && budgetSet {
		return adapter.ProviderSettingError("thinking_budget", errors.New("budget requires enabled mode"))
	}
	switch mode {
	case "enabled":
		value, ok := cast.ToInt64(budget)
		if !ok || value < minThinkingBudget || value >= params.MaxTokens {
			return adapter.ProviderSettingError(
				"thinking_budget",
				errors.New("integer budget must be at least 1024 and below max_tokens"),
			)
		}
		params.Thinking = anthropic.ThinkingConfigParamOfEnabled(value)
	case "adaptive":
		value := anthropic.NewThinkingConfigAdaptiveParam()
		params.Thinking.OfAdaptive = &value
	case "disabled":
		if displaySet {
			return adapter.ProviderSettingError("thinking_visibility", errors.New("visibility requires thinking"))
		}
		value := anthropic.NewThinkingConfigDisabledParam()
		params.Thinking.OfDisabled = &value
	default:
		return adapter.ProviderSettingError("thinking_mode", errors.New("unsupported mode"))
	}
	if displaySet {
		if display != "summarized" && display != "omitted" {
			return adapter.ProviderSettingError("thinking_visibility", errors.New("unsupported visibility"))
		}
		extra := map[string]any{"display": display}
		if params.Thinking.OfEnabled != nil {
			params.Thinking.OfEnabled.SetExtraFields(extra)
		} else {
			params.Thinking.OfAdaptive.SetExtraFields(extra)
		}
	}
	return applyThinkingEffort(params, settings)
}

func applyThinkingEffort(params *anthropic.MessageNewParams, settings map[string]any) error {
	value, present := settings["reasoning_effort"]
	if !present {
		return nil
	}
	switch value {
	case "low":
		params.OutputConfig.Effort = anthropic.OutputConfigEffortLow
	case "medium":
		params.OutputConfig.Effort = anthropic.OutputConfigEffortMedium
	case "high":
		params.OutputConfig.Effort = anthropic.OutputConfigEffortHigh
	case "max":
		params.OutputConfig.Effort = anthropic.OutputConfigEffortMax
	default:
		return adapter.ProviderSettingError("reasoning_effort", errors.New("unsupported effort"))
	}
	return nil
}
