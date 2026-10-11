package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestBatchNativeTruncationUnknownUsageAndUnmappedItemCancellation(t *testing.T) {
	t.Parallel()
	for _, fileMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "inline", true: "file"}[fileMode], func(t *testing.T) {
			t.Parallel()
			// Arrange: native MAX_TOKENS, absent usage, and RPC error with cancellation status.
			truncated := `{"response":{"candidates":[{"content":{"parts":[{"text":"prefix"}]},"finishReason":"MAX_TOKENS"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3}}}`
			unknownUsage := `{"response":{"candidates":[{"content":{"parts":[{"text":"label"}]},"finishReason":"STOP"}]}}`
			unmappedCancel := `{"error":{"code":1,"status":"CANCELLED","message":"provider detail"}}`
			inline := make([]string, 0, 3)
			lines := make([]string, 0, 3)
			for index, value := range []string{unmappedCancel, unknownUsage, truncated} {
				key := []string{"three", "two", "one"}[index]
				inline = append(inline, `{"metadata":{"key":"`+key+`"},`+value[1:])
				lines = append(lines, `{"key":"`+key+`",`+value[1:])
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if request.Method == http.MethodPost {
					_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
					return
				}
				if fileMode {
					_, _ = io.WriteString(
						w,
						`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/output"}}}`,
					)
					return
				}
				_, _ = io.WriteString(
					w,
					`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"inlinedResponses":{"inlinedResponses":[`+strings.Join(
						inline,
						",",
					)+`]}}}}`,
				)
			}))
			defer server.Close()
			base, _ := batchOperationFixture(t, server)
			manager := NewBatches("account-fixture", WithClient(base.client), WithModel("fixture"),
				WithProfiles(server.URL, base.profiles...))
			if fileMode {
				files := &batchFixtureFiles{
					binding: prompty.BatchFileBinding{
						Provider:       "gemini",
						Endpoint:       server.URL,
						AccountBinding: "account-fixture",
					},
					output: strings.Join(lines, "\n") + "\n", status: http.StatusOK,
				}
				manager = NewFileBatches("account-fixture", files, BatchFileOptions{}, WithClient(base.client),
					WithModel("fixture"), WithProfiles(server.URL, base.profiles...))
			}
			input, options := inlineBatchFixtureInputs()
			created, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			observed, err := manager.Get(t.Context(), *created.Handle)
			require.NoError(t, err)
			// Act.
			result, err := manager.Results(t.Context(), *observed.Handle, options)
			// Assert: generic RPC errors are not invented native per-item lifecycle enums.
			require.NoError(t, err)
			require.True(t, result.Completeness.DownloadComplete)
			require.True(t, result.Completeness.CorrelationComplete)
			require.Len(t, result.Items, 3)
			require.Equal(t, prompty.BatchItemError, result.Items[0].Outcome)
			if fileMode {
				require.Contains(t, string(result.Items[0].SourceJSON), `"status":"CANCELLED"`)
			} else {
				require.NotContains(t, string(result.Items[0].SourceJSON), `"status":"CANCELLED"`)
			}
			require.Nil(t, result.Items[0].Response)
			require.Equal(t, prompty.BatchItemSuccess, result.Items[1].Outcome)
			require.Equal(t, prompty.UsageUnknown, result.Items[1].Response.Usage.Availability())
			require.Empty(t, result.Items[1].Response.Usage.Known)
			require.Equal(t, prompty.BatchItemIncomplete, result.Items[2].Outcome)
			require.Equal(t, prompty.OutcomeIncomplete, result.Items[2].Response.Outcome)
			require.Equal(t, "MAX_TOKENS", result.Items[2].Response.FinishReason)
			require.Equal(t, 3, result.Items[2].Response.Usage.TotalTokens)
		})
	}
}
