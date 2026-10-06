package gemini

import (
	"bytes"
	"encoding/json"

	"github.com/skosovsky/prompty"
)

// partialArgument retains original number lexemes and explicit value presence.
// SDK PartialArg.NumberValue is float64 and cannot establish that fidelity.
type partialArgument struct {
	path      string
	value     any
	present   bool
	continues bool
}

func decodePartialArgument(payload json.RawMessage) (partialArgument, error) {
	var raw struct {
		Path     string          `json:"jsonPath"`
		Number   json.RawMessage `json:"numberValue"`
		String   json.RawMessage `json:"stringValue"`
		Bool     json.RawMessage `json:"boolValue"`
		Null     json.RawMessage `json:"nullValue"`
		Continue json.RawMessage `json:"willContinue"`
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if !json.Valid(payload) || uniqueGeminiJSONValue(json.NewDecoder(bytes.NewReader(payload))) != nil ||
		!validPartialArgumentFields(payload) || decoder.Decode(&raw) != nil || raw.Path == "" {
		return partialArgument{}, prompty.ErrStreamProtocol
	}
	argument := partialArgument{path: raw.Path}
	if len(raw.Continue) != 0 {
		if !bytes.Equal(raw.Continue, []byte("true")) && !bytes.Equal(raw.Continue, []byte("false")) {
			return partialArgument{}, prompty.ErrStreamProtocol
		}
		argument.continues = bytes.Equal(raw.Continue, []byte("true"))
	}
	count := 0
	for _, field := range []struct {
		raw  json.RawMessage
		kind string
	}{
		{raw.Number, "number"}, {raw.String, "string"}, {raw.Bool, "bool"}, {raw.Null, "null"},
	} {
		if len(field.raw) == 0 {
			continue
		}
		count++
		value, err := decodePartialValue(field.raw, field.kind)
		if err != nil {
			return partialArgument{}, err
		}
		argument.value, argument.present = value, true
	}
	if count > 1 || argument.continues && (count != 1 || len(raw.String) == 0) {
		return partialArgument{}, prompty.ErrStreamProtocol
	}
	return argument, nil
}

func validPartialArgumentFields(payload []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(payload, &fields) != nil || fields == nil {
		return false
	}
	for key := range fields {
		switch key {
		case "jsonPath", "numberValue", "stringValue", "boolValue", "nullValue", "willContinue":
		default:
			return false // Go struct decoder case aliases are not protocol fields.
		}
	}
	return true
}

func decodePartialValue(raw json.RawMessage, kind string) (any, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return nil, prompty.ErrStreamProtocol
	}
	valid := false
	switch kind {
	case "number":
		_, valid = value.(json.Number)
	case "string":
		_, valid = value.(string)
	case "bool":
		_, valid = value.(bool)
	case "null":
		// REST uses JSON null; SDK-semantic fixtures may use the enum name.
		// Both are an explicit present null, never an absent closing marker.
		valid = value == nil || value == "NULL_VALUE"
		value = nil
	}
	if !valid {
		return nil, prompty.ErrStreamProtocol
	}
	return value, nil
}
