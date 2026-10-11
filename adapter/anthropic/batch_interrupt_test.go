package anthropic

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/operationtest"
)

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestMessageBatchesActiveInterruptionAndUnknownCancel(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"results", "cancel"} {
		for _, deadline := range []bool{false, true} {
			t.Run(operation+"/"+map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
				t.Parallel()
				// Arrange: initial submission is known; interrupt a separate observation.
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					status := "ended"
					if operation == "cancel" {
						status = "in_progress"
					}
					writeBatchFixture(w, status)
				}))
				defer server.Close()
				manager, input := batchFixtureManager(server), batchFixtureInput()
				submitted, err := manager.Submit(t.Context(), input)
				require.NoError(t, err)
				handle := prompty.CloneBatchHandle(*submitted.Handle)
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				prefix := batchSuccessLine + "\n" + `{"custom_id":"two",`
				if operation == "cancel" {
					prefix = `{"id":"msgbatch_fixture","processing_status":`
				}
				body := operationtest.NewBody(ctx, prefix)
				var calls atomic.Int32
				httpClient := &http.Client{
					Transport: operationtest.RoundTripFunc(func(request *http.Request) (*http.Response, error) {
						calls.Add(1)
						return &http.Response{
							StatusCode: http.StatusOK,
							Header:     http.Header{"Content-Type": {"application/json"}},
							Body:       body,
							Request:    request,
						}, nil
					}),
				}
				client := sdk.NewClient(option.WithAPIKey("fixture"), option.WithBaseURL(server.URL),
					option.WithHTTPClient(httpClient))
				manager.base.client = &client
				type observed struct {
					result *prompty.BatchResult
					err    error
				}
				done := make(chan observed, 1)
				// Act.
				go func() {
					var result *prompty.BatchResult
					var callErr error
					if operation == "cancel" {
						result, callErr = manager.Cancel(ctx, handle)
					} else {
						result, callErr = manager.Results(ctx, handle, batchFixtureBindings(input))
					}
					done <- observed{result, callErr}
				}()
				select {
				case <-body.Blocked():
				case <-ctx.Done():
					t.Fatal("fixture did not reach active body read")
				}
				wantContext := context.DeadlineExceeded
				if !deadline {
					wantContext = context.Canceled
					cancel()
				}
				got := <-done
				// Assert: no mutation replay, no invented completion, source closes once.
				require.ErrorIs(t, got.err, wantContext)
				require.EqualValues(t, 1, calls.Load())
				require.EqualValues(t, 1, body.Closes())
				require.NotNil(t, got.result)
				require.Equal(t, handle.ProviderID, got.result.Handle.ProviderID)
				if operation == "cancel" {
					require.ErrorIs(t, got.err, prompty.ErrBatchUnknown)
					require.Equal(t, prompty.BatchUnknown, got.result.Handle.Status)
					require.Equal(t, prompty.BatchRunning, handle.Status)
				} else {
					require.ErrorIs(t, got.err, prompty.ErrBatchResults)
					require.Equal(t, prompty.BatchEnded, got.result.Handle.Status)
					require.Equal(t, handle.ProviderStatus, got.result.Handle.ProviderStatus)
					require.Len(t, got.result.Items, 1)
					require.Equal(t, "one", got.result.Items[0].ID)
					require.Equal(t, prompty.BatchItemSuccess, got.result.Items[0].Outcome)
					require.JSONEq(t, batchSuccessLine, string(got.result.Items[0].SourceJSON))
					require.False(t, got.result.Completeness.DownloadComplete)
					require.False(t, got.result.Completeness.CorrelationComplete)
					require.ElementsMatch(t, []string{"two", "three"}, got.result.Completeness.MissingIDs)
				}
			})
		}
	}
}
