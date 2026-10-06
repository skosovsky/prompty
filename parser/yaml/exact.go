package yaml

import (
	"encoding/json"
	"fmt"
	"math/big"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// exactMap preserves YAML numeric values at the JSON boundary.
type exactMap map[string]any

func (m *exactMap) UnmarshalYAML(node *yaml.Node) error {
	value, err := exactYAMLValue(node, make(map[*yaml.Node]bool))
	if err != nil {
		return err
	}
	if value == nil {
		*m = nil
		return nil
	}
	object, ok := value.(map[string]any)
	if !ok {
		return fmt.Errorf("line %d: expected JSON object", node.Line)
	}
	*m = object
	return nil
}

func exactYAMLValue(node *yaml.Node, active map[*yaml.Node]bool) (any, error) {
	if active[node] {
		return nil, fmt.Errorf("line %d: recursive YAML alias", node.Line)
	}
	active[node] = true
	defer delete(active, node)
	switch node.Kind {
	case yaml.AliasNode:
		return exactYAMLValue(node.Alias, active)
	case yaml.MappingNode:
		return exactYAMLObject(node, active)
	case yaml.SequenceNode:
		values := make([]any, len(node.Content))
		for i, item := range node.Content {
			value, err := exactYAMLValue(item, active)
			if err != nil {
				return nil, err
			}
			values[i] = value
		}
		return values, nil
	case yaml.ScalarNode:
		return exactYAMLScalar(node)
	default:
		return nil, fmt.Errorf("line %d: unsupported YAML node", node.Line)
	}
}

const decimalBase = 10
const yamlStringTag = "!!str"

var yamlIntegerToken = regexp.MustCompile(`^[+-]?(?:0[xX][0-9a-fA-F_]+|0[oO][0-7_]+|0[bB][01_]+|[0-9][0-9_]*)$`)
var yamlDecimalToken = regexp.MustCompile(`^[+-]?(?:[0-9][0-9_]*(?:\.[0-9_]*)?|\.[0-9_]+)(?:[eE][+-]?[0-9_]+)?$`)

func exactYAMLScalar(node *yaml.Node) (any, error) {
	// yaml.v3 falls back to string when its numeric resolver overflows. Recover
	// plain numeric tokens without reinterpreting quoted or explicitly tagged strings.
	if node.Tag == yamlStringTag && node.Style == 0 {
		copyNode := *node
		switch {
		case yamlIntegerToken.MatchString(node.Value):
			copyNode.Tag = "!!int"
		case yamlDecimalToken.MatchString(node.Value):
			copyNode.Tag = "!!float"
		default:
			return node.Value, nil
		}
		return exactYAMLScalar(&copyNode)
	}
	switch node.Tag {
	case "!!int":
		token := strings.ReplaceAll(node.Value, "_", "")
		number, ok := new(big.Int).SetString(token, 0)
		if !ok {
			number, ok = new(big.Int).SetString(token, decimalBase)
		}
		if !ok {
			return nil, fmt.Errorf("line %d: invalid integer", node.Line)
		}
		return json.Number(number.String()), nil
	case "!!float":
		return exactYAMLDecimal(node)
	case yamlStringTag:
		return node.Value, nil
	case "!!null":
		return nil, nil //nolint:nilnil // JSON null is a valid value.
	case "!!bool":
		var value bool
		if err := node.Decode(&value); err != nil {
			return nil, err
		}
		return value, nil
	default:
		return nil, fmt.Errorf("line %d: YAML tag %q is not JSON", node.Line, node.Tag)
	}
}

func exactYAMLObject(node *yaml.Node, active map[*yaml.Node]bool) (any, error) {
	object := make(map[string]any, len(node.Content)/2)
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i]
		if key.Tag != yamlStringTag {
			return nil, fmt.Errorf("line %d: JSON object key must be a string", key.Line)
		}
		if _, exists := object[key.Value]; exists {
			return nil, fmt.Errorf("line %d: duplicate key %q", key.Line, key.Value)
		}
		value, err := exactYAMLValue(node.Content[i+1], active)
		if err != nil {
			return nil, err
		}
		object[key.Value] = value
	}
	return object, nil
}

func exactYAMLDecimal(node *yaml.Node) (any, error) {
	token := strings.ReplaceAll(node.Value, "_", "")
	token = strings.TrimPrefix(token, "+")
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(token), "e")
	integer, fraction, hasFraction := strings.Cut(mantissa, ".")
	negative := strings.HasPrefix(integer, "-")
	integer = strings.TrimLeft(strings.TrimPrefix(integer, "-"), "0")
	if integer == "" {
		integer = "0"
	}
	if negative {
		integer = "-" + integer
	}
	token = integer
	if hasFraction {
		if fraction == "" {
			fraction = "0"
		}
		token += "." + fraction
	}
	if hasExponent {
		token += "e" + exponent
	}
	if !json.Valid([]byte(token)) {
		return nil, fmt.Errorf("line %d: number %q cannot be represented as JSON", node.Line, node.Value)
	}
	return json.Number(token), nil
}
