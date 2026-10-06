package anthropic

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestSyntheticBufferedOriginalSchema(t *testing.T) {
	t.Parallel()
	for _, valid := range []bool{true, false} {
		t.Run(strconv.FormatBool(valid), func(t *testing.T) {
			t.Parallel()
			// Arrange: completed transport does not bypass original schema acceptance.
			args := `{"kind":"ok","n":9007199254740993}`
			if !valid {
				args = `{"kind":"ok"}`
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(w, `{"id":"fixture","type":"message","role":"assistant","model":"fixture",`+
					`"content":[%s],"stop_reason":"tool_use","usage":{"output_tokens":0}}`, syntheticTool(args))
			}))
			defer server.Close()
			sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
			adp := NewSyntheticOutput(
				WithClient(&sdk),
				WithModel("fixture"),
				WithProfiles(server.URL, syntheticProfile(server.URL)),
			)
			// Act.
			stream := adapter.NewClient(adp).
				ExecuteStream(context.Background(), syntheticExecution(), prompty.StreamBuffered)
			defer stream.Close()
			terminals := 0
			for event, err := range stream.Events() {
				if valid {
					require.NoError(t, err)
				}
				if event != nil && event.Kind == prompty.StreamTerminal {
					terminals++
				}
			}
			// Assert: exactly one terminal, known partial JSON retained on mismatch.
			require.Equal(t, 1, terminals)
			status := stream.Status()
			require.NotNil(t, status.Result)
			require.Equal(t, args, status.Result.Content[0].(prompty.TextPart).Text)
			if valid {
				require.Equal(t, prompty.StreamCompleted, status.State)
			} else {
				require.Equal(t, prompty.StreamFailed, status.State)
				require.ErrorIs(t, status.Err, prompty.ErrSchemaMismatch)
			}
		})
	}
}

func TestNativeTranslatorRejectsSyntheticSelection(t *testing.T) {
	t.Parallel()
	// Arrange: raw translation cannot silently ignore output selection.
	exec := syntheticExecution()
	// Act.
	_, err := New().Translate(exec)
	// Assert.
	require.ErrorIs(t, err, adapter.ErrStructuredOutputNotSupported)
	_, err = NewSyntheticOutput().Translate(nil)
	require.ErrorIs(t, err, adapter.ErrNilExecution)
}
