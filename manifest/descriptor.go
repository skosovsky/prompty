package manifest

import (
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/cast"
)

// BuildDescriptorFromRaw builds metadata-only descriptor without compiling templates.
func BuildDescriptorFromRaw(raw *RawManifest, po *parseOpts) (prompty.TemplateDescriptor, error) { //nolint:gocognit
	if raw.LegacyModelConfig != nil || raw.LegacyInputSchema != nil {
		return prompty.TemplateDescriptor{}, fmt.Errorf(
			"%w: use model_options/inputs instead of model_config/input_schema",
			prompty.ErrLegacyManifestVersion,
		)
	}
	if raw.ID == "" {
		return prompty.TemplateDescriptor{}, fmt.Errorf("%w: missing id", prompty.ErrInvalidManifest)
	}
	working := raw
	var owned RawManifest
	//nolint:nestif // compose-aware descriptor mirrors BuildFromRaw expansion
	if len(raw.Layers) > 0 || len(raw.Imports) > 0 {
		composeCtx := ComposeContext{
			Ctx:                         nil,
			Values:                      prompty.ComposeValues{},
			Loader:                      nil,
			AllowMissingConditionValues: true,
		}
		if po != nil && po.compose != nil {
			composeCtx = *po.compose
			if !composeCtx.Values.IsSet() {
				composeCtx.AllowMissingConditionValues = true
			}
		}
		if composeCtx.Loader == nil {
			return prompty.TemplateDescriptor{}, fmt.Errorf(
				"%w: imports/layers require compose loader (use manifest.WithCompose)",
				prompty.ErrInvalidManifest,
			)
		}
		cloned, cloneErr := cloneRawManifest(raw)
		if cloneErr != nil {
			return prompty.TemplateDescriptor{}, cloneErr
		}
		if len(raw.Imports) > 0 && !composeCtx.Values.IsSet() {
			lctx := composeLoaderCtx(composeCtx)
			effective, schemaErr := ResolveEffectiveInputSchema(lctx, cloned, composeCtx.Loader)
			if schemaErr != nil {
				return prompty.TemplateDescriptor{}, schemaErr
			}
			cloned.InputSchema = effective
		}
		if expandErr := ExpandRawManifest(cloned, composeCtx); expandErr != nil {
			return prompty.TemplateDescriptor{}, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, expandErr)
		}
		owned = *cloned
		working = &owned
	}
	for i := range working.Messages {
		if working.Messages[i].LegacySourceID != "" {
			return prompty.TemplateDescriptor{}, fmt.Errorf(
				"%w: use layer_id instead of source_id",
				prompty.ErrLegacyManifestVersion,
			)
		}
	}
	meta := metadataToPromptMetadata(working)
	desc := prompty.TemplateDescriptor{
		Metadata:          meta,
		ModelOptions:      working.ModelOptions,
		Tools:             working.Tools,
		RequiredTools:     normalizeRequiredTools(working.RequiredTools),
		RequiredInputVars: nil,
		InputSchema:       working.InputSchema,
		ResponseFormat:    working.ResponseFormat,
		LayerIDs:          nil,
		Tags:              append([]string(nil), meta.Tags...),
		Capabilities:      append([]string(nil), meta.Capabilities...),
	}
	if working.InputSchema != nil && len(working.InputSchema.Schema) > 0 {
		schemaMap, err := prompty.JSONDocumentAsMap(working.InputSchema.Schema)
		if err != nil {
			return prompty.TemplateDescriptor{}, fmt.Errorf("descriptor schema: %w", err)
		}
		props, _ := schemaMap["properties"].(map[string]any)
		if req, ok := schemaMap["required"]; ok {
			ss, reqErr := cast.ToStringSlice(req)
			if reqErr != nil {
				return prompty.TemplateDescriptor{}, fmt.Errorf("descriptor schema required: %w", reqErr)
			}
			desc.RequiredInputVars = FilterEarlyRequired(ss, props)
		}
	}
	seen := make(map[string]bool)
	for _, rm := range working.Messages {
		if rm.LayerID == "" {
			continue
		}
		if !seen[rm.LayerID] {
			seen[rm.LayerID] = true
			desc.LayerIDs = append(desc.LayerIDs, rm.LayerID)
		}
	}
	return desc, nil
}

// ParseDescriptor unmarshals manifest bytes and returns descriptor without template AST compilation.
func ParseDescriptor(data []byte, u Unmarshaler, opts ...ParseOption) (prompty.TemplateDescriptor, error) {
	if u == nil {
		return prompty.TemplateDescriptor{}, prompty.ErrNoParser
	}
	var raw RawManifest
	if err := u.Unmarshal(data, &raw); err != nil {
		return prompty.TemplateDescriptor{}, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	var po parseOpts
	for _, opt := range opts {
		opt(&po)
	}
	return BuildDescriptorFromRaw(&raw, &po)
}
