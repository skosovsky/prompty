package hostdispatch

import (
	"bytes"
	"encoding/json"
	"reflect"
	"slices"
	"unicode/utf8"

	"github.com/skosovsky/contexty"

	"github.com/skosovsky/prompty"
)

const rawArgsType = "host.prompty-call-args"

type rawCall struct {
	Index int    `json:"index"`
	ID    string `json:"id"`
	Name  string `json:"name"`
	Args  string `json:"args"`
}

type rawArguments struct {
	Calls []rawCall `json:"calls"`
}

func (rawArguments) ExtensionType() string { return rawArgsType }
func (raw rawArguments) CloneExtension() contexty.Extension {
	return rawArguments{Calls: slices.Clone(raw.Calls)}
}

// ContextCodec registers the caller sidecar that preserves exact argument bytes
// across semantic AST JSON serialization. It must be used on save and restore.
func ContextCodec() contexty.MessageCodec {
	codec := contexty.DefaultMessageCodec()
	codec.Extensions.Register(rawArgsType, func(data []byte) (contexty.Extension, error) {
		var raw rawArguments
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&raw); err != nil {
			return nil, ErrProjection
		}
		return raw, nil
	})
	return codec
}

func contextRawArguments(message contexty.Message) (map[int]rawCall, error) {
	calls := make(map[int]rawCall)
	if len(message.Extensions) == 0 {
		return calls, nil
	}
	if len(message.Extensions) != 1 {
		return nil, ErrProjection
	}
	raw, ok := message.Extensions[0].(rawArguments)
	if !ok || len(raw.Calls) == 0 {
		return nil, ErrProjection
	}
	for _, call := range raw.Calls {
		if !utf8.ValidString(call.Args) || !utf8.ValidString(call.ID) || !utf8.ValidString(call.Name) ||
			call.Index < 0 ||
			call.Index >= len(message.Parts) {
			return nil, ErrProjection
		}
		if _, exists := calls[call.Index]; exists {
			return nil, ErrProjection
		}
		part, ok := message.Parts[call.Index].(contexty.ToolCallPart)
		if !ok || part.ID != call.ID || part.Name != call.Name {
			return nil, ErrProjection
		}
		original, err := prompty.JSONDocumentAsMap(prompty.JSONDocument(call.Args))
		if err != nil {
			return nil, ErrProjection
		}
		current, err := prompty.JSONDocumentAsMap(part.Arguments.Data)
		if err != nil || !reflect.DeepEqual(original, current) {
			return nil, ErrProjection
		}
		calls[call.Index] = call
	}
	return calls, nil
}
