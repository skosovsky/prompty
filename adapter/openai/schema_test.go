package openai

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestCheckedSDKLoweringStillRejectsNullAgainstOriginalSchema(t *testing.T) {
	t.Parallel()
	// Arrange: only a permitted required-nullable lowering can dispatch.
	var calls atomic.Int32
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "fixture read failed", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fixture","model":"fixture","choices":[{"index":0,`+
			`"message":{"role":"assistant","content":"{\"x\":null}"},"finish_reason":"stop"}]}`)
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local schema SDK fixture, not live model verification",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"x":{"type":"integer","minimum":10}},"additionalProperties":false}`,
	)}
	// Act / Assert: strict refuses the optional-property semantic change before dispatch.
	_, strictErr := client.Execute(context.Background(), exec)
	require.ErrorIs(t, strictErr, prompty.ErrSchemaUnsupported)
	require.Zero(t, calls.Load())
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode:              prompty.SchemaLowering,
		AllowedTransforms: []string{LoweringRequiredNullable},
	}
	// Act.
	response, loweredErr := client.Execute(context.Background(), exec)
	// Assert: actual SDK request is lowered, but source validation still rejects null.
	require.ErrorIs(t, loweredErr, prompty.ErrSchemaMismatch)
	require.NotNil(t, response)
	require.EqualValues(t, 1, calls.Load())
	require.Len(t, response.Diagnostics.Schemas, 1)
	change := response.Diagnostics.Schemas[0].Transformations[0]
	require.Equal(t, LoweringRequiredNullable, change.ID)
	require.Equal(t, "/properties/x", change.Pointer)
	var wire map[string]any
	require.NoError(t, json.Unmarshal(<-requests, &wire))
	schema := wire["response_format"].(map[string]any)["json_schema"].(map[string]any)["schema"].(map[string]any)
	require.Equal(t, []any{"x"}, schema["required"])
	x := schema["properties"].(map[string]any)["x"].(map[string]any)["anyOf"].([]any)
	require.InDelta(t, 10, x[0].(map[string]any)["minimum"], 0)
	require.Equal(t, map[string]any{"type": "null"}, x[1])
}

func TestLoweringPointersReferToOriginalNestedSchema(t *testing.T) {
	t.Parallel()
	// Arrange: the optional wrapper must not shift diagnostics into a generated anyOf.
	request := prompty.SchemaCompileRequest{
		Source: prompty.JSONDocument(
			`{"type":"object","properties":{"a~/b":{"type":"object","properties":{"x":{"enum":["a"]}}}},"additionalProperties":false}`,
		),
		Role:    prompty.SchemaOutput,
		Profile: prompty.ProfileIdentity{Provider: "openai", Endpoint: "https://fixture.invalid", Model: "fixture"},
		Policy: prompty.SchemaPolicy{
			Mode:              prompty.SchemaLowering,
			AllowedTransforms: []string{LoweringCloseObject, LoweringRequiredNullable},
		},
		Validator: prompty.NewJSONSchemaValidator(nil),
	}
	// Act.
	compiled, err := prompty.CompileProviderSchema(request, New())
	// Assert.
	require.NoError(t, err)
	var pointers []string
	for _, change := range compiled.Report().Transformations {
		pointers = append(pointers, change.Pointer)
	}
	require.ElementsMatch(t, []string{
		"/properties/a~0~1b", "/properties/a~0~1b/additionalProperties", "/properties/a~0~1b/properties/x",
	}, pointers)
	require.ErrorIs(
		t,
		compiled.ValidateOriginal(prompty.JSONDocument(`{"a~/b":{"x":null}}`)),
		prompty.ErrSchemaMismatch,
	)
	require.NoError(t, compiled.ValidateOriginal(prompty.JSONDocument(`{}`)))
}

func TestStrictSDKSchemaSerializationKeepsNumericConstraints(t *testing.T) {
	t.Parallel()
	// Arrange.
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"x":{"type":"integer","const":9007199254740993}},"required":["x"],"additionalProperties":false}`,
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

func TestNativeSDKFinalizationValidatesOriginalAfterTrailingUsage(t *testing.T) {
	t.Parallel()
	// Arrange: complete provider JSON is permitted by lowering but not the original.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "text/event-stream")
		for _, event := range []string{
			`{"choices":[{"index":0,"delta":{"content":"{\"x\":null}"}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"total_tokens":9}}`, "[DONE]",
		} {
			_, _ = io.WriteString(w, "data: "+event+"\n\n")
		}
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local schema/native SSE SDK fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
			prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
		},
	}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
	exec := prompty.SimplePrompt("fixture")
	exec.ResponseFormat = &prompty.SchemaDefinition{Schema: prompty.JSONDocument(
		`{"type":"object","properties":{"x":{"type":"integer"}},"additionalProperties":false}`,
	)}
	exec.Requirements.Schema = prompty.SchemaPolicy{
		Mode:              prompty.SchemaLowering,
		AllowedTransforms: []string{LoweringRequiredNullable},
	}
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamNative)
	require.Zero(t, calls.Load())
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
		}
	}
	// Assert.
	status := stream.Status()
	require.Equal(t, prompty.StreamFailed, status.State)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, status.Result.Usage.Known)
	require.Equal(t, prompty.OutcomeCompleted, status.Result.Outcome)
	require.Len(t, status.Result.Diagnostics.Schemas, 1)
	require.Equal(t, 1, terminals)
	require.EqualValues(t, 1, calls.Load())
}
