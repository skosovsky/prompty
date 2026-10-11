// Package exactjson compares JSON values without rounding decimal numbers.
package exactjson

import (
	"bytes"
	"encoding/json"
	"math/big"
	"reflect"
	"strings"
)

// Equal compares complete documents structurally. Invalid documents are never equal.
// Numeric spellings may differ when their exact decimal values are equal.
func Equal(left, right []byte) bool {
	a, ok := decode(left)
	if !ok {
		return false
	}
	b, ok := decode(right)
	return ok && equalValue(a, b)
}

func decode(document []byte) (any, bool) {
	if !json.Valid(document) {
		return nil, false
	}
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, false
	}
	return value, true
}

func equalValue(a, b any) bool {
	switch value := a.(type) {
	case json.Number:
		other, ok := b.(json.Number)
		if !ok {
			return false
		}
		ac, ae := decimal(value.String())
		bc, be := decimal(other.String())
		return ac == bc && ae.Cmp(be) == 0
	case map[string]any:
		other, ok := b.(map[string]any)
		return ok && equalObjects(value, other)
	case []any:
		other, ok := b.([]any)
		return ok && equalArrays(value, other)
	default:
		return reflect.DeepEqual(a, b)
	}
}

// decimal normalizes a valid JSON number into coefficient times ten to exponent.
// Exponents are kept symbolically: 1e1000000000 never allocates a billion digits.
func decimal(value string) (string, *big.Int) {
	mantissa, exponent, hasExponent := strings.Cut(strings.ToLower(value), "e")
	power := new(big.Int)
	if hasExponent {
		power.SetString(exponent, decimalBase)
	}
	negative := strings.HasPrefix(mantissa, "-")
	mantissa = strings.TrimPrefix(mantissa, "-")
	whole, fraction, _ := strings.Cut(mantissa, ".")
	coefficient := strings.TrimLeft(whole+fraction, "0")
	if coefficient == "" {
		return "0", new(big.Int)
	}
	trimmed := strings.TrimRight(coefficient, "0")
	shift := len(coefficient) - len(trimmed) - len(fraction)
	power.Add(power, big.NewInt(int64(shift)))
	if negative {
		trimmed = "-" + trimmed
	}
	return trimmed, power
}

const decimalBase = 10

func equalObjects(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		other, exists := b[key]
		if !exists || !equalValue(value, other) {
			return false
		}
	}
	return true
}
func equalArrays(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	for i, value := range a {
		if !equalValue(value, b[i]) {
			return false
		}
	}
	return true
}
