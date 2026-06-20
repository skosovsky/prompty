package gen

import (
	"context"
	"fmt"
	"slices"

	"github.com/skosovsky/prompty/manifest"
)

// ComposeConditionsFromRawManifest extracts direct and transitive condition.match keys
// for typed compose context generation.
func ComposeConditionsFromRawManifest(
	ctx context.Context,
	raw *manifest.RawManifest,
	loader manifest.ManifestLoader,
) ([]ComposeConditionSpec, error) {
	byKey := make(map[string]ComposeConditionSpec)
	seen := make(map[string]bool)
	if err := collectComposeConditions(ctx, raw, loader, byKey, seen); err != nil {
		return nil, err
	}
	return sortedComposeConditions(byKey), nil
}

// ComposeConditionsFromRawImports extracts condition.match keys for typed compose context generation.
func ComposeConditionsFromRawImports(imports []manifest.RawImport) ([]ComposeConditionSpec, error) {
	byKey := make(map[string]ComposeConditionSpec)
	if err := collectComposeConditionsFromImports(imports, byKey); err != nil {
		return nil, err
	}
	return sortedComposeConditions(byKey), nil
}

func collectComposeConditions(
	ctx context.Context,
	raw *manifest.RawManifest,
	loader manifest.ManifestLoader,
	byKey map[string]ComposeConditionSpec,
	seen map[string]bool,
) error {
	if raw == nil {
		return nil
	}
	if raw.ID != "" {
		if seen[raw.ID] {
			return nil
		}
		seen[raw.ID] = true
	}
	if err := collectComposeConditionsFromImports(raw.Imports, byKey); err != nil {
		return err
	}
	if loader == nil {
		return nil
	}
	for _, imp := range raw.Imports {
		if imp.ID == "" || seen[imp.ID] {
			continue
		}
		child, err := loader.LoadByID(ctx, imp.ID)
		if err != nil {
			return fmt.Errorf("compose condition: load import %q: %w", imp.ID, err)
		}
		if err := collectComposeConditions(ctx, child, loader, byKey, seen); err != nil {
			return err
		}
	}
	return nil
}

func collectComposeConditionsFromImports(
	imports []manifest.RawImport,
	byKey map[string]ComposeConditionSpec,
) error {
	for _, imp := range imports {
		if imp.Condition == nil {
			continue
		}
		for key, value := range imp.Condition.Match {
			if key == "" {
				continue
			}
			kind, err := composeConditionKind(value)
			if err != nil {
				return fmt.Errorf("compose condition %q: %w", key, err)
			}
			if prev, ok := byKey[key]; ok {
				if prev.Kind != kind {
					return fmt.Errorf(
						"compose condition %q: conflicting kinds %q and %q",
						key,
						prev.Kind,
						kind,
					)
				}
				continue
			}
			byKey[key] = ComposeConditionSpec{Key: key, FieldName: "", Kind: kind}
		}
	}
	return nil
}

func sortedComposeConditions(byKey map[string]ComposeConditionSpec) []ComposeConditionSpec {
	keys := make([]string, 0, len(byKey))
	for key := range byKey {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	out := make([]ComposeConditionSpec, 0, len(keys))
	for _, key := range keys {
		out = append(out, byKey[key])
	}
	return out
}

func composeConditionKind(value any) (string, error) {
	switch value.(type) {
	case bool:
		return composeConditionKindBool, nil
	case string:
		return composeConditionKindString, nil
	case int, int8, int16, int32, int64:
		return composeConditionKindInt, nil
	case uint, uint8, uint16, uint32:
		return composeConditionKindInt, nil
	case float32, float64:
		return composeConditionKindFloat, nil
	default:
		return "", fmt.Errorf("unsupported scalar kind %T", value)
	}
}
