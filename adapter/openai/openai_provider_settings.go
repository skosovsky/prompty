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
		wireReasoningEffort,
	}
}

func applyOpenAIProviderSettings(params *openai.ChatCompletionNewParams, settings map[string]any) (bool, error) {
	if len(settings) == 0 {
		return false, nil
	}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, openAIProviderSettingKeys()); err != nil {
		return false, err
	}
	if err := applyOpenAIPenalties(params, settings); err != nil {
		return false, err
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
	if raw, ok := settings[wireReasoningEffort]; ok {
		effort, err := cast.ToString(raw)
		if err != nil {
			return false, adapter.ProviderSettingError(wireReasoningEffort, err)
		}
		reasoningEffort, ok := parseReasoningEffort(effort)
		if !ok {
			return false, adapter.ProviderSettingError(
				wireReasoningEffort,
				fmt.Errorf("unsupported value %q", strings.TrimSpace(effort)),
			)
		}
		params.ReasoningEffort = reasoningEffort
		return true, nil
	}
	return false, nil
}

func applyOpenAIPenalties(params *openai.ChatCompletionNewParams, settings map[string]any) error {
	if raw, ok := settings["presence_penalty"]; ok {
		penalty, err := cast.ToFloat32(raw)
		if err != nil {
			return adapter.ProviderSettingError("presence_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("presence_penalty", penalty, -2, 2); err != nil {
			return err
		}
		params.PresencePenalty = openai.Float(float64(penalty))
	}
	if raw, ok := settings["frequency_penalty"]; ok {
		penalty, err := cast.ToFloat32(raw)
		if err != nil {
			return adapter.ProviderSettingError("frequency_penalty", err)
		}
		if err := adapter.ValidateFloat32Range("frequency_penalty", penalty, -2, 2); err != nil {
			return err
		}
		params.FrequencyPenalty = openai.Float(float64(penalty))
	}

	return nil
}
