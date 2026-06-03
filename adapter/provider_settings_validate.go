package adapter

import (
	"errors"
	"fmt"
	"slices"
)

// RejectUnknownProviderSettingKeys returns ErrInvalidProviderSettings when settings
// contains keys outside allowed (fail-closed).
func RejectUnknownProviderSettingKeys(settings map[string]any, allowed []string) error {
	if len(settings) == 0 {
		return nil
	}
	for key := range settings {
		if !slices.Contains(allowed, key) {
			return ProviderSettingError(key, errors.New("unknown provider setting key"))
		}
	}
	return nil
}

// ValidateFloat32Range rejects values outside [min, max] inclusive.
func ValidateFloat32Range(key string, value float32, minVal, maxVal float32) error {
	if value < minVal || value > maxVal {
		return ProviderSettingError(
			key,
			fmt.Errorf("value %v out of range [%v, %v]", value, minVal, maxVal),
		)
	}
	return nil
}

// ValidateInt32Min rejects values below minVal.
func ValidateInt32Min(key string, value int32, minVal int32) error {
	if value < minVal {
		return ProviderSettingError(key, fmt.Errorf("value %d must be >= %d", value, minVal))
	}
	return nil
}
