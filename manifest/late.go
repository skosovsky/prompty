package manifest

import "github.com/skosovsky/prompty/internal/late"

// PropertyIsLate reports whether an input schema property is late-bound.
func PropertyIsLate(prop map[string]any) bool {
	return late.PropertyIsLate(prop)
}

// FilterEarlyRequired drops late-bound names from a required list.
func FilterEarlyRequired(required []string, props map[string]any) []string {
	return late.FilterEarlyRequired(required, props)
}
