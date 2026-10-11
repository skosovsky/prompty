package openai

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestRefusalAndIncompletePreserveEmptyResults(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name    string
		message string
		reason  string
		outcome prompty.Outcome
		parts   int
	}{
		{"refusal", `{"refusal":"denied"}`, "stop", prompty.OutcomeRefusal, 1},
		{"empty refusal", `{"refusal":""}`, "stop", prompty.OutcomeRefusal, 1},
		{"null refusal", `{"content":"ok","refusal":null}`, "stop", prompty.OutcomeCompleted, 1},
		{"truncated", `{}`, "length", prompty.OutcomeIncomplete, 0},
		{"filtered", `{}`, "content_filter", prompty.OutcomeIncomplete, 0},
		{"unknown reason", `{"content":"ok"}`, "future_reason", prompty.OutcomeIncomplete, 1},
		{"missing reason", `{"content":"ok"}`, "", prompty.OutcomeIncomplete, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var native openai.ChatCompletion
			data := fmt.Sprintf(`{"choices":[{"index":0,"message":%s,"finish_reason":%q}],`+
				`"usage":{"total_tokens":0}}`, test.message, test.reason)
			require.NoError(t, native.UnmarshalJSON([]byte(data)))
			// Act.
			got, err := New().ParseResponse(&native)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, test.outcome, got.Outcome)
			require.Len(t, got.Content, test.parts)
			require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, got.Usage.Known)
			if test.outcome == prompty.OutcomeRefusal {
				require.Len(t, got.Annotations, 1)
				require.Equal(t, "openai.refusal", got.Annotations[0].Kind)
			}
		})
	}
}

func TestCitationRawJSONSurvivesResponseWire(t *testing.T) {
	t.Parallel()
	// Arrange: preserve provider extension fields rather than re-encode only SDK fields.
	const citation = `{"type":"url_citation","url_citation":{"start_index":0,"end_index":4,` +
		`"url":"https://example.org/source","title":"Source","future_field":true}}`
	var native openai.ChatCompletion
	data := `{"choices":[{"index":0,"finish_reason":"stop","message":{"content":"text",` +
		`"refusal":"denied","annotations":[` + citation + `]}}]}`
	require.NoError(t, native.UnmarshalJSON([]byte(data)))
	// Act.
	response, err := New().ParseResponse(&native)
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalResponse(wire)
	// Assert.
	require.NoError(t, err)
	require.Len(t, restored.Content, 2)
	require.Equal(t, prompty.OutcomeRefusal, restored.Outcome)
	require.Len(t, restored.Annotations, 2)
	require.Equal(t, 0, restored.Annotations[0].PartIndex)
	require.Equal(t, 1, restored.Annotations[1].PartIndex)
	require.JSONEq(t, citation, string(restored.Annotations[0].Value))
	restored.Annotations[0].Value[0] = 'x'
	require.JSONEq(t, citation, string(response.Annotations[0].Value))
}

func TestParseRejectsLossyChoiceOrAnnotationAssociation(t *testing.T) {
	t.Parallel()
	for _, data := range []string{
		`{"choices":[{"index":1,"message":{"content":"text"}}]}`,
		`{"choices":[{"index":0,"message":{"content":"a"}},{"index":1,"message":{"content":"b"}}]}`,
		`{"choices":[{"index":0,"message":{"annotations":[{"type":"url_citation"}]}}]}`,
	} {
		// Arrange.
		var native openai.ChatCompletion
		require.NoError(t, native.UnmarshalJSON([]byte(data)))
		// Act.
		_, err := New().ParseResponse(&native)
		// Assert.
		require.ErrorIs(t, err, adapter.ErrInvalidResponse)
	}
}

func TestToolCallFinishWithoutCallsIsNotAnEmptySuccess(t *testing.T) {
	t.Parallel()
	// Arrange.
	var native openai.ChatCompletion
	require.NoError(t, native.UnmarshalJSON([]byte(
		`{"choices":[{"index":0,"finish_reason":"tool_calls","message":{}}]}`,
	)))
	// Act.
	_, err := New().ParseResponse(&native)
	// Assert.
	require.ErrorIs(t, err, adapter.ErrEmptyResponse)
}

func TestNativeRefusalPreservesPartsAndTrailingUsage(t *testing.T) {
	t.Parallel()
	// Arrange: refusal starts before the finish event; null is not another refusal.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, data := range []string{
			`{"choices":[{"index":0,"delta":{"content":"prefix","refusal":"no"}}]}`,
			`{"choices":[{"index":0,"delta":{"refusal":" thanks"}}]}`,
			`{"choices":[{"index":0,"delta":{"refusal":null},"finish_reason":"stop"}]}`,
			`{"choices":[],"usage":{"total_tokens":9}}`,
			"[DONE]",
		} {
			_, _ = fmt.Fprintf(w, "data: %s\n\n", data)
		}
	}))
	defer server.Close()
	sdk := openai.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	native := New(WithClient(&sdk))
	request := &openai.ChatCompletionNewParams{Model: openai.ChatModelGPT4o}
	source := func(ctx context.Context) iter.Seq2[*prompty.ResponseChunk, error] {
		return native.StreamEvents(ctx, request)
	}
	stream := prompty.NewStream(context.Background(), prompty.StreamNative, source)
	// Act.
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	result := stream.Status()
	// Assert.
	require.Equal(t, prompty.StreamCompleted, result.State)
	require.Equal(t, prompty.OutcomeRefusal, result.Result.Outcome)
	require.Equal(t, 9, result.Result.Usage.TotalTokens)
	require.Len(t, result.Result.Content, 3)
	require.Len(t, result.Result.Annotations, 2)
	require.Equal(t, 1, result.Result.Annotations[0].PartIndex)
	require.Equal(t, 2, result.Result.Annotations[1].PartIndex)
}
