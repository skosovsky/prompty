package prompty

import (
	"errors"
	"fmt"
	"math"
	"strings"
)

var ErrComposeValues = errors.New("prompty: invalid compose values")

// RegistryPlanInput is an opaque token: payload already bound for template rendering.
type RegistryPlanInput struct {
	boundVars     map[string]any
	chatHistory   []ChatMessage
	historyValues map[string]any
	composeValues map[string]any
	composeSet    bool
}

// PlanInputFrom binds a typed payload for Registry.Plan (fail-fast before registry access).
func PlanInputFrom[T any](payload T) (RegistryPlanInput, error) {
	if check, ok := any(payload).(interface{ ValidateSchema() error }); ok {
		if err := check.ValidateSchema(); err != nil {
			return RegistryPlanInput{}, fmt.Errorf("plan input schema: %w", err)
		}
	}
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
		historyValues: historySchemaValues(payload),
		composeValues: nil,
		composeSet:    false,
	}, nil
}

func (r RegistryPlanInput) isEmpty() bool {
	return len(r.boundVars) == 0 && len(r.chatHistory) == 0 && len(r.historyValues) == 0
}

func cloneComposeMap(values map[string]any) map[string]any {
	return cloneMapAny(values)
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

// NewComposeValuesFromPairs builds programmer-supplied compose values.
// It panics on invalid paths/collisions; use CheckedComposeValuesFromPairs
// for error-return admission of dynamic input.
func NewComposeValuesFromPairs(pairs ...ComposeValue) ComposeValues {
	values, err := CheckedComposeValuesFromPairs(pairs...)
	if err != nil {
		panic(err)
	}
	return values
}

// CheckedComposeValuesFromPairs rejects invalid input atomically.
func CheckedComposeValuesFromPairs(pairs ...ComposeValue) (ComposeValues, error) {
	values := make(map[string]any)
	for _, pair := range pairs {
		if value, ok := pair.value.(float64); ok && (math.IsNaN(value) || math.IsInf(value, 0)) {
			return ComposeValues{}, fmt.Errorf("%w: nonfinite value for %q", ErrComposeValues, pair.path)
		}
		if err := setComposeDotPath(values, pair.path, pair.value); err != nil {
			return ComposeValues{}, err
		}
	}
	return ComposeValues{values: values, set: true}, nil
}

func setComposeDotPath(dst map[string]any, path string, value any) error {
	parts := strings.Split(path, ".")
	for _, part := range parts {
		if strings.TrimSpace(part) == "" {
			return fmt.Errorf("%w: empty segment in %q", ErrComposeValues, path)
		}
	}
	cur := dst
	for i, part := range parts {
		if i == len(parts)-1 {
			if _, exists := cur[part]; exists {
				return fmt.Errorf("%w: repeated or colliding path %q", ErrComposeValues, path)
			}
			cur[part] = value
			return nil
		}
		node, exists := cur[part]
		next, ok := node.(map[string]any)
		if exists && !ok {
			return fmt.Errorf("%w: scalar parent in %q", ErrComposeValues, path)
		}
		if !exists {
			next = make(map[string]any)
			cur[part] = next
		}
		cur = next
	}
	return nil
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

// Lookup returns an owned copy of a runtime compose value by dot path.
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
	return cloneAny(cur), true
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
func PlanInputWithComposeContext(in RegistryPlanInput, ctx ComposeContext) (RegistryPlanInput, error) {
	values, err := admittedComposeValues(ctx)
	if err != nil {
		return RegistryPlanInput{}, err
	}
	return planInputWithComposeValues(in, values), nil
}

func admittedComposeValues(ctx ComposeContext) (ComposeValues, error) {
	if ctx == nil {
		return ComposeValues{}, nil
	}
	if validator, ok := ctx.(interface{ ValidateComposeContext() error }); ok {
		if err := validator.ValidateComposeContext(); err != nil {
			return ComposeValues{}, err
		}
	}
	return ctx.ComposeValues(), nil
}

// ComposeValues returns runtime compose values for declarative manifest composition.
// The zero value means unset; a set empty value enables strict condition evaluation.
func (r RegistryPlanInput) ComposeValues() ComposeValues {
	if !r.composeSet {
		return ComposeValues{}
	}
	return newComposeValuesFromMap(r.composeValues)
}
