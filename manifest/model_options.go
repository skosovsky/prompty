package manifest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/skosovsky/prompty"
)

var allowedModelOptionKeys = map[string]struct{}{
	"model":             {},
	"temperature":       {},
	"max_tokens":        {},
	"top_p":             {},
	"stop":              {},
	"provider_settings": {},
}

// DecodeModelOptions converts a normalized model_config block into typed ModelOptions.
// Vendor-specific settings are accepted only from model_config.provider_settings.
func DecodeModelOptions(raw map[string]any) (*prompty.ModelOptions, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	for _, key := range sortedModelOptionKeys(raw) {
		if _, ok := allowedModelOptionKeys[key]; !ok {
			return nil, fmt.Errorf("invalid model_config key: %s; use provider_settings", key)
		}
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

func sortedModelOptionKeys(raw map[string]any) []string {
	keys := make([]string, 0, len(raw))
	for key := range raw {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
