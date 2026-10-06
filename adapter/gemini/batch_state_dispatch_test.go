package gemini

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

type stateExpiryBatchFiles struct {
	*batchFixtureFiles

	expires time.Time
}

func (f *stateExpiryBatchFiles) Upload(
	ctx context.Context, input prompty.BatchFileUpload,
) (*prompty.BatchFileUploadResult, error) {
	result, err := f.batchFixtureFiles.Upload(ctx, input)
	if err == nil {
		waitForCacheExpiry(f.expires)
	}
	return result, err
}

func signedBatchExpiryExecution(t *testing.T, base *Adapter) (*prompty.PromptExecution, time.Time) {
	t.Helper()
	response, err := base.ParseResponseForProfile(&genai.GenerateContentResponse{
		Candidates: []*genai.Candidate{{
			Content:      &genai.Content{Parts: []*genai.Part{{Text: "signed", ThoughtSignature: []byte("signature")}}},
			FinishReason: genai.FinishReasonStop,
		}},
	}, base.profiles[0].Identity)
	require.NoError(t, err)
	expires := time.Now().Add(time.Second)
	response.ProviderState[0].Envelope.ExpiresAt = &expires
	message, err := response.AssistantMessage()
	require.NoError(t, err)
	return prompty.SimplePrompt("question").AddMessage(message), expires
}

func TestBatchStateExpiryBeforeRemoteSubmit(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"inline", "before upload", "after upload"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			// Arrange: capture or upload outlives valid original signed state.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			base, _ := batchOperationFixture(t, server)
			base.profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
			exec, expires := signedBatchExpiryExecution(t, base)
			sink := &cacheExpirySink{cacheCaptureSink: &cacheCaptureSink{}, expires: expires}
			files := &stateExpiryBatchFiles{batchFixtureFiles: &batchFixtureFiles{
				binding: prompty.BatchFileBinding{
					Provider:       "gemini",
					Endpoint:       server.URL,
					AccountBinding: "account-fixture",
				},
			}, expires: time.Time{}}
			if phase == "after upload" {
				sink.expires = time.Time{}
				files.expires = expires
			}
			opts := []Option{WithClient(base.client), WithModel("fixture"), WithProfiles(server.URL, base.profiles...)}
			manager := NewBatches("account-fixture", opts...)
			if phase != "inline" {
				manager = NewFileBatches("account-fixture", files, BatchFileOptions{}, opts...)
			}
			input := prompty.BatchSubmitRequest{
				Items:   []prompty.BatchItem{{ID: "item", AttemptID: "attempt", Execution: exec}},
				Options: prompty.BatchOperationOptions{Binding: &prompty.CaptureBinding{Sink: sink}},
			}
			// Act.
			result, err := manager.Submit(t.Context(), input)
			// Assert: no job dispatch/replay; a completed upload stays explicitly known.
			require.ErrorIs(t, err, prompty.ErrStateExpired)
			require.NotErrorIs(t, err, prompty.ErrBatchUnknown)
			require.Zero(t, calls.Load())
			require.False(t, result.SubmissionAttempted)
			require.Nil(t, result.Handle)
			require.True(t, result.Capture.Closed)
			if phase == "after upload" {
				require.Equal(t, 1, files.uploads)
				require.Equal(
					t,
					[]prompty.BatchResourceRef{{Kind: "input", ProviderID: "files/input"}},
					result.Resources,
				)
				require.True(t, sink.records[len(sink.records)-1].AfterDispatch)
			} else {
				require.Zero(t, files.uploads)
				require.Empty(t, result.Resources)
				require.False(t, sink.records[len(sink.records)-1].AfterDispatch)
			}
		})
	}
}
