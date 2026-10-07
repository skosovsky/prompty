package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty/adapter"
)

func TestResponseRoleBoundary(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"", "model", "user", "system"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			response := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{
					{
						Content:      &genai.Content{Role: role, Parts: []*genai.Part{{Text: "ok"}}},
						FinishReason: genai.FinishReasonStop,
					},
				},
			}
			// Act.
			parsed, err := New().ParseResponse(response)
			// Assert.
			if role == "" || role == "model" {
				require.NoError(t, err)
				require.NotNil(t, parsed)
			} else {
				require.ErrorIs(t, err, adapter.ErrInvalidResponse)
				require.Nil(t, parsed)
			}
		})
	}
}

func task45CheckedFixture(t *testing.T, body string) (prompty.Invoker, *atomic.Int32) {
	t.Helper()
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if strings.Contains(r.URL.Path, "streamGenerateContent") {
			w.Header().Set("Content-Type", "text/event-stream")
			_, _ = fmt.Fprintf(w, "data: %s\n\n", body)
		} else {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, body)
		}
	}))
	t.Cleanup(server.Close)
	sdk, err := genai.NewClient(
		t.Context(),
		&genai.ClientConfig{
			APIKey:      "fixture",
			Backend:     genai.BackendGeminiAPI,
			HTTPClient:  server.Client(),
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
		},
	)
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "actual local SDK fixture",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityNativeStream: prompty.CapabilitySupported,
			prompty.CapabilityTools:        prompty.CapabilitySupported,
		},
	}
	return adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile))), &calls
}

func TestCheckedResponseRoleModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, role := range []string{"", "model", "user", "system"} {
			t.Run(fmt.Sprintf("%s/%s", mode, role), func(t *testing.T) {
				t.Parallel()
				// Arrange: the same provider envelope goes through sync/buffered/native SDK paths.
				body := fmt.Sprintf(
					`{"candidates":[{"content":{"role":%q,"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`,
					role,
				)
				client, calls := task45CheckedFixture(t, body)
				var response *prompty.Response
				var err error
				// Act.
				if mode == "" {
					response, err = client.Execute(t.Context(), prompty.SimplePrompt("fixture"))
				} else {
					stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), mode)
					terminals := 0
					for frame := range stream.Events() {
						if frame.Kind == prompty.StreamTerminal {
							terminals++
						}
					}
					require.Equal(t, 1, terminals)
					response, err = stream.Status().Result, stream.Status().Err
					if role != "" && role != "model" {
						require.Equal(t, prompty.StreamFailed, stream.Status().State)
					}
				}
				// Assert.
				if role == "" || role == "model" {
					require.NoError(t, err)
					require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
				} else {
					require.ErrorIs(t, err, adapter.ErrInvalidResponse)
					if response != nil {
						require.NotEqual(t, prompty.OutcomeCompleted, response.Outcome)
					}
				}
				require.EqualValues(t, 1, calls.Load())
			})
		}
	}
}

func TestForeignRoleToolNeverCallsHandler(t *testing.T) {
	t.Parallel()
	for _, role := range []string{"user", "model"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			// Arrange: a requested call is admitted only as an assistant/model response.
			body := fmt.Sprintf(
				`{"candidates":[{"content":{"role":%q,"parts":[{"functionCall":{"id":"call","name":"lookup","args":{}}}]},"finishReason":"STOP"}]}`,
				role,
			)
			client, calls := task45CheckedFixture(t, body)
			handlers := 0
			tool, err := prompty.NewTypedTool(
				"lookup",
				func(context.Context, struct{}) (string, error) { handlers++; return "ok", nil },
			)
			require.NoError(t, err)
			registry := prompty.NewTypedToolRegistry()
			require.NoError(t, prompty.RegisterTool(registry, tool))
			execution := prompty.SimplePrompt("fixture")
			execution.Tools = []prompty.ToolDefinition{{
				Name:       "lookup",
				Parameters: prompty.JSONDocument(`{"type":"object","properties":{}}`),
			}}
			// Act / Assert.
			_, err = prompty.ExecuteWithTypedTools(t.Context(), client, execution, registry)
			if role == "user" {
				require.ErrorIs(t, err, adapter.ErrInvalidResponse)
				require.Zero(t, handlers)
			} else {
				require.NoError(t, err)
				require.Equal(t, 1, handlers)
			}
			require.EqualValues(t, 1, calls.Load())
		})
	}
}
