package manifest

import (
	"fmt"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/cast"
)

// BuildDescriptorFromRaw builds metadata-only descriptor without compiling templates.
func BuildDescriptorFromRaw(raw *RawManifest) (prompty.TemplateDescriptor, error) { //nolint:gocognit
	if raw.LegacyModelConfig != nil || raw.LegacyInputSchema != nil {
		return prompty.TemplateDescriptor{}, fmt.Errorf(
			"%w: use model_options/inputs instead of model_config/input_schema",
			prompty.ErrLegacyManifestVersion,
		)
	}
	if raw.ID == "" {
		return prompty.TemplateDescriptor{}, fmt.Errorf("%w: missing id", prompty.ErrInvalidManifest)
	}
	for i := range raw.Messages {
		if raw.Messages[i].LegacySourceID != "" {
			return prompty.TemplateDescriptor{}, fmt.Errorf(
				"%w: use layer_id instead of source_id",
				prompty.ErrLegacyManifestVersion,
			)
		}
	}
	meta := metadataToPromptMetadata(raw)
	desc := prompty.TemplateDescriptor{
		Metadata:          meta,
		ModelOptions:      raw.ModelOptions,
		Tools:             raw.Tools,
		RequiredTools:     normalizeRequiredTools(raw.RequiredTools),
		RequiredInputVars: nil,
		InputSchema:       raw.InputSchema,
		ResponseFormat:    raw.ResponseFormat,
		LayerIDs:          nil,
		Tags:              append([]string(nil), meta.Tags...),
		Capabilities:      append([]string(nil), meta.Capabilities...),
	}
	//nolint:nestif // required-input extraction is intentionally sequential.
	if raw.InputSchema != nil && len(raw.InputSchema.Schema) > 0 {
		schemaMap, err := prompty.JSONDocumentAsMap(raw.InputSchema.Schema)
		if err == nil && schemaMap != nil {
			if req, ok := schemaMap["required"]; ok {
				if ss, err := cast.ToStringSlice(req); err == nil {
					desc.RequiredInputVars = ss
				}
			}
		}
	}
	seen := make(map[string]bool)
	for _, rm := range raw.Messages {
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
func ParseDescriptor(data []byte, u Unmarshaler) (prompty.TemplateDescriptor, error) {
	if u == nil {
		return prompty.TemplateDescriptor{}, prompty.ErrNoParser
	}
	var raw RawManifest
	if err := u.Unmarshal(data, &raw); err != nil {
		return prompty.TemplateDescriptor{}, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
	}
	return BuildDescriptorFromRaw(&raw)
}
