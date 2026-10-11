package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func nativeSchemaFixture(t *testing.T, answer string) (*Adapter, *atomic.Int64, <-chan string) {
	t.Helper()
	calls := &atomic.Int64{}
	bodies := make(chan string, 8)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
		text, _ := json.Marshal(answer)
		_, _ = io.WriteString(
			w,
			`{"candidates":[{"content":{"parts":[{"text":`+string(text)+`}]},"finishReason":"STOP"}]}`,
		)
	}))
	t.Cleanup(server.Close)
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "offline native schema wire fixture; not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
			prompty.CapabilityTools:            prompty.CapabilitySupported,
		},
	}
	return New(
		WithNativeJSONSchema(),
		WithModel("fixture"),
		WithProfiles(server.URL, profile),
		WithClient(sdk),
	), calls, bodies
}

func TestSDKNativeSchemaNumericSerializationCharacterization(t *testing.T) {
	t.Parallel()
	for _, tool := range []bool{false, true} {
		t.Run(map[bool]string{false: "output", true: "tool"}[tool], func(t *testing.T) {
			t.Parallel()
			// Arrange: use the public adapter and actual SDK HTTP serialization.
			adp, calls, bodies := nativeSchemaFixture(t, `{"value":9007199254740993,"nested":[{"decimal":1.1}]}`)
			schema := prompty.JSONDocument(
				`{"type":"object","properties":{"value":{"type":"integer","minimum":9007199254740993,"maximum":9007199254740993},"nested":{"type":"array","items":{"type":"object","additionalProperties":false,"properties":{"decimal":{"type":"number","maximum":1.234567890123456789}}}}},"additionalProperties":false,"required":["value"]}`,
			)
			exec := prompty.SimplePrompt("fixture")
			if tool {
				exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: schema}}
			} else {
				exec.ResponseFormat = &prompty.SchemaDefinition{Schema: schema}
			}
			// Act.
			_, err := adapter.NewClient(adp).Execute(context.Background(), exec)
			// Assert: verify the bytes that reached HTTP, not just the SDK DTO.
			require.NoError(t, err)
			require.EqualValues(t, 1, calls.Load())
			body := <-bodies
			require.Contains(t, body, "9007199254740992")
			require.Contains(t, body, "1.2345678901234567")
			require.Contains(t, body, `"additionalProperties":false`)
			require.NotContains(t, body, `"responseSchema"`)
			require.NotContains(t, body, `"parameters"`)
			field := "responseJsonSchema"
			if tool {
				field = "parametersJsonSchema"
			}
			require.Contains(t, body, `"`+field+`"`)
		})
	}
}

func TestCheckedJSONNativeSchemaRejectsUnmappedSemanticsBeforeDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct{ schema, pointer string }{
		{`{"type":"string","pattern":"secret-pattern"}`, "/pattern"},
		{`{"oneOf":[{"type":"number"},{"type":"integer"}]}`, "/oneOf"},
		{`{"type":"boolean","enum":[true]}`, "/enum/0"},
		{`{"$ref":"https://foreign.invalid/schema"}`, "/$ref"},
		{`{"$ref":"#/missing"}`, "/$ref"},
		{`{"$ref":"#"}`, "/$ref"},
		{`{"properties":{"a~/b":{"minimum":"secret"}}}`, "/properties/a~0~1b/minimum"},
	}
	for _, tt := range tests {
		t.Run(tt.pointer+tt.schema, func(t *testing.T) {
			t.Parallel()
			// Arrange: exercise admission independently of original-schema compilation.
			adp, calls, _ := nativeSchemaFixture(t, `{}`)
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(tt.schema)}
			// Act: direct translation shares native keyword admission with checked mapping.
			_, err := adp.Translate(exec)
			// Assert.
			var fidelity *prompty.SchemaFidelityError
			require.ErrorAs(t, err, &fidelity)
			require.Equal(t, tt.pointer, fidelity.Pointer)
			require.Zero(t, calls.Load())
			require.NotContains(t, err.Error(), "secret")
			_, err = adapter.NewClient(adp).Execute(context.Background(), exec)
			require.Error(t, err)
			require.Zero(t, calls.Load())
			require.NotContains(t, err.Error(), "secret")
			if !strings.Contains(tt.schema, "$ref") && !strings.Contains(tt.schema, `"minimum":"`) {
				require.ErrorAs(t, err, &fidelity)
				require.Equal(t, tt.pointer, fidelity.Pointer)
				require.Zero(t, calls.Load())
			}
		})
	}
}

func TestNativeSchemaOriginalValidation(t *testing.T) {
	t.Parallel()
	for _, answer := range []string{`{"value":0}`, `{"value":2,"extra":true}`} {
		// Arrange.
		adp, calls, _ := nativeSchemaFixture(t, answer)
		exec := prompty.SimplePrompt("fixture")
		exec.ResponseFormat = &prompty.SchemaDefinition{
			Schema: prompty.JSONDocument(
				`{"type":"object","properties":{"value":{"type":"integer","minimum":1}},"additionalProperties":false}`,
			),
		}
		// Act.
		result, err := adapter.NewClient(adp).Execute(context.Background(), exec)
		// Assert: native dispatch does not replace original-schema validation.
		require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
		require.NotNil(t, result)
		require.EqualValues(t, 1, calls.Load())
	}
}

func TestNativeSchemaForeignBinding(t *testing.T) {
	t.Parallel()
	// Arrange.
	adp, calls, _ := nativeSchemaFixture(t, `{}`)
	for _, identity := range []prompty.ProfileIdentity{
		{Provider: "other", Endpoint: adp.RuntimeDescriptor().Endpoint, Model: "fixture"},
		{Provider: "gemini", Endpoint: "https://foreign.invalid", Model: "fixture"},
		{Provider: "gemini", Endpoint: adp.RuntimeDescriptor().Endpoint, Model: ""},
	} {
		// Act.
		_, err := adp.MapSchema(
			prompty.SchemaMappingRequest{Profile: identity, Source: prompty.JSONDocument(`{"type":"object"}`)},
		)
		// Assert.
		var fidelity *prompty.SchemaFidelityError
		require.ErrorAs(t, err, &fidelity)
		require.Zero(t, calls.Load())
	}
}

func TestSDKClientExtrasHookCopyCharacterization(t *testing.T) {
	t.Parallel()
	// SDK-GENAI-HOOK: ClientConfig/NewClient public copy drops function-valued hooks.
	// Arrange: configure only the public client-level hook.
	adp, calls, bodies := nativeSchemaFixture(t, `{"value":2}`)
	var hookCalls atomic.Int64
	clientConfig := adp.client.ClientConfig()
	clientConfig.HTTPOptions.ExtrasRequestProvider = func(body map[string]any) map[string]any {
		hookCalls.Add(1)
		body["caller_marker"] = "preserved"
		return body
	}
	sdk, err := genai.NewClient(context.Background(), &clientConfig)
	require.NoError(t, err)
	WithClient(sdk)(adp)
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(
			`{"type":"object","$defs":{"value":{"anyOf":[{"type":"integer","minimum":1,"maximum":9007199254740993},{"type":"number","maximum":1.234567890123456789}]}},"properties":{"value":{"$ref":"#/$defs/value"}}}`,
		),
	}
	// Act.
	_, err = adapter.NewClient(adp).Execute(context.Background(), exec)
	// Assert.
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	require.Zero(t, hookCalls.Load())
	body := <-bodies
	require.NotContains(t, body, `"caller_marker":"preserved"`)
	require.Contains(t, body, "9007199254740992")
	require.Contains(t, body, "1.2345678901234567")
}

func TestNativeSchemaUnknownModelAndForeignOverride(t *testing.T) {
	t.Parallel()
	for _, foreign := range []bool{false, true} {
		// Arrange.
		adp, calls, _ := nativeSchemaFixture(t, `{}`)
		exec := prompty.SimplePrompt("fixture")
		exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(`{"type":"object"}`)}
		if foreign {
			profile := adp.RuntimeDescriptor().Profiles[0]
			profile.Identity.Endpoint = "https://foreign.invalid"
			exec.Requirements.Override = &profile
		} else {
			exec.ModelOptions = &prompty.ModelOptions{Model: "unknown-model"}
		}
		// Act.
		_, err := adapter.NewClient(adp).Execute(context.Background(), exec)
		// Assert.
		require.ErrorIs(t, err, adapter.ErrCapabilityRequirement)
		require.Zero(t, calls.Load())
	}
}
