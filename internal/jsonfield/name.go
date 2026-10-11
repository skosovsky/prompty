// Package jsonfield defines the encoding/json field-name subset shared by
// schema inference and generated DTOs.
package jsonfield

import (
	"strings"
	"unicode"
)

// ValidTagName reports whether encoding/json can use name as a struct tag name.
func ValidTagName(name string) bool {
	for _, character := range name {
		if strings.ContainsRune("!#$%&()*+-./:;<=>?@[]^_{|}~ ", character) {
			continue
		}
		if !unicode.IsLetter(character) && !unicode.IsDigit(character) {
			return false
		}
	}
	return name != ""
}
