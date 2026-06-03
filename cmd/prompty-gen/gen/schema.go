package gen

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/dave/jennifer/jen"
)

// JSON Schema primitive and structural type keywords (draft subset used by prompty-gen).
const (
	jsonSchemaTypeObject  = "object"
	jsonSchemaTypeString  = "string"
	jsonSchemaTypeInteger = "integer"
	jsonSchemaTypeNumber  = "number"
	jsonSchemaTypeBoolean = "boolean"
	jsonSchemaTypeArray   = "array"
)

// schemaMapper maps JSON Schema to Go types using jennifer.
type schemaMapper struct {
	rootName string
	rootPath string // "Input" or "Output" — empty object allowed only at this path
	types    map[string]jen.Code
}

func newSchemaMapper(rootName string) *schemaMapper {
	return &schemaMapper{
		rootName: rootName,
		rootPath: "",
		types:    make(map[string]jen.Code),
	}
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
		return errors.New("input_schema root must be type: object")
	}
	typ, _ := schema["type"].(string)
	if typ != jsonSchemaTypeObject {
		return fmt.Errorf("input_schema root must be type: object, got %q", typ)
	}
	props, _ := schema["properties"].(map[string]any)
	if len(props) > 0 {
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

// schemaTypeInfo resolves JSON Schema type keyword including nullable unions.
func schemaTypeInfo(schema map[string]any) (string, bool, bool) {
	if schema == nil {
		return "", false, false
	}
	if primary, nullable, ok := resolveOneOfNullable(schema); ok {
		return primary, nullable, false
	}
	if _, ok := schema["oneOf"]; ok {
		return "", false, true
	}
	if _, ok := schema["anyOf"]; ok {
		return "", false, true
	}
	if typ, ok := schema["type"].(string); ok {
		return typ, false, false
	}
	types, ok := schema["type"].([]any)
	if !ok {
		return "", false, false
	}
	var prim string
	hasNull := false
	for _, item := range types {
		s, ok := item.(string)
		if !ok {
			continue
		}
		if s == "null" {
			hasNull = true
			continue
		}
		if prim == "" {
			prim = s
			continue
		}
		if prim != s {
			return "", false, true
		}
	}
	return prim, hasNull, false
}

func resolveOneOfNullable(schema map[string]any) (string, bool, bool) {
	oneOf, exists := schema["oneOf"].([]any)
	if !exists || len(oneOf) != 2 {
		return "", false, false
	}
	var prim string
	hasNull := false
	for _, branch := range oneOf {
		b, ok := branch.(map[string]any)
		if !ok {
			return "", false, false
		}
		t, n, u := schemaTypeInfo(b)
		if u {
			return "", false, false
		}
		if t == "" && n {
			hasNull = true
			continue
		}
		if t == "" {
			return "", false, false
		}
		if prim != "" && prim != t {
			return "", false, false
		}
		prim = t
		if n {
			hasNull = true
		}
	}
	if prim == "" {
		return "", false, false
	}
	return prim, hasNull, true
}

func schemaPathLabel(path []string) string {
	if len(path) == 0 {
		return "<root>"
	}
	return strings.Join(path, ".")
}

func errStrictSchema(path []string, reason string) error {
	return fmt.Errorf("prompty-gen: schema at %s: %s", schemaPathLabel(path), reason)
}

// mapSchemaToGo generates Go type code from JSON Schema.
func (m *schemaMapper) mapSchemaToGo(schema map[string]any, path ...string) (jen.Code, error) {
	if schema == nil {
		return nil, errStrictSchema(path, "schema is nil")
	}
	typ, _, union := schemaTypeInfo(schema)
	if union {
		return nil, errStrictSchema(path, "oneOf/anyOf union requires explicit typed branches")
	}
	switch typ {
	case jsonSchemaTypeString:
		return jen.String(), nil
	case jsonSchemaTypeInteger:
		return jen.Int64(), nil
	case jsonSchemaTypeNumber:
		return jen.Float64(), nil
	case jsonSchemaTypeBoolean:
		return jen.Bool(), nil
	case jsonSchemaTypeArray:
		items, _ := schema["items"].(map[string]any)
		elem, err := m.mapSchemaToGo(items, append(path, "Item")...)
		if err != nil {
			return nil, err
		}
		return jen.Index().Add(elem), nil
	case jsonSchemaTypeObject:
		props, _ := schema["properties"].(map[string]any)
		if len(props) == 0 {
			if len(path) == 1 && path[0] == m.rootPath {
				name := m.typeName(path...)
				if _, exists := m.types[name]; !exists {
					m.types[name] = nil
				}
				return jen.Qual("", name), nil
			}
			addl, ok := schema["additionalProperties"]
			if !ok || addl == nil {
				return nil, errStrictSchema(path, "object without properties requires typed additionalProperties")
			}
			addlSchema, ok := addl.(map[string]any)
			if !ok {
				return nil, errStrictSchema(path, "additionalProperties must be a schema object with type")
			}
			addlTyp, _ := addlSchema["type"].(string)
			if addlTyp == "" || addlTyp == jsonSchemaTypeObject {
				return nil, errStrictSchema(path, "additionalProperties must declare a primitive or array item type")
			}
			elem, err := m.mapSchemaToGo(addlSchema, append(path, "Val")...)
			if err != nil {
				return nil, err
			}
			return jen.Map(jen.String()).Add(elem), nil
		}
		name := m.typeName(path...)
		_, exists := m.types[name]
		if exists {
			return jen.Qual("", name), nil
		}
		m.types[name] = nil
		return jen.Qual("", name), nil
	default:
		return nil, errStrictSchema(path, "missing or unsupported type keyword")
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
	if minLen, ok := toFloat(propSchema["minLength"]); ok && minLen > 0 {
		tags = append(tags, fmt.Sprintf("min=%d", int(minLen)))
	}
	if maxLen, ok := toFloat(propSchema["maxLength"]); ok {
		tags = append(tags, fmt.Sprintf("max=%d", int(maxLen)))
	}
	if minVal, ok := toFloat(propSchema["minimum"]); ok {
		tags = append(tags, fmt.Sprintf("gte=%d", int(minVal)))
	}
	if maxVal, ok := toFloat(propSchema["maximum"]); ok {
		tags = append(tags, fmt.Sprintf("lte=%d", int(maxVal)))
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
	if typ, _ := propSchema["type"].(string); typ == jsonSchemaTypeArray {
		if minItems, ok := toFloat(propSchema["minItems"]); ok && minItems > 0 {
			tags = append(tags, fmt.Sprintf("min=%d", int(minItems)))
		}
		if maxItems, ok := toFloat(propSchema["maxItems"]); ok {
			tags = append(tags, fmt.Sprintf("max=%d", int(maxItems)))
		}
		items, _ := propSchema["items"].(map[string]any)
		if items != nil {
			itemType, _ := items["type"].(string)
			switch itemType {
			case jsonSchemaTypeObject:
				tags = append(tags, "dive")
			case jsonSchemaTypeString, jsonSchemaTypeInteger, jsonSchemaTypeNumber:
				// item-level constraints (minLength, maxLength, minimum, maximum) apply after dive
				tags = append(tags, "dive")
				if itemMinLen, ok := toFloat(items["minLength"]); ok && itemMinLen > 0 {
					tags = append(tags, fmt.Sprintf("min=%d", int(itemMinLen)))
				}
				if itemMaxLen, ok := toFloat(items["maxLength"]); ok {
					tags = append(tags, fmt.Sprintf("max=%d", int(itemMaxLen)))
				}
				if itemMinVal, ok := toFloat(items["minimum"]); ok {
					tags = append(tags, fmt.Sprintf("gte=%d", int(itemMinVal)))
				}
				if itemMaxVal, ok := toFloat(items["maximum"]); ok {
					tags = append(tags, fmt.Sprintf("lte=%d", int(itemMaxVal)))
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
	m.rootPath = rootTypeName
	if _, err := m.mapSchemaToGo(schema, rootTypeName); err != nil {
		return nil, err
	}

	var specs []typeSpec
	seen := make(map[string]string) // typeName -> path (for collision guard)
	if err := m.collectTypeSpecs(schema, rootTypeName, &specs, seen); err != nil {
		return nil, err
	}

	var stmts []jen.Code
	for _, ts := range specs {
		stmt, err := m.emitStruct(ts)
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
	}
	return stmts, nil
}

func (m *schemaMapper) collectTypeSpecs(
	schema map[string]any,
	path string,
	specs *[]typeSpec,
	seen map[string]string,
) error {
	if schema == nil {
		return nil
	}
	typ, _ := schema["type"].(string)
	// array: recurse into items (handles array-of-array, array-of-object)
	if typ == jsonSchemaTypeArray {
		items, _ := schema["items"].(map[string]any)
		if items != nil {
			return m.collectTypeSpecs(items, path+"Item", specs, seen)
		}
		return nil
	}
	if typ != jsonSchemaTypeObject {
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
			if ptype == jsonSchemaTypeObject {
				if err := m.collectTypeSpecs(ps, path+goPart, specs, seen); err != nil {
					return err
				}
			}
			if ptype == jsonSchemaTypeArray {
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

func (m *schemaMapper) emitStruct(ts typeSpec) (jen.Code, error) {
	var fields []jen.Code
	// Base path for children: ts.Name = rootName+path; children need path+propPart (matches collectTypeSpecs).
	basePath := strings.TrimPrefix(ts.Name, m.rootName)
	for _, propName := range sortedKeys(ts.Props) {
		propVal := ts.Props[propName]
		propSchema, _ := propVal.(map[string]any)
		// Child type name = ParentName + Pascal(propName); for array, mapSchemaToGo appends "Item".
		nestedPath := basePath + pascal(propName)
		goType, err := m.mapSchemaToGo(propSchema, nestedPath)
		if err != nil {
			return nil, err
		}
		optional := !ts.Required[propName]
		typ, nullable, union := schemaTypeInfo(propSchema)
		usePointer := optional || nullable
		if usePointer && !union {
			// array: never * (slice is ref type). object with properties: use * for optional struct.
			// object without properties (map-like): no *.
			// unknown/union: no * — use json.RawMessage without pointer.
			switch typ {
			case jsonSchemaTypeArray:
				// slice is already a reference type; no pointer
			case jsonSchemaTypeObject:
				props, _ := propSchema["properties"].(map[string]any)
				if props != nil {
					goType = jen.Op("*").Add(goType)
				}
			case jsonSchemaTypeString, jsonSchemaTypeInteger, jsonSchemaTypeNumber, jsonSchemaTypeBoolean:
				goType = jen.Op("*").Add(goType)
			}
		} else if typ == jsonSchemaTypeBoolean && ts.Required[propName] {
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
	return jen.Type().Id(ts.Name).Struct(fields...), nil
}
