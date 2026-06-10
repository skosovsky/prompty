package late

// PropertyIsLate reports whether an input schema property is late-bound.
func PropertyIsLate(prop map[string]any) bool {
	if prop == nil {
		return false
	}
	if v, ok := prop["late"].(bool); ok && v {
		return true
	}
	if v, ok := prop["x-prompty-late"].(bool); ok && v {
		return true
	}
	return false
}

// FilterEarlyRequired drops late-bound names from a required list.
func FilterEarlyRequired(required []string, props map[string]any) []string {
	if len(required) == 0 {
		return nil
	}
	out := make([]string, 0, len(required))
	for _, name := range required {
		raw, _ := props[name].(map[string]any)
		if PropertyIsLate(raw) {
			continue
		}
		out = append(out, name)
	}
	return out
}
