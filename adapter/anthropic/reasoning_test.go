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

func TestThinkingModesMapToSDKWithoutInferringBudget(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name     string
		settings string
		want     string
	}{
		{"manual", `{"thinking_mode":"enabled","thinking_budget":1024.0,"thinking_visibility":"summarized"}`, `{"type":"enabled","budget_tokens":1024,"display":"summarized"}`},
		{"adaptive", `{"thinking_mode":"adaptive","thinking_visibility":"omitted"}`, `{"type":"adaptive","display":"omitted"}`},
		{"disabled", `{"thinking_mode":"disabled"}`, `{"type":"disabled"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			exec := prompty.SimplePrompt("fixture")
			exec.ModelOptions = &prompty.ModelOptions{
				MaxTokens:        new(int64(4096)),
				ProviderSettings: prompty.JSONDocument(tc.settings),
			}
			// Act.
			request, err := New().Translate(exec)
			require.NoError(t, err)
			payload, err := json.Marshal(request.Thinking)
			// Assert.
			require.NoError(t, err)
			require.JSONEq(t, tc.want, string(payload))
		})
	}
}

func TestThinkingInvalidModesAndBudgetsFailBeforeTransport(t *testing.T) {
	t.Parallel()
	for _, settings := range []string{
		`{"thinking_budget":1024}`, `{"thinking_mode":"enabled"}`,
		`{"thinking_mode":"enabled","thinking_budget":4096}`,
		`{"thinking_mode":"enabled","thinking_budget":1024.5}`,
		`{"thinking_mode":"adaptive","thinking_budget":1024}`,
		`{"thinking_mode":"disabled","thinking_visibility":"omitted"}`,
		`{"thinking_mode":"adaptive","thinking_visibility":"raw_hidden_reasoning"}`,
		`{"thinking_mode":{}}`, `{"reasoning_effort":"unsupported"}`,
	} {
		// Arrange.
		exec := prompty.SimplePrompt("fixture")
		exec.ModelOptions = &prompty.ModelOptions{
			MaxTokens:        new(int64(4096)),
			ProviderSettings: prompty.JSONDocument(settings),
		}
		// Act: there is no SDK client to reach.
		_, err := New().Translate(exec)
		// Assert.
		require.ErrorIs(t, err, adapter.ErrInvalidProviderSettings)
	}
}

func TestCheckedAdaptiveVisibilityAndEffortThroughSDK(t *testing.T) {
	t.Parallel()
	// Arrange: exact destination knowledge, not a live-model support claim.
	requests := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"id":"fixture","type":"message","role":"assistant","model":"fixture",`+
			`"content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1}}`)
	}))
	defer server.Close()
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK reasoning fixture", ModelOptionsSchema: prompty.JSONDocument(
			`{"type":"object","properties":{"provider_settings":{"type":"object","properties":{` +
				`"thinking_mode":{"const":"adaptive"},"thinking_visibility":{"const":"omitted"},"reasoning_effort":{"const":"high"}},` +
				`"additionalProperties":false}}}`),
	}
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{ProviderSettings: prompty.JSONDocument(
		`{"thinking_mode":"adaptive","thinking_visibility":"omitted","reasoning_effort":"high"}`)}
	client := adapter.NewClient(New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL, profile)))
	// Act.
	response, err := client.Execute(context.Background(), exec)
	// Assert.
	require.NoError(t, err)
	require.Equal(t, prompty.CapabilitySupported, response.Diagnostics.Checks[0].Status)
	var request map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.JSONEq(t, `{"type":"adaptive","display":"omitted"}`, string(request["thinking"]))
	require.JSONEq(t, `{"effort":"high"}`, string(request["output_config"]))
}
