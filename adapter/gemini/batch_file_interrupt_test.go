package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/operationtest"
)

type interruptedBatchFiles struct {
	*batchFixtureFiles

	body *operationtest.Body
}

func (f *interruptedBatchFiles) Download(
	context.Context,
	prompty.BatchResourceRef,
) (*prompty.BatchFileDownload, error) {
	f.downloads++
	return &prompty.BatchFileDownload{Body: f.body, HTTPStatus: http.StatusOK}, nil
}

func TestFileBatchActiveInterruptionRetainsPrefixAndRemoteStatus(t *testing.T) {
	t.Parallel()
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			t.Parallel()
			// Arrange: persisted scoped results resource; reading never polls the job.
			server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				t.Error("unexpected job/generation request")
			}))
			defer server.Close()
			base, handle := batchOperationFixture(t, server)
			input, options := inlineBatchFixtureInputs()
			handle.Status, handle.ProviderStatus = prompty.BatchEnded, "JOB_STATE_SUCCEEDED"
			handle.Resources = []prompty.BatchResourceRef{{Kind: "output", ProviderID: "files/output"}}
			handle.Items = nil
			for _, item := range input.Items {
				original := item.Execution.Clone()
				original.ModelOptions = &prompty.ModelOptions{Model: "fixture"}
				digest, err := prompty.BatchExecutionDigest(original)
				require.NoError(t, err)
				handle.Items = append(handle.Items, prompty.BatchItemBinding{
					ID: item.ID, OriginalID: item.ID, AttemptID: item.AttemptID,
					Identity: base.profiles[0].Identity, ExecutionDigest: digest,
				})
			}
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			line := `{"key":"one","response":{"candidates":[{"content":{"parts":[{"text":"label"}]},"finishReason":"STOP"}]}}`
			body := operationtest.NewBody(ctx, line+"\n"+`{"key":"two",`)
			files := &interruptedBatchFiles{batchFixtureFiles: &batchFixtureFiles{
				binding: prompty.BatchFileBinding{
					Provider:       "gemini",
					Endpoint:       server.URL,
					AccountBinding: "account-fixture",
				},
			}, body: body}
			manager := NewFileBatches("account-fixture", files, BatchFileOptions{}, WithClient(base.client),
				WithModel("fixture"), WithProfiles(server.URL, base.profiles...))
			type observed struct {
				result *prompty.BatchResult
				err    error
			}
			done := make(chan observed, 1)
			// Act.
			go func() {
				result, err := manager.Results(ctx, handle, options)
				done <- observed{result, err}
			}()
			select {
			case <-body.Blocked():
			case <-ctx.Done():
				t.Fatal("fixture did not reach active file read")
			}
			wantContext := context.DeadlineExceeded
			if !deadline {
				wantContext = context.Canceled
				cancel()
			}
			got := <-done
			// Assert: cancellation of download is not remote job cancellation.
			require.ErrorIs(t, got.err, prompty.ErrBatchResults)
			require.ErrorIs(t, got.err, wantContext)
			require.Equal(t, handle, *got.result.Handle)
			require.Len(t, got.result.Items, 1)
			require.Equal(t, "one", got.result.Items[0].ID)
			require.Equal(t, prompty.BatchItemSuccess, got.result.Items[0].Outcome)
			require.JSONEq(t, line, string(got.result.Items[0].SourceJSON))
			require.Equal(t, prompty.UsageUnknown, got.result.Items[0].Response.Usage.Availability())
			require.False(t, got.result.Completeness.DownloadComplete)
			require.False(t, got.result.Completeness.CorrelationComplete)
			require.ElementsMatch(t, []string{"two", "three"}, got.result.Completeness.MissingIDs)
			require.Equal(t, 0, files.uploads)
			require.Equal(t, 1, files.downloads)
			require.EqualValues(t, 1, body.Closes())
		})
	}
}
