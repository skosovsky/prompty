package anthropic

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func modelMessageFixture(model string) string {
	field := `,"model":` + model
	if model == "absent" {
		field = ""
	}
	return `{"id":"msg_fixture","type":"message","role":"assistant"` + field + `,"content":[{"type":"text","text":"known"}],"stop_reason":"end_turn","usage":{"input_tokens":3,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`
}

func TestObservedModelSourceActualSDK(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, model, want string
		malformed         bool
	}{
		{"number", "42", "", true},
		{"boolean", "true", "", true},
		{"object", `{"id":"model"}`, "", true},
		{"array", `["model"]`, "", true},
		{"duplicate", `"lost","model":"kept"`, "", true},
		{"null", "null", "", false},
		{"absent", "absent", "", false},
		{"empty string", `""`, "", false},
		{"source string", `"observed"`, "observed", false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: checked HTTP execution, with selected and observed model separated.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, modelMessageFixture(test.model))
			}))
			t.Cleanup(server.Close)
			client := sdk.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
				Provenance: "actual SDK fixture, not live conformance",
			}
			invoker := adapter.NewClient(
				New(WithModel("fixture"), WithClient(&client), WithProfiles(server.URL, profile)),
			)
			// Act.
			response, err := invoker.Execute(t.Context(), prompty.SimplePrompt("fixture"))
			// Assert: malformed model metadata cannot fabricate a trusted observed string.
			if test.malformed {
				require.ErrorIs(t, err, adapter.ErrInvalidResponse)
			} else {
				require.NoError(t, err)
			}
			require.NotNil(t, response)
			require.Equal(t, test.want, response.Model)
			require.Equal(t, test.malformed, response.ContinuationUnavailable)
			require.Equal(t, "known", response.Content[0].(prompty.TextPart).Text)
			require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
			require.Equal(t, "end_turn", response.FinishReason)
			require.Equal(t, 5, response.Usage.TotalTokens)
			require.Equal(t, "fixture", response.Diagnostics.Profile.Model)
		})
	}
}

func TestObservedModelRawFreeTypedSDK(t *testing.T) {
	t.Parallel()
	// Arrange: a caller-owned typed SDK message has no coercible raw source.
	message := &sdk.Message{
		Model:      "typed-observed",
		Content:    []sdk.ContentBlockUnion{{Type: "text", Text: "known"}},
		StopReason: sdk.StopReasonEndTurn,
	}
	// Act.
	response, err := New().ParseResponse(message)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, "typed-observed", response.Model)
	require.False(t, response.ContinuationUnavailable)
}

func TestObservedModelSourceActualSDKBatch(t *testing.T) {
	t.Parallel()
	// Arrange: selected model42 exercises the equality gate that SDK coercion can pass.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/results") {
			_, _ = fmt.Fprintf(
				w,
				`{"custom_id":"one","result":{"type":"succeeded","message":%s}}`+"\n",
				modelMessageFixture("42"),
			)
			return
		}
		writeBatchFixture(w, "ended")
	}))
	t.Cleanup(server.Close)
	manager := batchFixtureManager(server)
	manager.base.defaultModel = "42"
	manager.base.profiles[0].Identity.Model = "42"
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{Model: "42"}
	input := prompty.BatchSubmitRequest{Items: []prompty.BatchItem{{ID: "one", AttemptID: "first", Execution: exec}}}
	submitted, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	// Act.
	result, err := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
	// Assert: the shared parser retains non-model evidence and reports item failure.
	require.NoError(t, err)
	require.Len(t, result.Items, 1)
	require.Equal(t, prompty.BatchItemIncomplete, result.Items[0].Outcome)
	require.Equal(t, "validation_failed", result.Items[0].ErrorKind)
	require.NotNil(t, result.Items[0].Response)
	require.Empty(t, result.Items[0].Response.Model)
	require.True(t, result.Items[0].Response.ContinuationUnavailable)
	require.Equal(t, "known", result.Items[0].Response.Content[0].(prompty.TextPart).Text)
	require.Equal(t, 5, result.Items[0].Response.Usage.TotalTokens)
}

func TestObservedModelNativeSourceGuardActualSDK(t *testing.T) {
	t.Parallel()
	for _, model := range []string{"42", "true", "null", `{}`, `[]`} {
		t.Run(model, func(t *testing.T) {
			t.Parallel()
			// Arrange: native message_start already requires a genuine model string.
			start := strings.Replace(messageStartFixture, `"model":"fixture"`, `"model":`+model, 1)
			invoker, _ := nativeFixture(t, []string{start, `{"type":"message_stop"}`})
			// Act.
			stream := invoker.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			for range stream.Events() {
			}
			// Assert.
			require.ErrorIs(t, stream.Status().Err, adapter.ErrInvalidResponse)
			require.Empty(t, stream.Status().Result.Model)
		})
	}
}
