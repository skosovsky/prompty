package openai

import (
	"errors"
	"slices"

	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func applyResponsesReasoning(
	params *responses.ResponseNewParams,
	options *prompty.ModelOptions,
) error {
	settings, err := prompty.ProviderSettingsMap(options)
	if err != nil {
		return err
	}
	keys := []string{"reasoning_effort", "reasoning_summary"}
	if err := adapter.RejectUnknownProviderSettingKeys(settings, keys); err != nil {
		return err
	}
	if raw, exists := settings["reasoning_effort"]; exists {
		effort, ok := raw.(string)
		if !ok ||
			!slices.Contains(
				[]string{"none", "minimal", "low", "medium", "high", "xhigh"},
				effort,
			) {
			return adapter.ProviderSettingError(
				"reasoning_effort",
				errors.New("unsupported Responses effort"),
			)
		}
		params.Reasoning.Effort = shared.ReasoningEffort(effort)
	}
	if raw, exists := settings["reasoning_summary"]; exists {
		summary, ok := raw.(string)
		if !ok || !slices.Contains([]string{"auto", "concise", "detailed"}, summary) {
			return adapter.ProviderSettingError(
				"reasoning_summary",
				errors.New("unsupported Responses summary"),
			)
		}
		params.Reasoning.Summary = shared.ReasoningSummary(summary)
	}
	return nil
}

// Strict tools use the original object schema without hidden closing/nullability
// changes. The subset is deliberately explicit; unknown keywords fail closed.
func validateStrictFunctionSchema(schema map[string]any) error {
	if schema["type"] != "object" {
		return schemaError("/type", "strict function requires object root")
	}
	if _, err := mapOutputSchema(schema, prompty.SchemaPolicy{}); err != nil {
		return err
	}
	return validateStrictKeywords(schema, "")
}

func validateStrictKeywords(node map[string]any, pointer string) error {
	if _, hasType := node["type"]; !hasType {
		_, reference := node["$ref"]
		_, union := node["anyOf"]
		if !reference && !union {
			return schemaError(pointer, "strict schema requires type, $ref or anyOf")
		}
	}
	allowed := []string{
		"$schema",
		"$id",
		"$ref",
		"$defs",
		"definitions",
		"type",
		"title",
		"description",
		"properties",
		"required",
		"additionalProperties",
		"items",
		"anyOf",
		"enum",
		"const",
		"format",
		"pattern",
		"minimum",
		"maximum",
		"exclusiveMinimum",
		"exclusiveMaximum",
		"multipleOf",
		"minItems",
		"maxItems",
	}
	for _, key := range sortedSchemaKeys(node) {
		if !slices.Contains(allowed, key) {
			return schemaError(schemaPointer(pointer, key), "unsupported native strict keyword")
		}
		path := schemaPointer(pointer, key)
		switch key {
		case "properties", "$defs", "definitions":
			children, ok := node[key].(map[string]any)
			if !ok {
				return schemaError(path, "strict schema children must be objects")
			}
			for _, name := range sortedSchemaKeys(children) {
				child, ok := children[name].(map[string]any)
				if !ok {
					return schemaError(
						schemaPointer(path, name),
						"strict schema child must be an object",
					)
				}
				if err := validateStrictKeywords(child, schemaPointer(path, name)); err != nil {
					return err
				}
			}
		case "items":
			child, ok := node[key].(map[string]any)
			if !ok {
				return schemaError(path, "strict items must be a schema object")
			}
			if err := validateStrictKeywords(child, path); err != nil {
				return err
			}
		case "anyOf":
			children, ok := node[key].([]any)
			if !ok {
				return schemaError(path, "strict anyOf must be an array")
			}
			for _, item := range children {
				child, ok := item.(map[string]any)
				if !ok {
					return schemaError(path, "strict anyOf child must be a schema object")
				}
				if err := validateStrictKeywords(child, path); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
