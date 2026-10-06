package ollama

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

type schemaTransportFixture struct {
	*Adapter

	calls int
}

func (f *schemaTransportFixture) Execute(context.Context, *api.ChatRequest) (*api.ChatResponse, error) {
	f.calls++
	return nil, nil
}

func TestUnsupportedToolSchemaFailsBeforeDispatch(t *testing.T) {
	t.Parallel()
	for _, tt := range []struct{ name, schema, pointer string }{
		{"root keyword", `{"type":"object","additionalProperties":false}`, "/additionalProperties"},
		{"nested minimum", `{"type":"object","properties":{"x":{"type":"integer","minimum":10}}}`, "/properties/x/minimum"},
		{"nested required", `{"type":"object","properties":{"x":{"type":"object","required":["a"]}}}`, "/properties/x/required"},
		{"enum precision", `{"type":"object","properties":{"x":{"type":"integer","enum":[9007199254740993]}}}`, "/properties/x/enum/0"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: source schema is valid; only SDK mapping causes rejection.
			profile := prompty.CapabilityProfile{
				Identity: prompty.ProfileIdentity{
					Provider: "ollama",
					Endpoint: "https://fixture.invalid",
					Model:    "fixture",
				},
				Provenance: "local tool schema conformance fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityTools: prompty.CapabilitySupported,
				},
			}
			fixture := &schemaTransportFixture{
				Adapter: New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint, profile)),
			}
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(tt.schema)}}
			// Act.
			_, err := adapter.NewClient(fixture).Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
			var loss *prompty.SchemaFidelityError
			require.ErrorAs(t, err, &loss)
			require.Equal(t, tt.pointer, loss.Pointer)
			require.Zero(t, fixture.calls)
			require.Equal(t, tt.schema, string(exec.Tools[0].Parameters))
		})
	}
}

func TestRepresentableToolSchemaSurvivesSDKSerialization(t *testing.T) {
	t.Parallel()
	// Arrange.
	source := prompty.JSONDocument(
		`{"type":"object","properties":{"x":{"type":"integer","enum":[1,2]}},"required":["x"]}`,
	)
	// Act.
	params, err := decodeToolSchema(source)
	require.NoError(t, err)
	wire, err := json.Marshal(params)
	// Assert.
	require.NoError(t, err)
	require.JSONEq(t, string(source), string(wire))
}

func TestForcedToolOverrideCannotCreateOllamaMapping(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: ordinary tools are mapped, but claimed forced selection cannot add a port.
			profile := prompty.CapabilityProfile{
				Identity: prompty.ProfileIdentity{
					Provider: "ollama",
					Endpoint: "https://fixture.invalid",
					Model:    "fixture",
				},
				Provenance: "adversarial mapping fixture",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityTools:               prompty.CapabilitySupported,
					prompty.CapabilityForcedToolSelection: prompty.CapabilitySupported,
				},
			}
			fixture := &schemaTransportFixture{
				Adapter: New(WithModel("fixture"), WithProfiles(profile.Identity.Endpoint)),
				calls:   0,
			}
			exec := prompty.SimplePrompt("fixture")
			exec.Tools = []prompty.ToolDefinition{
				{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object"}`)},
			}
			exec.ForcedTool = "lookup"
			exec.Requirements.Override = &profile
			// Act.
			prepared, err := adapter.Prepare(fixture, exec, mode)
			// Assert.
			require.Nil(t, prepared)
			require.ErrorIs(t, err, adapter.ErrUnsupportedCapability)
			require.ErrorIs(t, err, adapter.ErrCapabilityRequirement)
			require.Zero(t, fixture.calls)
			require.Len(t, exec.Messages, 1)
		})
	}
}
