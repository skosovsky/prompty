package gemini

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type schemaDispatchFixture struct {
	*Adapter

	calls int
}

func (f *schemaDispatchFixture) Execute(context.Context, *Request) (*genai.GenerateContentResponse, error) {
	f.calls++
	return nil, nil //nolint:nilnil // No mapped value or emitted frame is a successful absence, not an operation failure.
}

func TestStrictSchemaRejectsLossBeforeDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, schema, pointer string
	}{
		{"numeric enum", `{"type":"integer","enum":[1,2]}`, "/enum/0"},
		{"minimum", `{"type":"number","minimum":1}`, "/minimum"},
		{"reference", `{"$ref":"https://unprovided.invalid/schema"}`, "/$ref"},
		{"nested escaped name", `{"properties":{"a~/b":{"items":{"minimum":1}}}}`, "/properties/a~0~1b/items/minimum"},
		{"boolean schema", `{"properties":{"x":false}}`, "/properties/x"},
		{"type union", `{"type":["string","null"]}`, "/type"},
		{"additional properties", `{"type":"object","additionalProperties":false}`, "/additionalProperties"},
		{"composition", `{"anyOf":[{"type":"string"}]}`, "/anyOf"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: only schema mapping, not capability knowledge, can reject this request.
			profile := prompty.CapabilityProfile{
				Identity: prompty.ProfileIdentity{
					Provider: "gemini",
					Endpoint: "https://fixture.invalid",
					Model:    "fixture",
				},
				Provenance: "local strict mapping fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
				},
			}
			fixture := &schemaDispatchFixture{
				Adapter: New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile)),
			}
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(tt.schema)}
			if tt.name == "reference" {
				// Source is valid via an explicit resource; the mapper still cannot preserve $ref.
				// Unprovided-reference failure before mapping has a separate core regression.
				exec.Validator = prompty.NewJSONSchemaValidator(map[string]prompty.JSONDocument{
					"https://unprovided.invalid/schema": prompty.JSONDocument(`{"type":"string"}`),
				})
			}
			// Act.
			_, err := adapter.NewClient(fixture).Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			var fidelity *prompty.SchemaFidelityError
			require.ErrorAs(t, err, &fidelity)
			require.Equal(t, tt.pointer, fidelity.Pointer)
			require.Zero(t, fixture.calls)
			require.Equal(t, tt.schema, string(exec.ResponseFormat.Schema))
		})
	}
}

func TestStrictSchemaMappingRejectsMalformedShapes(t *testing.T) {
	t.Parallel()
	for _, source := range []map[string]any{
		{"type": "unknown"}, {"properties": nil}, {"items": false},
		{"enum": "a"}, {"enum": []any{"a", 2}}, {"required": []any{1}},
		{"description": false},
	} {
		// Arrange / Act.
		mapped, err := mapToGenaiSchema(source)
		// Assert.
		require.Nil(t, mapped)
		require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
	}
}

func TestStrictSchemaErrorPointerIsDeterministic(t *testing.T) {
	t.Parallel()
	// Arrange: unsupported nested fields have stable lexical traversal.
	source := map[string]any{"properties": map[string]any{
		"z": map[string]any{"maximum": 10}, "a": map[string]any{"minimum": 1},
	}}
	for range 50 {
		// Act.
		_, err := mapToGenaiSchema(source)
		// Assert.
		var fidelity *prompty.SchemaFidelityError
		require.ErrorAs(t, err, &fidelity)
		require.Equal(t, "/properties/a/minimum", fidelity.Pointer)
	}
}
