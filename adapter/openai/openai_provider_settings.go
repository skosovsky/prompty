package openai

import (
	"fmt"
	"strings"

	"github.com/openai/openai-go/v3"

	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/internal/cast"
)

func openAIProviderSettingKeys() []string {
	return []string{
		"presence_penalty",
		"frequency_penalty",
		"seed",
		"logprobs",
		"top_logprobs",
		"reasoning_effort",
	}
}

func applyOpenAIProviderSettings(params *openai.ChatCompletionNewParams, settings map[string]any) (bool, error) {
	if len(settings) == 0 {
		return false, nil
	}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, openAIProviderSettingKeys()); err != nil {
		return false, err
	}
	if raw, ok := settings["presence_penalty"]; ok {
		penalty, err := cast.ToFloat32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("presence_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("presence_penalty", penalty, -2, 2); err != nil {
			return false, err
		}
		params.PresencePenalty = openai.Float(float64(penalty))
	}
	if raw, ok := settings["frequency_penalty"]; ok {
		penalty, err := cast.ToFloat32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("frequency_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("frequency_penalty", penalty, -2, 2); err != nil {
			return false, err
		}
		params.FrequencyPenalty = openai.Float(float64(penalty))
	}
	if raw, ok := settings["seed"]; ok {
		seed, err := cast.ToInt32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("seed", err)
		}
		params.Seed = openai.Int(int64(seed))
	}
	if raw, ok := settings["logprobs"]; ok {
		logprobs, err := cast.ToBool(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("logprobs", err)
		}
		params.Logprobs = openai.Bool(logprobs)
	}
	if raw, ok := settings["top_logprobs"]; ok {
		topLogprobs, err := cast.ToInt32(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("top_logprobs", err)
		}
		if err := adapter.ValidateInt32Min("top_logprobs", topLogprobs, 0); err != nil {
			return false, err
		}
		params.TopLogprobs = openai.Int(int64(topLogprobs))
	}
	if raw, ok := settings["reasoning_effort"]; ok {
		effort, err := cast.ToString(raw)
		if err != nil {
			return false, adapter.ProviderSettingError("reasoning_effort", err)
		}
		reasoningEffort, ok := parseReasoningEffort(effort)
		if !ok {
			return false, adapter.ProviderSettingError(
				"reasoning_effort",
				fmt.Errorf("unsupported value %q", strings.TrimSpace(effort)),
			)
		}
		params.ReasoningEffort = reasoningEffort
		return true, nil
	}
	return false, nil
}
