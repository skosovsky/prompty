package prompty

import (
	"reflect"
	"strings"
	"sync"
	"unicode"
)

// structBinding maps template input aliases (prompt/json/snake_case) to struct field indices.
// Built once per [reflect.Type] and cached for zero-reflection hot paths in Execute().
type structBinding struct {
	fields     []bindingField
	aliasToIdx map[string]int
	historyIdx int // -1 when absent
	hasHistory bool
}

type bindingField struct {
	index int
	alias string
}

//nolint:gochecknoglobals // per-type binding cache keyed by reflect.Type
var structBindingCache sync.Map

func structTypeOf(v any) (reflect.Type, reflect.Value, bool) {
	if v == nil {
		return nil, reflect.Value{}, false
	}
	val := reflect.ValueOf(v)
	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil, reflect.Value{}, false
		}
		val = val.Elem()
	}
	if val.Kind() != reflect.Struct {
		return nil, reflect.Value{}, false
	}
	return val.Type(), val, true
}

func getStructBinding(typ reflect.Type) (*structBinding, error) {
	if cached, ok := structBindingCache.Load(typ); ok {
		if b, typeOK := cached.(*structBinding); typeOK {
			return b, nil
		}
	}
	b, err := buildStructBinding(typ)
	if err != nil {
		return nil, err
	}
	actual, _ := structBindingCache.LoadOrStore(typ, b)
	if stored, typeOK := actual.(*structBinding); typeOK {
		return stored, nil
	}
	return b, nil
}

func buildStructBinding(typ reflect.Type) (*structBinding, error) {
	b := &structBinding{
		fields:     nil,
		aliasToIdx: make(map[string]int),
		historyIdx: -1,
		hasHistory: false,
	}
	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		if field.Type == reflect.TypeFor[[]ChatMessage]() {
			b.hasHistory = true
			b.historyIdx = i
			continue
		}
		aliases := fieldAliases(field)
		if len(aliases) == 0 {
			continue
		}
		idx := len(b.fields)
		b.fields = append(b.fields, bindingField{index: i, alias: aliases[0]})
		for _, alias := range aliases {
			if alias == "" || alias == "-" {
				continue
			}
			if _, exists := b.aliasToIdx[alias]; exists {
				continue
			}
			b.aliasToIdx[alias] = idx
		}
	}
	if len(b.fields) == 0 && !b.hasHistory {
		return nil, ErrInvalidPayload
	}
	return b, nil
}

func fieldAliases(field reflect.StructField) []string {
	var out []string
	seen := make(map[string]bool)
	add := func(s string) {
		s = strings.TrimSpace(strings.Split(s, ",")[0])
		if s == "" || s == "-" || seen[s] {
			return
		}
		seen[s] = true
		out = append(out, s)
	}
	add(field.Tag.Get("prompt"))
	add(field.Tag.Get("json"))
	add(camelToSnake(field.Name))
	add(field.Name)
	return out
}

func camelToSnake(s string) string {
	if s == "" {
		return ""
	}
	var b strings.Builder
	const snakeCaseExtraCap = 4
	b.Grow(len(s) + snakeCaseExtraCap)
	for i, r := range s {
		if unicode.IsUpper(r) {
			if i > 0 {
				b.WriteByte('_')
			}
			b.WriteRune(unicode.ToLower(r))
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

// extractStructPayload returns history and keeps struct value for direct template binding.
func extractStructPayload(input any) (reflect.Value, *structBinding, []ChatMessage, error) {
	typ, val, ok := structTypeOf(input)
	if !ok {
		return reflect.Value{}, nil, nil, ErrInvalidPayload
	}
	binding, err := getStructBinding(typ)
	if err != nil {
		return reflect.Value{}, nil, nil, err
	}
	var history []ChatMessage
	if binding.hasHistory {
		hv := val.Field(binding.historyIdx)
		if hv.CanInterface() {
			if cm, ok := hv.Interface().([]ChatMessage); ok {
				history = cm
			}
		}
	}
	for _, fi := range binding.fields {
		if fi.alias == "Tools" {
			return reflect.Value{}, nil, nil, ErrReservedVariable
		}
	}
	return val, binding, history, nil
}

func validateStructInputVars(tpl *ChatPromptTemplate, val reflect.Value, binding *structBinding) error {
	required := mergeRequiredVars(tpl.RequiredVars, tpl.requiredFromAST)
	for _, name := range required {
		if tpl.PartialVariables != nil {
			if _, ok := tpl.PartialVariables[name]; ok {
				continue
			}
		}
		idx, ok := binding.aliasToIdx[name]
		if !ok {
			return &VariableError{
				Variable: name,
				Template: tpl.Metadata.ID,
				Err:      ErrMissingVariable,
			}
		}
		fi := binding.fields[idx]
		fv := val.Field(fi.index)
		if !fieldValuePresent(fv) {
			return &VariableError{
				Variable: name,
				Template: tpl.Metadata.ID,
				Err:      ErrMissingVariable,
			}
		}
	}
	return nil
}

func fieldValuePresent(v reflect.Value) bool {
	if !v.IsValid() {
		return false
	}
	switch v.Kind() {
	case reflect.Pointer, reflect.Interface, reflect.Map, reflect.Slice:
		return !v.IsNil()
	default:
		return !v.IsZero()
	}
}

//nolint:gochecknoglobals // pooled map projection for text/template (.Input field access)
var boundInputMapPool = sync.Pool{
	New: func() any { return make(map[string]any) },
}

// buildStructTemplateInput projects struct fields into a pooled map for text/template execution.
// Validation and alias resolution use struct metadata directly; map materialization is limited to template engine needs.
func buildStructTemplateInput(tpl *ChatPromptTemplate, val reflect.Value, binding *structBinding) map[string]any {
	m := borrowBoundInputMap(val, binding)
	if tpl != nil && tpl.PartialVariables != nil {
		for k, v := range tpl.PartialVariables {
			if _, exists := m[k]; !exists {
				m[k] = v
			}
		}
	}
	return m
}

// borrowBoundInputMap fills a pooled map from struct using cached binding metadata.
func borrowBoundInputMap(val reflect.Value, binding *structBinding) map[string]any {
	m, ok := boundInputMapPool.Get().(map[string]any)
	if !ok || m == nil {
		m = make(map[string]any, len(binding.aliasToIdx))
	}
	clear(m)
	for alias, fiIdx := range binding.aliasToIdx {
		fi := binding.fields[fiIdx]
		fv := val.Field(fi.index)
		if fv.CanInterface() {
			m[alias] = fv.Interface()
		}
	}
	return m
}

func releaseBoundInputMap(m map[string]any) {
	if m == nil {
		return
	}
	clear(m)
	boundInputMapPool.Put(m)
}
