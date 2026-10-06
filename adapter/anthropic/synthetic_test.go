package anthropic

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func syntheticExecution() *prompty.PromptExecution {
	exec := prompty.SimplePrompt("extract")
	exec.Requirements.OutputMode = prompty.OutputSyntheticTool
	exec.ResponseFormat = &prompty.SchemaDefinition{
		Name: "extract_record",
		Schema: prompty.JSONDocument(
			`{"type":"object","properties":{"kind":{"type":"string","enum":["ok"]},"n":{"type":"integer","minimum":9007199254740993}},"required":["kind","n"],"additionalProperties":false}`,
		),
	}
	return exec
}

func syntheticProfile(endpoint string) prompty.CapabilityProfile {
	return prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: endpoint, Model: "fixture"},
		Provenance: "local synthetic SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilitySyntheticOutput:     prompty.CapabilitySupported,
			prompty.CapabilityTools:               prompty.CapabilitySupported,
			prompty.CapabilityForcedToolSelection: prompty.CapabilitySupported,
			prompty.CapabilityStructuredOutput:    prompty.CapabilityUnsupported,
		},
	}
}

func syntheticTool(args string) string {
	return `{"type":"tool_use","id":"call","name":"extract_record","input":` + args + `}`
}

func TestSyntheticSDKCheckedResults(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, blocks, stop string
		cause              error
		reason             string
	}{
		{"valid", syntheticTool(`{"kind":"ok","n":9007199254740993}`), "tool_use", nil, ""},
		{"enum", syntheticTool(`{"kind":"bad","n":9007199254740993}`), "tool_use", prompty.ErrSchemaMismatch, ""},
		{"required", syntheticTool(`{"kind":"ok"}`), "tool_use", prompty.ErrSchemaMismatch, ""},
		{"wrong name", `{"type":"tool_use","id":"call","name":"other","input":{}}`, "tool_use", ErrSyntheticOutput, "wrong_output_name"},
		{"missing", "", "end_turn", ErrSyntheticOutput, "missing_output_call"},
		{"multiple", syntheticTool(`{}`) + "," + syntheticTool(`{}`), "tool_use", ErrSyntheticOutput, "multiple_output_calls"},
		{"nonobject", syntheticTool(`[]`), "tool_use", ErrSyntheticOutput, "invalid_arguments_object"},
		{"null", syntheticTool(`null`), "tool_use", ErrSyntheticOutput, "invalid_arguments_object"},
		{"duplicate keys", syntheticTool(`{"kind":"bad","kind":"ok","n":9007199254740993}`), "tool_use", ErrSyntheticOutput, "invalid_arguments_object"},
		{"refusal", `{"type":"text","text":"declined"}`, "refusal", prompty.ErrResponseOutcome, ""},
		{"truncated", syntheticTool(`{"kind":"ok"}`), "max_tokens", prompty.ErrResponseOutcome, ""},
		{"commentary", `{"type":"text","text":"prefix"},` + syntheticTool(`{}`), "tool_use", ErrSyntheticOutput, "unexpected_content"},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: actual SDK transport observes exact tool schema and one submit.
			var calls atomic.Int32
			requests := make(chan []byte, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				body, err := io.ReadAll(r.Body)
				if err != nil {
					http.Error(w, "read", http.StatusInternalServerError)
					return
				}
				requests <- body
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(
					w,
					`{"id":"fixture","type":"message","role":"assistant","model":"fixture","content":[%s],"stop_reason":%q,"usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
					test.blocks,
					test.stop,
				)
			}))
			defer server.Close()
			sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			adp := NewSyntheticOutput(
				WithClient(&sdk),
				WithModel("fixture"),
				WithProfiles(server.URL, syntheticProfile(server.URL)),
			)
			exec := syntheticExecution()
			original := string(exec.ResponseFormat.Schema)
			// Act.
			response, err := adapter.NewClient(adp).Execute(context.Background(), exec)
			// Assert: failures retain known usage and never publish executable output calls.
			if test.cause == nil {
				require.NoError(t, err)
				require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
			} else {
				require.ErrorIs(t, err, test.cause)
			}
			if test.reason != "" {
				var structural *SyntheticOutputError
				require.ErrorAs(t, err, &structural)
				require.Equal(t, test.reason, structural.Reason)
			}
			require.NotNil(t, response)
			require.Contains(t, response.Usage.Known, prompty.UsageTotal)
			for _, part := range response.Content {
				require.IsType(t, prompty.TextPart{}, part)
			}
			require.EqualValues(t, 1, calls.Load())
			require.Equal(t, original, string(exec.ResponseFormat.Schema))
			require.Empty(t, exec.Tools)
			body := <-requests
			require.Contains(t, string(body), `9007199254740993`)
			var decoded map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(body, &decoded))
			require.NotContains(t, decoded, "output_config")
			require.JSONEq(t, `{"type":"tool","name":"extract_record"}`, string(decoded["tool_choice"]))
		})
	}
}

func TestSyntheticPreflightConflicts(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name   string
		mutate func(*prompty.PromptExecution)
		mode   prompty.StreamMode
		native bool
	}{
		{"real tools", func(e *prompty.PromptExecution) { e.Tools = []prompty.ToolDefinition{{Name: "lookup"}} }, "", false},
		{"collision", func(e *prompty.PromptExecution) { e.Tools = []prompty.ToolDefinition{{Name: e.ResponseFormat.Name}} }, "", false},
		{"invalid name", func(e *prompty.PromptExecution) { e.ResponseFormat.Name = "invalid name" }, "", false},
		{"manual thinking", func(e *prompty.PromptExecution) {
			e.ModelOptions = &prompty.ModelOptions{MaxTokens: new(int64(2048)), ProviderSettings: prompty.JSONDocument(`{"thinking_mode":"enabled","thinking_budget":1024}`)}
		}, "", false},
		{"native selection", func(e *prompty.PromptExecution) { e.Requirements.OutputMode = prompty.OutputNative }, "", false},
		{"override cannot invent mapper", func(*prompty.PromptExecution) {}, "", true},
		{"native stream absent", func(*prompty.PromptExecution) {}, prompty.StreamNative, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			profile := syntheticProfile(server.URL)
			profile.Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			profile.ModelOptionsSchema = prompty.JSONDocument(`{"type":"object"}`)
			exec := syntheticExecution()
			test.mutate(exec)
			original := exec.Clone()
			// Act.
			var err error
			if test.native {
				_, err = adapter.Prepare(
					New(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)),
					exec,
					test.mode,
				)
			} else {
				_, err = adapter.Prepare(
					NewSyntheticOutput(WithClient(&sdk), WithModel("fixture"), WithProfiles(server.URL, profile)),
					exec,
					test.mode,
				)
			}
			// Assert.
			require.Error(t, err)
			require.Zero(t, calls.Load())
			require.Equal(t, original, exec)
		})
	}
}

func TestSyntheticUnknownAcceptanceNoRetry(t *testing.T) {
	t.Parallel()
	// Arrange: SDK default retries are explicitly disabled by this operation.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"), option.WithMaxRetries(3))
	adp := NewSyntheticOutput(
		WithClient(&sdk),
		WithModel("fixture"),
		WithProfiles(server.URL, syntheticProfile(server.URL)),
	)
	// Act.
	_, err := adapter.NewClient(adp).Execute(context.Background(), syntheticExecution())
	// Assert.
	require.Error(t, err)
	require.EqualValues(t, 1, calls.Load())
}

func TestSyntheticRequestBoundName(t *testing.T) {
	t.Parallel()
	// Arrange: one adapter can translate requests with independent output names.
	adp := NewSyntheticOutput()
	exec := syntheticExecution()
	first, err := adp.Translate(exec)
	require.NoError(t, err)
	exec.ResponseFormat.Name = "second_name"
	second, err := adp.Translate(exec)
	require.NoError(t, err)
	// Act: parse a response for each immutable request binding.
	for _, request := range []*SyntheticRequest{first, second} {
		var message anthropic.Message
		body := fmt.Sprintf(
			`{"content":[{"type":"tool_use","id":"call","name":%q,"input":{}}],"stop_reason":"tool_use"}`,
			request.outputName,
		)
		require.NoError(t, message.UnmarshalJSON([]byte(body)))
		response, parseErr := adp.ParseResponse(&SyntheticResponse{message: &message, outputName: request.outputName})
		// Assert.
		require.NoError(t, parseErr)
		require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	}
	require.Equal(t, "extract_record", first.outputName)
	require.Equal(t, "second_name", second.outputName)
}
