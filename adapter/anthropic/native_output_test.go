package anthropic

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestNativeOutputProfileWithToolsThroughSDK(t *testing.T) {
	t.Parallel()
	// Arrange: caller-owned profile applies only to this local transport fixture.
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failed", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"fixture","type":"message","role":"assistant","model":"fixture",`+
				`"content":[{"type":"text","text":"{\"value\":1}"}],"stop_reason":"end_turn",`+
				`"usage":{"input_tokens":1,"output_tokens":1}}`,
		)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity: prompty.ProfileIdentity{
			Provider: "anthropic",
			Endpoint: server.URL,
			Model:    "fixture",
		},
		Provenance: "local native-output SDK conformance fixture, not live model verification",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
			prompty.CapabilityTools:            prompty.CapabilitySupported,
		},
	}
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = []prompty.ToolDefinition{
		{Name: "output_format", Parameters: prompty.JSONDocument(`{"type":"object"}`)},
	}
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(
			`{"type":"object","properties":{"value":{"type":"integer"}},"required":["value"],"additionalProperties":false}`,
		),
	}
	client := adapter.NewClient(
		New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)),
	)
	// Act.
	result, err := client.Execute(context.Background(), exec)
	// Assert: output schema does not inject or replace the user's tool.
	require.NoError(t, err)
	require.Equal(t, profile.Identity, result.Diagnostics.Profile)
	var request map[string]any
	require.NoError(t, json.Unmarshal(<-requests, &request))
	tools := request["tools"].([]any)
	require.Len(t, tools, 1)
	require.Equal(t, "output_format", tools[0].(map[string]any)["name"])
	require.NotContains(t, request, "tool_choice")
	format := request["output_config"].(map[string]any)["format"].(map[string]any)
	require.Equal(t, "json_schema", format["type"])
	schemaBytes, marshalErr := json.Marshal(format["schema"])
	require.NoError(t, marshalErr)
	require.JSONEq(t, string(exec.ResponseFormat.Schema), string(schemaBytes))
}

func TestNativeOutputPreservesForcedToolAndSourceSchema(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := prompty.SimplePrompt("fixture")
	exec.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
	exec.ForcedTool = "lookup"
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Schema: prompty.JSONDocument(`{"type":"object","properties":{"x":{"type":"string"}}}`),
	}
	original := string(exec.ResponseFormat.Schema)
	// Act.
	request, err := New().Translate(exec)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "lookup", request.ToolChoice.OfTool.Name)
	request.OutputConfig.Format.Schema["properties"].(map[string]any)["x"] = false
	require.Equal(t, original, string(exec.ResponseFormat.Schema))
}

func TestNativeSchemaSDKSerializationPreservesLargeInteger(t *testing.T) {
	t.Parallel()
	// Arrange: constraint exceeds float64's exact integer range.
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"x":{"type":"integer","const":9007199254740993}}}`,
	)}
	// Act.
	request, err := New().Translate(exec)
	require.NoError(t, err)
	wire, err := json.Marshal(request)
	// Assert.
	require.NoError(t, err)
	require.Contains(t, string(wire), `"const":9007199254740993`)
	require.NotContains(t, string(wire), `9007199254740992`)
}
