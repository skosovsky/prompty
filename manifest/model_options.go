package manifest

import (
	"encoding/json"

	"github.com/skosovsky/prompty"
)

var knownModelOptionKeys = map[string]struct{}{
	"model":             {},
	"temperature":       {},
	"max_tokens":        {},
	"top_p":             {},
	"stop":              {},
	"provider_settings": {},
}

// DecodeModelOptions converts a normalized model_config block into typed ModelOptions.
// Unknown top-level keys are preserved in ProviderSettings, while explicit provider_settings wins on conflicts.
func DecodeModelOptions(raw map[string]any) (*prompty.ModelOptions, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	data, err := json.Marshal(raw)
	if err != nil {
		return nil, err
	}

	type alias prompty.ModelOptions
	var typed alias
	if err := json.Unmarshal(data, &typed); err != nil {
		return nil, err
	}

	var all map[string]any
	if err := json.Unmarshal(data, &all); err != nil {
		return nil, err
	}
	for key := range knownModelOptionKeys {
		delete(all, key)
	}

	providerSettings := typed.ProviderSettings
	if len(all) > 0 {
		if providerSettings == nil {
			providerSettings = all
		} else {
			for key, value := range all {
				if _, exists := providerSettings[key]; !exists {
					providerSettings[key] = value
				}
			}
		}
	}
	typed.ProviderSettings = providerSettings

	opts := prompty.ModelOptions(typed)
	if opts.Model == "" &&
		opts.Temperature == nil &&
		opts.MaxTokens == nil &&
		opts.TopP == nil &&
		len(opts.Stop) == 0 &&
		len(opts.ProviderSettings) == 0 {
		return nil, nil
	}
	return &opts, nil
}
