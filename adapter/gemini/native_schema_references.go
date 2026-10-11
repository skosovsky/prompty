package gemini

import (
	"net/url"
	"slices"
	"strings"
)

// nativeSchemaGraph indexes only validated schema positions, never instance objects.
type nativeSchemaGraph struct {
	nodes map[string]map[string]any
	edges map[string][]string
}

//nolint:gocognit // Deterministic reference edges followed by DFS colors.
func (graph *nativeSchemaGraph) validateReferences() error {
	pointers := make([]string, 0, len(graph.nodes))
	for pointer := range graph.nodes {
		pointers = append(pointers, pointer)
	}
	slices.Sort(pointers)
	for _, pointer := range pointers {
		if ref, ok := graph.nodes[pointer][nativeReferenceKeyword].(string); ok {
			target, err := nativeReferencePointer(ref)
			if err != nil {
				return fidelityError(pointerKey(pointer, nativeReferenceKeyword), "invalid local JSON pointer")
			}
			if graph.nodes[target] == nil {
				return fidelityError(
					pointerKey(pointer, nativeReferenceKeyword),
					"reference target must be an indexed local schema",
				)
			}
			graph.edges[pointer] = append(graph.edges[pointer], target)
		}
	}
	colors := make(map[string]uint8, len(graph.nodes))
	var visit func(string) error
	visit = func(pointer string) error {
		if colors[pointer] == 1 {
			return fidelityError(pointer, "recursive schemas are outside the strict native JSON profile")
		}
		if colors[pointer] == 2 {
			return nil
		}
		colors[pointer] = 1
		for _, target := range graph.edges[pointer] {
			if err := visit(target); err != nil {
				return err
			}
		}
		colors[pointer] = 2
		return nil
	}
	for _, pointer := range pointers {
		if err := visit(pointer); err != nil {
			return err
		}
	}
	return nil
}

// Decode a URI fragment once, validate RFC6901 escapes, then use canonical pointers.
func nativeReferencePointer(ref string) (string, error) {
	decoded, err := url.PathUnescape(strings.TrimPrefix(ref, "#"))
	if err != nil {
		return "", err
	}
	pointer := ""
	for segment := range strings.SplitSeq(strings.TrimPrefix(decoded, "/"), "/") {
		for i := 0; i < len(segment); i++ {
			if segment[i] == '~' {
				if i+1 == len(segment) || (segment[i+1] != '0' && segment[i+1] != '1') {
					return "", fidelityError("", "invalid JSON pointer escape")
				}
				i++
			}
		}
		name := strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		pointer = pointerKey(pointer, name)
	}
	return pointer, nil
}
