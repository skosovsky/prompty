package gen

import (
	"fmt"
	"slices"
	"strings"

	"github.com/dave/jennifer/jen"
)

// schemaField describes one property for code generation.
type schemaField struct {
	Name     string // JSON/template variable name
	GoName   string // PascalCase for Go field
	Type     jen.Code
	Optional bool
	Validate []string // validate tag parts
}

// schemaMapper maps JSON Schema to Go types using jennifer.
type schemaMapper struct {
	rootName string
	types    map[string]jen.Code
}

func newSchemaMapper(rootName string) *schemaMapper {
	return &schemaMapper{rootName: rootName, types: make(map[string]jen.Code)}
}

// pascal converts snake_case and kebab-case to PascalCase.
// Keys like user-query or user_query both become UserQuery; - is normalized to _ before splitting.
// Sanitizes identifiers (invalid chars -> _, digit prefix -> X).
func pascal(s string) string {
	s = sanitizeIdent(s)
	parts := strings.Split(s, "_")
	for i := range parts {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
}

// sanitizeIdent replaces [^a-zA-Z0-9_] with _, adds prefix when starting with digit.
func sanitizeIdent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" {
		return "_"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "X" + out
	}
	return out
}

// typeName builds RootName + Path (e.g. SupportAgentPatientAddress).
func (m *schemaMapper) typeName(path ...string) string {
	return m.rootName + strings.Join(path, "")
}

// validateObjectSchemaForInput returns an error if the root schema is not a valid object.
// Allows input_schema: { type: object } without properties (empty Input struct).
func validateObjectSchemaForInput(schema map[string]any) error {
	if schema == nil {
		return fmt.Errorf("input_schema root must be type: object")
	}
	typ, _ := schema["type"].(string)
	if typ != "object" {
		return fmt.Errorf("input_schema root must be type: object, got %q", typ)
	}
	props, _ := schema["properties"].(map[string]any)
	if props != nil && len(props) > 0 {
		return validatePropNames(props)
	}
	return nil
}

// validatePropNames checks property keys for invalid Go identifiers and collisions.
// Fail-fast: keys like "1st_query" or "user-query"+"user_query" (same UserQuery) cause errors.
func validatePropNames(props map[string]any) error {
	seen := make(map[string]string) // goName -> first propName that produced it
	for _, propName := range sortedKeys(props) {
		goName := pascal(propName)
		if len(goName) == 0 {
			return fmt.Errorf("property %q produces empty Go name", propName)
		}
		if goName[0] >= '0' && goName[0] <= '9' {
			return fmt.Errorf("property %q produces Go identifier %q starting with digit", propName, goName)
		}
		if first, ok := seen[goName]; ok && first != propName {
			return fmt.Errorf("property keys %q and %q both map to Go field %q (collision)", first, propName, goName)
		}
		seen[goName] = propName
	}
	return nil
}

// toFloat extracts a numeric value from YAML/JSON (may be float64, int, int64, etc).
func toFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case uint:
		return float64(x), true
	case uint64:
		return float64(x), true
	default:
		return 0, false
	}
}

// defaultToJenLit converts propSchema["default"] to jen literal for code generation.
// Returns (nil, false) if no default or unsupported type. Supported: string, integer, number, boolean.
func defaultToJenLit(propSchema map[string]any) (jen.Code, bool) {
	def, ok := propSchema["default"]
	if !ok || def == nil {
		return nil, false
	}
	typ, _ := propSchema["type"].(string)
	switch typ {
	case "string":
		s, ok := def.(string)
		if !ok {
			s = fmt.Sprintf("%v", def)
		}
		return jen.Lit(s), true
	case "integer":
		if v, ok := toFloat(def); ok {
			return jen.Lit(int64(v)), true
		}
		return nil, false
	case "number":
		if v, ok := toFloat(def); ok {
			return jen.Lit(v), true
		}
		return nil, false
	case "boolean":
		if b, ok := def.(bool); ok {
			if b {
				return jen.True(), true
			}
			return jen.False(), true
		}
		return nil, false
	default:
		return nil, false
	}
}

// getRequired returns property names from schema["required"].
func getRequired(schema map[string]any) map[string]bool {
	req := make(map[string]bool)
	if r, ok := schema["required"]; ok {
		switch v := r.(type) {
		case []string:
			for _, s := range v {
				req[s] = true
			}
		case []any:
			for _, e := range v {
				if s, ok := e.(string); ok {
					req[s] = true
				}
			}
		}
	}
	return req
}

// mapSchemaToGo generates Go type code from JSON Schema.
func (m *schemaMapper) mapSchemaToGo(schema map[string]any, path ...string) (jen.Code, error) {
	if schema == nil {
		return jen.Id("any"), nil
	}
	typ, _ := schema["type"].(string)
	switch typ {
	case "string":
		return jen.String(), nil
	case "integer":
		return jen.Int64(), nil
	case "number":
		return jen.Float64(), nil
	case "boolean":
		return jen.Bool(), nil
	case "array":
		items, _ := schema["items"].(map[string]any)
		elem, err := m.mapSchemaToGo(items, append(path, "Item")...)
		if err != nil {
			return nil, err
		}
		return jen.Index().Add(elem), nil
	case "object":
		props, _ := schema["properties"].(map[string]any)
		if props == nil || len(props) == 0 {
			// additionalProperties: { type: string } -> map[string]string; true/absent -> map[string]any
			if addl, ok := schema["additionalProperties"]; ok && addl != nil {
				if addlSchema, ok := addl.(map[string]any); ok {
					addlTyp, _ := addlSchema["type"].(string)
					// Limitation: additionalProperties with type "object" (nested schema) falls back to map[string]any
					// to avoid recursive struct generation; only primitive types (string, integer, number, boolean) produce typed maps.
					if addlTyp != "" && addlTyp != "object" {
						elem, err := m.mapSchemaToGo(addlSchema, append(path, "Val")...)
						if err == nil {
							return jen.Map(jen.String()).Add(elem), nil
						}
					}
				}
			}
			return jen.Map(jen.String()).Add(jen.Id("any")), nil
		}
		name := m.typeName(path...)
		_, exists := m.types[name]
		if exists {
			return jen.Qual("", name), nil
		}
		m.types[name] = nil
		return jen.Qual("", name), nil
	default:
		return jen.Id("any"), nil
	}
}

// buildValidateTags returns validate tag from schema constraints.
func buildValidateTags(propSchema map[string]any, required bool) []string {
	var tags []string
	if required {
		tags = append(tags, "required")
	}
	if propSchema == nil {
		return tags
	}
	if min, ok := toFloat(propSchema["minLength"]); ok && min > 0 {
		tags = append(tags, fmt.Sprintf("min=%d", int(min)))
	}
	if max, ok := toFloat(propSchema["maxLength"]); ok {
		tags = append(tags, fmt.Sprintf("max=%d", int(max)))
	}
	if min, ok := toFloat(propSchema["minimum"]); ok {
		tags = append(tags, fmt.Sprintf("gte=%d", int(min)))
	}
	if max, ok := toFloat(propSchema["maximum"]); ok {
		tags = append(tags, fmt.Sprintf("lte=%d", int(max)))
	}
	if enum, ok := propSchema["enum"]; ok {
		if arr, ok := enum.([]any); ok && len(arr) > 0 {
			var vals []string
			skipOneof := false
			for _, e := range arr {
				s := fmt.Sprintf("%v", e)
				// validator oneof uses space as delimiter; comma also breaks tag parsing.
				if strings.Contains(s, " ") || strings.Contains(s, ",") {
					skipOneof = true
					break
				}
				vals = append(vals, s)
			}
			if !skipOneof && len(vals) > 0 {
				tags = append(tags, fmt.Sprintf("oneof=%s", strings.Join(vals, " ")))
			}
		}
	}
	// array: add minItems/maxItems (array-level), then dive and item-level constraints
	if typ, _ := propSchema["type"].(string); typ == "array" {
		if min, ok := toFloat(propSchema["minItems"]); ok && min > 0 {
			tags = append(tags, fmt.Sprintf("min=%d", int(min)))
		}
		if max, ok := toFloat(propSchema["maxItems"]); ok {
			tags = append(tags, fmt.Sprintf("max=%d", int(max)))
		}
		items, _ := propSchema["items"].(map[string]any)
		if items != nil {
			itemType, _ := items["type"].(string)
			if itemType == "object" {
				tags = append(tags, "dive")
			} else if itemType == "string" || itemType == "integer" || itemType == "number" {
				// item-level constraints (minLength, maxLength, minimum, maximum) apply after dive
				tags = append(tags, "dive")
				if min, ok := toFloat(items["minLength"]); ok && min > 0 {
					tags = append(tags, fmt.Sprintf("min=%d", int(min)))
				}
				if max, ok := toFloat(items["maxLength"]); ok {
					tags = append(tags, fmt.Sprintf("max=%d", int(max)))
				}
				if min, ok := toFloat(items["minimum"]); ok {
					tags = append(tags, fmt.Sprintf("gte=%d", int(min)))
				}
				if max, ok := toFloat(items["maximum"]); ok {
					tags = append(tags, fmt.Sprintf("lte=%d", int(max)))
				}
			}
		}
	}
	// optional fields with constraints: omitempty skips validation when nil (validator would fail on nil pointer)
	if !required && len(tags) > 0 {
		tags = append([]string{"omitempty"}, tags...)
	}
	return tags
}

// typeSpec holds type name and its schema for emission.
type typeSpec struct {
	Name     string
	Schema   map[string]any
	Required map[string]bool
	Props    map[string]any
}

// GenerateTypes produces all struct definitions from the schema (nested types first).
func (m *schemaMapper) GenerateTypes(schema map[string]any, rootTypeName string) ([]jen.Code, error) {
	if schema == nil {
		return nil, nil
	}
	// Build type refs in mapSchemaToGo
	_, _ = m.mapSchemaToGo(schema, rootTypeName)

	var specs []typeSpec
	seen := make(map[string]string) // typeName -> path (for collision guard)
	if err := m.collectTypeSpecs(schema, rootTypeName, &specs, seen); err != nil {
		return nil, err
	}

	var stmts []jen.Code
	for _, ts := range specs {
		stmts = append(stmts, m.emitStruct(ts))
	}
	return stmts, nil
}

func (m *schemaMapper) collectTypeSpecs(schema map[string]any, path string, specs *[]typeSpec, seen map[string]string) error {
	if schema == nil {
		return nil
	}
	typ, _ := schema["type"].(string)
	// array: recurse into items (handles array-of-array, array-of-object)
	if typ == "array" {
		items, _ := schema["items"].(map[string]any)
		if items != nil {
			return m.collectTypeSpecs(items, path+"Item", specs, seen)
		}
		return nil
	}
	if typ != "object" {
		return nil
	}
	props, _ := schema["properties"].(map[string]any)
	if props == nil {
		props = make(map[string]any) // allow empty Input struct
	}
	// Nested types first (depth-first); iterate in sorted order for deterministic output
	for _, propName := range sortedKeys(props) {
		propVal := props[propName]
		if ps, ok := propVal.(map[string]any); ok {
			ptype, _ := ps["type"].(string)
			goPart := pascal(propName)
			if ptype == "object" {
				if err := m.collectTypeSpecs(ps, path+goPart, specs, seen); err != nil {
					return err
				}
			}
			if ptype == "array" {
				items, _ := ps["items"].(map[string]any)
				if items != nil {
					if err := m.collectTypeSpecs(items, path+goPart+"Item", specs, seen); err != nil {
						return err
					}
				}
			}
		}
	}
	typeName := m.typeName(path)
	if prev, ok := seen[typeName]; ok && prev != path {
		return fmt.Errorf("type name %q collision: generated from path %q and %q", typeName, prev, path)
	}
	seen[typeName] = path
	*specs = append(*specs, typeSpec{
		Name:     typeName,
		Schema:   schema,
		Required: getRequired(schema),
		Props:    props,
	})
	return nil
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

func (m *schemaMapper) emitStruct(ts typeSpec) jen.Code {
	var fields []jen.Code
	for _, propName := range sortedKeys(ts.Props) {
		propVal := ts.Props[propName]
		propSchema, _ := propVal.(map[string]any)
		goType, _ := m.mapSchemaToGo(propSchema, pascal(propName))
		optional := !ts.Required[propName]
		typ, _ := propSchema["type"].(string)
		if optional {
			// array: never * (slice is ref type). object with properties: use * for optional struct.
			// object without properties (map-like): no *.
			// any (typ=="" or unknown): no * — interface zero value is nil, *any is redundant.
			if typ == "array" {
				// no *
			} else if typ == "object" {
				props, _ := propSchema["properties"].(map[string]any)
				if props != nil {
					goType = jen.Op("*").Add(goType)
				}
			} else if typ == "string" || typ == "integer" || typ == "number" || typ == "boolean" {
				goType = jen.Op("*").Add(goType)
			}
			// else typ=="" or unknown -> any, no *
		} else if typ == "boolean" {
			// required bool: use *bool + validate:"required" for presence semantics (nil=invalid, false=valid)
			goType = jen.Op("*").Add(goType)
		}
		validate := buildValidateTags(propSchema, ts.Required[propName])
		jsonTag := propName
		if optional {
			jsonTag = propName + ",omitempty"
		}
		if jsonTag == "-" {
			jsonTag = "-,"
		}
		tags := map[string]string{"json": jsonTag, "prompt": propName}
		if len(validate) > 0 {
			tags["validate"] = strings.Join(validate, ",")
		}
		fields = append(fields, jen.Id(pascal(propName)).Add(goType).Tag(tags))
	}
	return jen.Type().Id(ts.Name).Struct(fields...)
}
