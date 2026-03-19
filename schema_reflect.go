package prompty

import (
	"fmt"
	"reflect"
	"sort"
	"strings"
)

const maxSchemaDepth = 32

type schemaBuilder struct {
	active map[reflect.Type]int
	order  int
}

type schemaFieldCandidate struct {
	name     string
	schema   map[string]any
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
	base, err := normalizedSchemaType(t)
	if err != nil {
		return nil, err
	}
	if schema, ok := schemaFromProvider(base); ok {
		return schema, nil
	}

	switch base.Kind() {
	case reflect.Struct:
		return b.schemaForStructType(base, depth)
	case reflect.Slice:
		if base.Elem().Kind() == reflect.Uint8 {
			return map[string]any{"type": "string"}, nil
		}
		items, err := b.schemaForType(base.Elem(), depth+1)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		return map[string]any{
			"type":  "array",
			"items": items,
		}, nil
	case reflect.Array:
		items, err := b.schemaForType(base.Elem(), depth+1)
		if err != nil {
			return nil, fmt.Errorf("items: %w", err)
		}
		return map[string]any{
			"type":  "array",
			"items": items,
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
		return terminalObjectSchema(), nil
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
		properties[field.name] = field.schema
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

func (b *schemaBuilder) collectStructFields(t reflect.Type, depth int, inheritedOptional bool) ([]schemaFieldCandidate, error) {
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
				continue
			}

			b.active[baseFieldType]++
			embedded, err := b.collectStructFields(baseFieldType, depth+1, inheritedOptional || isPointerType(field.Type))
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

		schema, err := b.schemaForType(field.Type, depth+1)
		if err != nil {
			return nil, fmt.Errorf("schema: field %s.%s: %w", t, field.Name, err)
		}

		candidates = append(candidates, schemaFieldCandidate{
			name:     fieldName,
			schema:   schema,
			optional: inheritedOptional || omitEmpty || isPointerType(field.Type),
			tagged:   hasName,
			depth:    depth,
			order:    b.nextOrder(),
		})
	}
	return candidates, nil
}

func (b *schemaBuilder) nextOrder() int {
	order := b.order
	b.order++
	return order
}

func terminalObjectSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"properties":           map[string]any{},
		"additionalProperties": false,
	}
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

func parseJSONSchemaTag(tag string) (name string, hasName bool, omitEmpty bool, ignore bool) {
	if tag == "" {
		return "", false, false, false
	}
	if tag == "-" {
		return "", false, false, true
	}
	head, tail, _ := strings.Cut(tag, ",")
	if head != "" {
		name = head
		hasName = true
	}
	if tail == "" {
		return name, hasName, false, false
	}
	for opt := range strings.SplitSeq(tail, ",") {
		if opt == "omitempty" {
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
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	return t
}

func isPointerType(t reflect.Type) bool {
	return t != nil && t.Kind() == reflect.Pointer
}
