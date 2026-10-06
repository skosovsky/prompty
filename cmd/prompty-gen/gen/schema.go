package gen

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/dave/jennifer/jen"

	"github.com/skosovsky/prompty/internal/jsonfield"
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
	dialect  string
	rootPath string // "Input" or "Output" — empty object allowed only at this path
	types    map[string]jen.Code
}

func newSchemaMapper(rootName string) *schemaMapper {
	return &schemaMapper{
		rootName: rootName,
		dialect:  "",
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
	typ, _, _ := schemaTypeInfo(schema)
	if typ != jsonSchemaTypeObject {
		return fmt.Errorf("input_schema root must be type: object, got %q", typ)
	}
	props, _ := schema["properties"].(map[string]any)
	if err := validatePropNames(props); err != nil {
		return err
	}
	return validateObjectProjection(schema, props, []string{"Input"})
}

// validatePropNames checks property keys for invalid Go identifiers and collisions.
// Fail-fast: keys like "1st_query" or "user-query"+"user_query" (same UserQuery) cause errors.
func validatePropNames(props map[string]any) error {
	seen := make(map[string]string) // goName -> first propName that produced it
	for _, propName := range sortedKeys(props) {
		if !jsonfield.ValidTagName(propName) {
			return fmt.Errorf("property %q cannot be represented by an encoding/json field tag", propName)
		}
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
//

func (m *schemaMapper) mapSchemaToGo(schema map[string]any, path ...string) (jen.Code, error) {
	code, err := m.mapNonNullableSchemaToGo(schema, path...)
	if err != nil {
		return nil, err
	}
	_, nullable, _ := schemaTypeInfo(schema)
	if nullable && !schemaTypeNilable(schema) {
		code = jen.Op("*").Add(code)
	}
	return code, nil
}

func (m *schemaMapper) mapNonNullableSchemaToGo(schema map[string]any, path ...string) (jen.Code, error) {
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
	case jsonSchemaTypeInteger, jsonSchemaTypeNumber:
		return jen.Qual("encoding/json", "Number"), nil
	case jsonSchemaTypeBoolean:
		return jen.Bool(), nil
	case jsonSchemaTypeArray:
		if format, _ := schema["format"].(string); format == "messages" {
			return jen.Index().Qual("github.com/skosovsky/prompty", "ChatMessage"), nil
		}
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
			addlTyp, _, _ := schemaTypeInfo(addlSchema)
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
	_, nullable, _ := schemaTypeInfo(schema)
	if nullable {
		return nil, errStrictSchema([]string{rootTypeName}, "nullable root object requires caller-owned pointer type")
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
	typ, _, _ := schemaTypeInfo(schema)
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
			ptype, _, _ := schemaTypeInfo(ps)
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
	if err := validatePropNames(props); err != nil {
		return err
	}
	if err := validateObjectProjection(schema, props, []string{path}); err != nil {
		return err
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
		typ, nullable, _ := schemaTypeInfo(propSchema)
		if optional {
			goType = jen.Op("*").Add(goType)
		} else if typ == jsonSchemaTypeBoolean && !nullable {
			goType = jen.Op("*").Add(goType)
		}
		fieldName := pascal(propName)
		if fieldName == "UnmarshalJSON" ||
			(ts.Name == m.typeName(m.rootPath) && (fieldName == "JSONSchema" || fieldName == "ValidateSchema")) {
			return nil, errStrictSchema([]string{ts.Name, propName}, "field name conflicts with generated method")
		}

		jsonTag := propName
		if optional {
			jsonTag = propName + ",omitempty"
		}
		if jsonTag == "-" {
			jsonTag = "-,"
		}
		promptTag := propName
		if optional {
			promptTag += ",omitempty"
		}
		tags := map[string]string{"json": jsonTag, "prompt": promptTag}

		fields = append(fields, jen.Id(pascal(propName)).Add(goType).Tag(tags))
	}
	declaration := jen.Type().Id(ts.Name).Struct(fields...)
	if ts.Name != m.typeName(m.rootPath) {
		decoder, err := m.emitJSONDecoder(ts, nil)
		if err != nil {
			return nil, err
		}
		return jen.Add(declaration).Line().Line().Add(decoder), nil
	}
	return m.emitSchemaMethods(ts, declaration)
}

func (m *schemaMapper) emitSchemaMethods(ts typeSpec, declaration *jen.Statement) (jen.Code, error) {
	document, err := json.Marshal(ts.Schema)
	if err != nil {
		return nil, err
	}
	definition := func() *jen.Statement {
		return jen.Op("&").Qual("github.com/skosovsky/prompty", "SchemaDefinition").Values(
			jen.Id("Schema").
				Op(":").
				Qual("github.com/skosovsky/prompty", "JSONDocument").
				Call(jen.Lit(string(document))),
			jen.Id("Dialect").Op(":").Lit(m.dialect),
		)
	}
	output := jen.Add(declaration).Line().Line()
	output.Add(jen.Func().Params(jen.Id("value").Id(ts.Name)).Id("JSONSchema").Params().Qual("github.com/skosovsky/prompty", "JSONDocument").Block(
		jen.Return(jen.Qual("github.com/skosovsky/prompty", "JSONDocument").Call(jen.Lit(string(document)))),
	)).
		Line().
		Line()
	output.Add(jen.Func().Params(jen.Id("value").Id(ts.Name)).Id("ValidateSchema").Params().Error().Block(
		jen.Return(jen.Qual("github.com/skosovsky/prompty", "ValidateSchemaValue").Call(definition(), jen.Id("value"))),
	)).Line().Line()

	decoder, err := m.emitJSONDecoder(ts, definition())
	if err != nil {
		return nil, err
	}
	output.Add(decoder)
	return output, nil
}

func schemaTypeNilable(schema map[string]any) bool {
	kind, _, _ := schemaTypeInfo(schema)
	if kind == jsonSchemaTypeArray {
		return true
	}
	properties, _ := schema["properties"].(map[string]any)
	return kind == jsonSchemaTypeObject && len(properties) == 0
}

func (m *schemaMapper) emitJSONDecoder(ts typeSpec, definition jen.Code) (jen.Code, error) {
	var body []jen.Code
	if definition != nil {
		body = append(
			body,
			jen.If(jen.Id("err").Op(":=").Qual("github.com/skosovsky/prompty", "ValidateSchemaJSON").Call(definition, jen.Id("document")), jen.Id("err").Op("!=").Nil()).
				Block(jen.Return(jen.Id("err"))),
		)
	}
	body = append(
		body,
		jen.Type().Id("decodedValue").Id(ts.Name),
		jen.Var().Id("decoded").Id("decodedValue"),
		jen.Id("decoder").
			Op(":=").
			Qual("encoding/json", "NewDecoder").
			Call(jen.Qual("bytes", "NewReader").Call(jen.Id("document"))),
		jen.Id("decoder").Dot("UseNumber").Call(),
		jen.If(jen.Id("err").Op(":=").Id("decoder").Dot("Decode").Call(jen.Op("&").Id("decoded")), jen.Id("err").Op("!=").Nil()).
			Block(jen.Return(jen.Id("err"))),
	)
	initializers, err := m.nullablePresenceInitializers(ts)
	if err != nil {
		return nil, err
	}
	if len(initializers) > 0 {
		body = append(
			body,
			jen.Var().Id("rawFields").Map(jen.String()).Qual("encoding/json", "RawMessage"),
			jen.If(jen.Id("err").Op(":=").Qual("encoding/json", "Unmarshal").Call(jen.Id("document"), jen.Op("&").Id("rawFields")), jen.Id("err").Op("!=").Nil()).
				Block(jen.Return(jen.Id("err"))),
		)
		body = append(body, initializers...)
	}
	body = append(body, jen.Op("*").Id("value").Op("=").Id(ts.Name).Call(jen.Id("decoded")), jen.Return(jen.Nil()))
	return jen.Func().
			Params(jen.Id("value").Op("*").Id(ts.Name)).
			Id("UnmarshalJSON").
			Params(jen.Id("document").Index().Byte()).
			Error().
			Block(body...),
		nil
}

func (m *schemaMapper) nullablePresenceInitializers(ts typeSpec) ([]jen.Code, error) {
	var initializers []jen.Code
	basePath := strings.TrimPrefix(ts.Name, m.rootName)
	for _, name := range sortedKeys(ts.Props) {
		property, _ := ts.Props[name].(map[string]any)
		_, nullable, _ := schemaTypeInfo(property)
		if !nullable || ts.Required[name] {
			continue
		}
		goType, err := m.mapSchemaToGo(property, basePath+pascal(name))
		if err != nil {
			return nil, err
		}
		localName := "nullable" + pascal(name)
		initializers = append(
			initializers,
			jen.If(jen.String().Call(jen.Qual("bytes", "TrimSpace").Call(jen.Id("rawFields").Index(jen.Lit(name)))).Op("==").Lit("null")).
				Block(
					jen.Var().
						Id(localName).
						Add(goType),
					jen.Id("decoded").Dot(pascal(name)).Op("=").Op("&").Id(localName)),
		)
	}
	return initializers, nil
}

// A declared-field struct cannot preserve unknown fields used by root constraints.
// Reject such schemas rather than silently changing their accepted value set.
func validateObjectProjection(schema, props map[string]any, path []string) error {
	// In draft-07 a $ref ignores siblings; additionalProperties:false does not
	// prove that a referenced object is closed. Keep reference projection explicit.
	for _, keyword := range []string{"$ref", "$dynamicRef", "$recursiveRef"} {
		if _, exists := schema[keyword]; exists {
			return errStrictSchema(path, "referenced object requires caller-owned type")
		}
	}

	for name := range getRequired(schema) {
		if _, ok := props[name]; !ok {
			return errStrictSchema(path, "required property "+name+" is not declared in properties")
		}
	}
	if explicit, ok := schema["additionalProperties"].(bool); ok && !explicit && schema["patternProperties"] == nil {
		return nil
	}
	for _, keyword := range []string{"minProperties", "const", "enum", "allOf", "anyOf", "oneOf", "not", "if", "then", "else", "dependencies", "dependentRequired", "dependentSchemas", "$ref", "$dynamicRef", "$recursiveRef", "unevaluatedProperties", "patternProperties", "propertyNames"} {
		if _, exists := schema[keyword]; exists {
			return errStrictSchema(
				path,
				"open object keyword "+keyword+" requires lossless unknown-field representation; use caller-owned types or closed object schema",
			)
		}
	}
	return nil
}
