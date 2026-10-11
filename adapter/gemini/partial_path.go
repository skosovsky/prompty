package gemini

import (
	"encoding/json"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/skosovsky/prompty"
)

type argumentPathStep struct {
	name  string
	index int
	array bool
}

// Matches the maximum nesting accepted by the shared encoding/json validator.
const maxArgumentJSONDepth = 10000

// Only singular member/index paths can identify a parameter fragment. Queries,
// wildcards/slices/filters do not prove a unique argument and must not be guessed.
func parseArgumentPath(path string) ([]argumentPathStep, error) {
	if !strings.HasPrefix(path, "$") || !utf8.ValidString(path) {
		return nil, prompty.ErrStreamProtocol
	}
	remaining := path[1:]
	var steps []argumentPathStep
	for remaining != "" {
		var step argumentPathStep
		var err error
		switch remaining[0] {
		case '.':
			step, remaining, err = parseDotArgumentPath(remaining[1:])
		case '[':
			step, remaining, err = parseBracketArgumentPath(remaining[1:])
		default:
			return nil, prompty.ErrStreamProtocol
		}
		if err != nil {
			return nil, err
		}
		steps = append(steps, step)
		// The shared encoding/json validator cannot accept a deeper JSON value.
		// Bound the path before recursive materialization or cloning can exhaust stack.
		if len(steps) > maxArgumentJSONDepth {
			return nil, prompty.ErrStreamProtocol
		}
	}
	if len(steps) == 0 || steps[0].array {
		return nil, prompty.ErrStreamProtocol // Tool arguments must be an object.
	}
	return steps, nil
}

func parseDotArgumentPath(path string) (argumentPathStep, string, error) {
	end := 0
	for offset, char := range path {
		valid := char == '_' || char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= 0x80 ||
			offset > 0 && char >= '0' && char <= '9'
		if !valid {
			break
		}
		end = offset + utf8.RuneLen(char)
	}
	if end == 0 {
		return argumentPathStep{}, "", prompty.ErrStreamProtocol
	}
	return argumentPathStep{index: 0, array: false, name: path[:end]}, path[end:], nil
}

func parseBracketArgumentPath(path string) (argumentPathStep, string, error) {
	path = strings.TrimLeft(path, " \t\r\n")
	if path == "" {
		return argumentPathStep{}, "", prompty.ErrStreamProtocol
	}
	if path[0] == '\'' || path[0] == '"' {
		name, tail, err := quotedArgumentMember(path)
		tail = strings.TrimLeft(tail, " \t\r\n")
		if err != nil || !strings.HasPrefix(tail, "]") {
			return argumentPathStep{}, "", prompty.ErrStreamProtocol
		}
		return argumentPathStep{index: 0, array: false, name: name}, tail[1:], nil
	}
	end := strings.IndexByte(path, ']')
	if end < 0 {
		return argumentPathStep{}, "", prompty.ErrStreamProtocol
	}
	number := strings.TrimSpace(path[:end])
	index, err := strconv.Atoi(number)
	if err != nil || index < 0 || strconv.Itoa(index) != number {
		return argumentPathStep{}, "", prompty.ErrStreamProtocol
	}
	return argumentPathStep{name: "", index: index, array: true}, path[end+1:], nil
}

func quotedArgumentMember(path string) (string, string, error) {
	quote := path[0]
	var encoded strings.Builder
	encoded.WriteByte('"')
	for index := 1; index < len(path); index++ {
		char := path[index]
		if char == quote {
			encoded.WriteByte('"')
			var value string
			if json.Unmarshal([]byte(encoded.String()), &value) != nil {
				return "", "", prompty.ErrStreamProtocol
			}
			return value, path[index+1:], nil
		}
		switch char {
		case '\\':
			index++
			if index >= len(path) {
				return "", "", prompty.ErrStreamProtocol
			}
			char = path[index]
			if char != quote || quote == '"' {
				encoded.WriteByte('\\')
			}
		case '"':
			encoded.WriteByte('\\')
		}
		encoded.WriteByte(char)
	}
	return "", "", prompty.ErrStreamProtocol
}
