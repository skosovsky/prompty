package openai

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/operationtest"
)

func TestResponsesActiveInterruptionPreservesKnownIdentity(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"create", "get", "cancel"} {
		for _, deadline := range []bool{false, true} {
			t.Run(operation+"/"+map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
				t.Parallel()
				// Arrange: SDK receives HTTP acceptance, but body never reaches complete JSON.
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					writeResponseFixture(w, "completed", "reconciled")
				}))
				defer server.Close()
				manager := responseFixtureAdapter(server)
				originalClient := manager.base.client
				handle := remoteFixtureHandle(manager)
				ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				defer cancel()
				var calls atomic.Int32
				body := operationtest.NewBody(ctx, `{"id":"resp_fixture","status":"in_progress",`)
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
				client := sdk.NewClient(option.WithAPIKey("fixture"), option.WithBaseURL(server.URL+"/v1"),
					option.WithHTTPClient(httpClient))
				manager.base.client = &client
				type observed struct {
					result *prompty.RemoteResponseResult
					err    error
				}
				done := make(chan observed, 1)
				// Act.
				go func() {
					var result *prompty.RemoteResponseResult
					var err error
					switch operation {
					case "create":
						result, err = manager.Create(ctx, prompty.RemoteResponseRequest{
							Execution: prompty.SimplePrompt("document"), Background: true, Store: new(true),
						})
					case "get":
						result, err = manager.Get(ctx, handle)
					case "cancel":
						result, err = manager.Cancel(ctx, handle)
					}
					done <- observed{result, err}
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
				// Assert: partial JSON is not an authoritative ID/status observation.
				require.ErrorIs(t, got.err, wantContext)
				require.EqualValues(t, 1, calls.Load())
				require.EqualValues(t, 1, body.Closes())
				require.NotNil(t, got.result)
				if operation == "create" {
					require.ErrorIs(t, got.err, prompty.ErrRemoteResponseUnknown)
					require.Nil(t, got.result.Handle)
				} else {
					require.Equal(t, handle.ProviderID, got.result.Handle.ProviderID)
					if operation == "cancel" {
						require.ErrorIs(t, got.err, prompty.ErrRemoteResponseUnknown)
						require.Equal(t, prompty.RemoteResponseUnknown, got.result.Handle.Status)
					} else {
						require.ErrorIs(t, got.err, prompty.ErrRemoteResponseUnavailable)
						require.Equal(t, handle.Status, got.result.Handle.Status)
					}
				}
				require.Equal(t, prompty.RemoteResponseQueued, handle.Status)
				if operation != "create" {
					// Reconciliation is an explicit new host operation, not automatic retry.
					manager.base.client = originalClient
					reconciled, getErr := manager.Get(t.Context(), *got.result.Handle)
					require.NoError(t, getErr)
					require.Equal(t, prompty.RemoteResponseCompleted, reconciled.Handle.Status)
					require.EqualValues(t, 1, calls.Load())
				}
			})
		}
	}
}
