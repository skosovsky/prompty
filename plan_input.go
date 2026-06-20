package prompty

import (
	"fmt"
	"maps"
	"strings"
)

// RegistryPlanInput is an opaque token: payload already bound for template rendering.
type RegistryPlanInput struct {
	boundVars     map[string]any
	chatHistory   []ChatMessage
	composeValues map[string]any
	composeSet    bool
}

// PlanInputFrom binds a typed payload for Registry.Plan (fail-fast before registry access).
func PlanInputFrom[T any](payload T) (RegistryPlanInput, error) {
	vars, history, err := bindTemplateVars(payload)
	if err != nil {
		return RegistryPlanInput{}, fmt.Errorf("plan input: %w", err)
	}
	if vars == nil {
		vars = make(map[string]any)
	}
	return RegistryPlanInput{
		boundVars:     vars,
		chatHistory:   history,
		composeValues: nil,
		composeSet:    false,
	}, nil
}

func (r RegistryPlanInput) isEmpty() bool {
	return len(r.boundVars) == 0 && len(r.chatHistory) == 0
}

func cloneComposeMap(values map[string]any) map[string]any {
	if values == nil {
		return nil
	}
	out := make(map[string]any, len(values))
	for k, v := range values {
		if nested, ok := v.(map[string]any); ok {
			out[k] = maps.Clone(nested)
			continue
		}
		out[k] = v
	}
	return out
}

// ComposeValues is an opaque typed token for manifest condition.match evaluation.
// The zero value means "unset". Values built through NewComposeValuesFromPairs
// are set, including an intentionally empty value set.
type ComposeValues struct {
	values map[string]any
	set    bool
}

// ComposeValue is one typed condition.match value for ComposeValues.
type ComposeValue struct {
	path  string
	value any
}

// ComposeBool builds one bool compose value.
func ComposeBool(path string, value bool) ComposeValue {
	return ComposeValue{path: path, value: value}
}

// ComposeString builds one string compose value.
func ComposeString(path string, value string) ComposeValue {
	return ComposeValue{path: path, value: value}
}

// ComposeInt builds one integer compose value.
func ComposeInt(path string, value int64) ComposeValue {
	return ComposeValue{path: path, value: value}
}

// ComposeFloat builds one number compose value.
func ComposeFloat(path string, value float64) ComposeValue {
	return ComposeValue{path: path, value: value}
}

// newComposeValuesFromMap builds a compose value token from registry/runtime internals.
// The input map is cloned. Passing an empty map enables strict runtime compose evaluation.
func newComposeValuesFromMap(values map[string]any) ComposeValues {
	return ComposeValues{values: cloneComposeMap(values), set: true}
}

// NewComposeValuesFromPairs builds compose values from typed path/value pairs.
func NewComposeValuesFromPairs(pairs ...ComposeValue) ComposeValues {
	values := make(map[string]any)
	for _, pair := range pairs {
		if pair.path == "" {
			continue
		}
		setComposeDotPath(values, pair.path, pair.value)
	}
	return ComposeValues{values: values, set: true}
}

func setComposeDotPath(dst map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	cur := dst
	for i, part := range parts {
		if part == "" {
			return
		}
		if i == len(parts)-1 {
			cur[part] = value
			return
		}
		next, ok := cur[part].(map[string]any)
		if !ok {
			next = make(map[string]any)
			cur[part] = next
		}
		cur = next
	}
}

// IsSet reports whether compose values were intentionally provided.
func (v ComposeValues) IsSet() bool {
	return v.set
}

func (v ComposeValues) mapValue() map[string]any {
	if !v.set {
		return nil
	}
	if v.values == nil {
		return map[string]any{}
	}
	return cloneComposeMap(v.values)
}

// Lookup returns a runtime compose value by dot path.
func (v ComposeValues) Lookup(path string) (any, bool) {
	if !v.set || path == "" {
		return nil, false
	}
	parts := strings.Split(path, ".")
	var cur any = v.values
	for _, part := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		next, ok := m[part]
		if !ok {
			return nil, false
		}
		cur = next
	}
	return cur, true
}

// ComposeContext is the typed public contract generated prompt packages can implement.
// It keeps application code away from nested map construction.
type ComposeContext interface {
	ComposeValues() ComposeValues
}

func planInputWithComposeValues(in RegistryPlanInput, values ComposeValues) RegistryPlanInput {
	if !values.IsSet() {
		return in
	}
	in.composeValues = values.mapValue()
	in.composeSet = true
	return in
}

// PlanInputWithComposeContext attaches runtime compose values from a typed compose context.
func PlanInputWithComposeContext(in RegistryPlanInput, ctx ComposeContext) RegistryPlanInput {
	if ctx == nil {
		return in
	}
	return planInputWithComposeValues(in, ctx.ComposeValues())
}

// ComposeValues returns runtime compose values for declarative manifest composition.
// The zero value means unset; a set empty value enables strict condition evaluation.
func (r RegistryPlanInput) ComposeValues() ComposeValues {
	if !r.composeSet {
		return ComposeValues{}
	}
	return newComposeValuesFromMap(r.composeValues)
}
