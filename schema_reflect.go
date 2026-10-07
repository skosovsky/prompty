package prompty

import (
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/skosovsky/prompty/internal/jsonfield"
)

const maxSchemaDepth = 32

const schemaTypeArray = "array"

const schemaKeywordItems = "items"

const schemaKeywordType = "type"
const schemaKeywordRef = "$ref"
const schemaKeywordDynamicRef = "$dynamicRef"
const schemaKeywordAnyOf = "anyOf"

type schemaBuilder struct {
	active map[reflect.Type]int
	order  int
}

type schemaFieldCandidate struct {
	name     string
	field    reflect.StructField
	owner    reflect.Type
	optional bool
	tagged   bool
	depth    int
	order    int
}

func newSchemaBuilder() *schemaBuilder {
	return &schemaBuilder{
		active: make(map[reflect.Type]int),
	}
}

func (b *schemaBuilder) schemaForType(t reflect.Type, depth int) (map[string]any, error) {
	if depth >= maxSchemaDepth {
		return nil, fmt.Errorf("%w: recursive or excessive-depth type %s", ErrSchemaUnsupported, t)
	}

	if t.Kind() == reflect.Pointer {
		schema, err := b.schemaForType(t.Elem(), depth+1)
		if err != nil {
			return nil, err
		}
		return nullableInferredSchema(schema), nil
	}
	base, err := normalizedSchemaType(t)
	if err != nil {
		return nil, err
	}
	if schema, ok, providerErr := schemaFromProvider(base); ok {
		return schema, providerErr
	}
	if base == reflect.TypeFor[json.Number]() {
		return map[string]any{"type": "number"}, nil
	}
	if hasCustomJSONCodec(base) {
		return nil, fmt.Errorf("%w: type %s has a custom codec; provide JSONSchema", ErrSchemaUnsupported, base)
	}

	switch base.Kind() {
	case reflect.Struct:
		return b.schemaForStructType(base, depth)
	case reflect.Slice:
		if base.Elem().Kind() == reflect.Uint8 {
			if hasCustomJSONCodec(base.Elem()) {
				return nil, fmt.Errorf("%w: byte element has a custom JSON codec", ErrSchemaUnsupported)
			}
			return nullableInferredSchema(map[string]any{"type": "string"}), nil
		}
		items, err := b.schemaForType(base.Elem(), depth+1)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		return nullableInferredSchema(map[string]any{
			"type": schemaTypeArray, schemaKeywordItems: items,
		}), nil
	case reflect.Array:
		items, err := b.schemaForType(base.Elem(), depth+1)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		return map[string]any{
			"type":     schemaTypeArray,
			"items":    items,
			"minItems": base.Len(), "maxItems": base.Len(),
		}, nil
	case reflect.String:
		return map[string]any{"type": "string"}, nil
	case reflect.Bool:
		return map[string]any{"type": "boolean"}, nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return map[string]any{"type": "integer"}, nil
	case reflect.Float32, reflect.Float64:
		return map[string]any{"type": "number"}, nil
	default:
		return nil, fmt.Errorf("schema: unsupported type %s", base)
	}
}

func (b *schemaBuilder) schemaForStructType(t reflect.Type, depth int) (map[string]any, error) {
	if depth >= maxSchemaDepth || b.active[t] > 0 {
		return nil, fmt.Errorf("%w: recursive or excessive-depth type %s", ErrSchemaUnsupported, t)
	}

	b.active[t]++
	defer func() {
		b.active[t]--
		if b.active[t] == 0 {
			delete(b.active, t)
		}
	}()

	fields, err := b.collectStructFields(t, depth, false)
	if err != nil {
		return nil, err
	}
	fields = resolveSchemaFieldCandidates(fields)

	properties := make(map[string]any, len(fields))
	required := make([]string, 0, len(fields))
	for _, field := range fields {
		schema, err := b.schemaForSelectedField(field)
		if err != nil {
			return nil, err
		}
		properties[field.name] = schema
		if !field.optional {
			required = append(required, field.name)
		}
	}

	schema := map[string]any{
		"type":                 "object",
		"properties":           properties,
		"additionalProperties": false,
	}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema, nil
}

func (b *schemaBuilder) collectStructFields(
	t reflect.Type,
	depth int,
	inheritedOptional bool,
) ([]schemaFieldCandidate, error) {
	candidates := make([]schemaFieldCandidate, 0, t.NumField())
	for field := range t.Fields() {
		if shouldSkipSchemaField(field) {
			continue
		}

		name, hasName, omitEmpty, ignore := parseJSONSchemaTag(field.Tag.Get("json"))
		if ignore {
			continue
		}
		baseFieldType := indirectSchemaType(field.Type)
		if field.Anonymous && !hasName && baseFieldType.Kind() == reflect.Struct {
			if depth+1 >= maxSchemaDepth || b.active[baseFieldType] > 0 {
				return nil, fmt.Errorf("%w: recursive embedded type %s", ErrSchemaUnsupported, baseFieldType)
			}

			b.active[baseFieldType]++
			embedded, err := b.collectStructFields(
				baseFieldType,
				depth+1,
				inheritedOptional || isPointerType(field.Type),
			)
			b.active[baseFieldType]--
			if b.active[baseFieldType] == 0 {
				delete(b.active, baseFieldType)
			}
			if err != nil {
				return nil, err
			}
			candidates = append(candidates, embedded...)
			continue
		}

		fieldName := name
		if fieldName == "" {
			fieldName = field.Name
		}

		candidates = append(candidates, schemaFieldCandidate{
			name:     fieldName,
			field:    field,
			owner:    t,
			optional: inheritedOptional || omitEmpty,
			tagged:   hasName,
			depth:    depth,
			order:    b.nextOrder(),
		})
	}
	return candidates, nil
}

func (b *schemaBuilder) schemaForSelectedField(candidate schemaFieldCandidate) (map[string]any, error) {
	field, t := candidate.field, candidate.owner
	if candidate.tagged && !jsonfield.ValidTagName(candidate.name) {
		return nil, fmt.Errorf(
			"%w: field %s.%s has unsupported JSON tag name %q",
			ErrSchemaUnsupported,
			t,
			field.Name,
			candidate.name,
		)
	}
	baseFieldType, err := normalizedSchemaType(field.Type)
	if err != nil {
		return nil, fmt.Errorf("schema: field %s.%s: %w", t, field.Name, err)
	}

	schema, err := b.schemaForType(field.Type, candidate.depth+1)
	if err != nil {
		return nil, fmt.Errorf("schema: field %s.%s: %w", t, field.Name, err)
	}

	if jsonTagOption(field.Tag.Get("json"), "string") {
		if baseFieldType.Implements(schemaProviderType) ||
			reflect.PointerTo(baseFieldType).Implements(schemaProviderType) {
			return nil, fmt.Errorf(
				"%w: field %s.%s combines JSONSchema with json string option; provide schema for enclosing type",
				ErrSchemaUnsupported,
				t,
				field.Name,
			)
		}
		schema, err = quotedJSONSchema(baseFieldType)
		if err != nil {
			return nil, fmt.Errorf("schema: field %s.%s: %w", t, field.Name, err)
		}
	}
	if isPointerType(field.Type) && jsonTagOption(field.Tag.Get("json"), "string") {
		schema = nullableInferredSchema(schema)
	}

	return schema, nil
}

func (b *schemaBuilder) nextOrder() int {
	order := b.order
	b.order++
	return order
}

func shouldSkipSchemaField(field reflect.StructField) bool {
	if field.Tag.Get("json") == "-" {
		return true
	}
	if field.IsExported() {
		return false
	}
	if !field.Anonymous {
		return true
	}
	return indirectSchemaType(field.Type).Kind() != reflect.Struct
}

func parseJSONSchemaTag(tag string) (string, bool, bool, bool) {
	if tag == "" {
		return "", false, false, false
	}
	if tag == "-" {
		return "", false, false, true
	}
	head, tail, _ := strings.Cut(tag, ",")
	var name string
	hasName := false
	if head != "" {
		name = head
		hasName = true
	}
	if tail == "" {
		return name, hasName, false, false
	}
	omitEmpty := false
	for opt := range strings.SplitSeq(tail, ",") {
		if opt == "omitempty" || opt == "omitzero" {
			omitEmpty = true
		}
	}
	return name, hasName, omitEmpty, false
}

func resolveSchemaFieldCandidates(candidates []schemaFieldCandidate) []schemaFieldCandidate {
	grouped := make(map[string][]schemaFieldCandidate)
	order := make([]string, 0, len(candidates))
	seen := make(map[string]bool, len(candidates))

	for _, candidate := range candidates {
		grouped[candidate.name] = append(grouped[candidate.name], candidate)
		if !seen[candidate.name] {
			order = append(order, candidate.name)
			seen[candidate.name] = true
		}
	}

	resolved := make([]schemaFieldCandidate, 0, len(grouped))
	for _, name := range order {
		group := grouped[name]
		if winner, ok := resolveSchemaFieldGroup(group); ok {
			resolved = append(resolved, winner)
		}
	}

	sort.Slice(resolved, func(i, j int) bool {
		return resolved[i].order < resolved[j].order
	})
	return resolved
}

func resolveSchemaFieldGroup(group []schemaFieldCandidate) (schemaFieldCandidate, bool) {
	if len(group) == 0 {
		return schemaFieldCandidate{}, false
	}

	minDepth := group[0].depth
	for _, candidate := range group[1:] {
		if candidate.depth < minDepth {
			minDepth = candidate.depth
		}
	}

	filtered := make([]schemaFieldCandidate, 0, len(group))
	tagged := make([]schemaFieldCandidate, 0, len(group))
	for _, candidate := range group {
		if candidate.depth != minDepth {
			continue
		}
		filtered = append(filtered, candidate)
		if candidate.tagged {
			tagged = append(tagged, candidate)
		}
	}

	if len(tagged) > 0 {
		filtered = tagged
	}
	if len(filtered) != 1 {
		return schemaFieldCandidate{}, false
	}
	return filtered[0], true
}

func indirectSchemaType(t reflect.Type) reflect.Type {
	for range maxSchemaDepth {
		if t.Kind() != reflect.Pointer {
			break
		}
		t = t.Elem()
	}
	return t
}

func isPointerType(t reflect.Type) bool {
	return t != nil && t.Kind() == reflect.Pointer
}

func jsonTagOption(tag, option string) bool {
	_, options, _ := strings.Cut(tag, ",")
	for value := range strings.SplitSeq(options, ",") {
		if value == option {
			return true
		}
	}
	return false
}

func hasCustomJSONCodec(t reflect.Type) bool {
	for _, candidate := range []reflect.Type{t, reflect.PointerTo(t)} {
		if candidate.Implements(reflect.TypeFor[json.Marshaler]()) ||
			candidate.Implements(reflect.TypeFor[json.Unmarshaler]()) ||
			candidate.Implements(reflect.TypeFor[encoding.TextMarshaler]()) ||
			candidate.Implements(reflect.TypeFor[encoding.TextUnmarshaler]()) {
			return true
		}
	}
	return false
}

func quotedJSONSchema(t reflect.Type) (map[string]any, error) {
	const numericPattern = `^-?(0|[1-9][0-9]*)(\.[0-9]+)?([eE][+-]?[0-9]+)?$`
	if t == reflect.TypeFor[json.Number]() {
		return map[string]any{"type": "string", "pattern": numericPattern}, nil
	}
	var pattern string
	switch t.Kind() {
	case reflect.Bool:
		pattern = "^(true|false)$"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		pattern = "^-?(0|[1-9][0-9]*)$"
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		pattern = "^(0|[1-9][0-9]*)$"
	case reflect.Float32, reflect.Float64:
		pattern = numericPattern
	case reflect.String:
		pattern = `^"([^"\\\x00-\x1f]|\\(["\\/bfnrt]|u[0-9a-fA-F]{4}))*"$`
	default:
		return nil, fmt.Errorf("%w: string option on %s", ErrSchemaUnsupported, t)
	}
	return map[string]any{"type": "string", "pattern": pattern}, nil
}

func nullableInferredSchema(schema map[string]any) map[string]any {
	return map[string]any{schemaKeywordAnyOf: []any{schema, map[string]any{"type": "null"}}}
}
