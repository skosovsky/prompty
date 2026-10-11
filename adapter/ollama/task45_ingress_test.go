package ollama

import (
	"context"
	"fmt"
	"testing"

	"github.com/skosovsky/prompty"

	"github.com/ollama/ollama/api"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/adapter"
)

func TestRawStreamNilInputsAndEnvelopeParity(t *testing.T) {
	t.Parallel()
	a := New()
	// Arrange / Act / Assert: raw invalid inputs produce typed errors, never panics.
	for _, value := range []any{nil, (*api.ChatResponse)(nil), "wrong"} {
		require.NotPanics(
			t,
			func() { _, err := a.ParseStreamChunk(value); require.ErrorIs(t, err, adapter.ErrInvalidResponse) },
		)
	}
	for _, role := range []string{"", "assistant", "user", "system"} {
		// Arrange.
		response := &api.ChatResponse{Message: api.Message{Role: role, Content: "ok"}, Done: true, DoneReason: "stop"}
		// Act.
		parsed, err := a.ParseResponse(response)
		_, chunkErr := a.ParseStreamChunk(response)
		// Assert.
		if role == "" || role == "assistant" {
			require.NoError(t, err)
			require.NotNil(t, parsed)
			require.NoError(t, chunkErr)
		} else {
			require.ErrorIs(t, err, adapter.ErrInvalidResponse)
			require.Nil(t, parsed)
			require.ErrorIs(t, chunkErr, adapter.ErrInvalidResponse)
		}
	}
	response := &api.ChatResponse{
		Message:    api.Message{Role: "assistant", Content: "caption", Images: []api.ImageData{{1, 2, 3}}},
		Done:       true,
		DoneReason: "stop",
	}
	_, err := a.ParseResponse(response)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	_, err = a.ParseStreamChunk(response)
	require.ErrorIs(t, err, adapter.ErrUnsupportedContentType)
	_, err = a.ParseStreamChunk(&api.ChatResponse{})
	require.NoError(t, err)
}

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestCheckedRoleAndImageModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		for _, test := range []struct {
			name, body string
			valid      bool
			sentinel   error
		}{
			{"assistant", `{"message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop"}`, true, nil},
			{"omitted", `{"message":{"content":"ok"},"done":true,"done_reason":"stop"}`, true, nil},
			{"user", `{"message":{"role":"user","content":"bad"},"done":true,"done_reason":"stop"}`, false, adapter.ErrInvalidResponse},
			{"system", `{"message":{"role":"system","content":"bad"},"done":true,"done_reason":"stop"}`, false, adapter.ErrInvalidResponse},
			{"images", `{"message":{"role":"assistant","content":"caption","images":["AQID"]},"done":true,"done_reason":"stop"}`, false, adapter.ErrUnsupportedContentType},
		} {
			t.Run(fmt.Sprintf("%s/%s", mode, test.name), func(t *testing.T) {
				t.Parallel()
				// Arrange: real SDK HTTP decoding, one identical envelope in each mode.
				client, calls := nativeFixture(t, []string{test.body})
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
					if !test.valid {
						require.Equal(t, prompty.StreamFailed, stream.Status().State)
					}
				}
				// Assert: a rejected role/image cannot become a completed assistant result.
				if test.valid {
					require.NoError(t, err)
					require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
				} else {
					require.ErrorIs(t, err, test.sentinel)
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
	for _, role := range []string{"user", "assistant"} {
		t.Run(role, func(t *testing.T) {
			t.Parallel()
			// Arrange: the same well-formed, requested tool call changes only its response role.
			body := fmt.Sprintf(
				`{"message":{"role":%q,"tool_calls":[{"id":"call","function":{"name":"lookup","arguments":{}}}]},"done":true,"done_reason":"stop"}`,
				role,
			)
			client, calls := nativeFixture(t, []string{body}, prompty.CapabilityTools)
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
			// Act.
			_, err = prompty.ExecuteWithTypedTools(t.Context(), client, execution, registry)
			// Assert: a foreign role never calls a handler, while the admitted control does.
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
