package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestObservedClientRetainsCallerAuthenticationHeaders(t *testing.T) {
	t.Parallel()
	// Arrange: public SDK auth and caller headers must reach the original transport.
	seen := make(chan http.Header, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen <- r.Header.Clone()
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`)
	}))
	t.Cleanup(server.Close)
	headers := http.Header{"X-Host-Policy": {"caller-value"}}
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture-auth", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta", Headers: headers},
	})
	require.NoError(t, err)
	before := sdk.ClientConfig().HTTPOptions.Headers.Clone()
	client := adapter.NewClient(New(WithClient(sdk), WithProfiles(server.URL)))
	// Act.
	_, err = client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
	// Assert: auth and host policy survive, caller headers are not mutated.
	require.NoError(t, err)
	actual := <-seen
	require.Equal(t, "fixture-auth", actual.Get("X-Goog-Api-Key"))
	require.Equal(t, "caller-value", actual.Get("X-Host-Policy"))
	require.Equal(t, before, sdk.ClientConfig().HTTPOptions.Headers)
}

func TestObservedClientRetainsCancellationAndTimeoutDuringBodyRead(t *testing.T) {
	t.Parallel()
	for _, timeout := range []bool{false, true} {
		name := "cancel"
		if timeout {
			name = "timeout"
		}
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK reads the caller-owned body once until its context is canceled.
			started := make(chan struct{})
			body := &canceledSourceBody{started: started}
			var calls atomic.Int64
			httpClient := &http.Client{Transport: sourceFixtureTransport(func(r *http.Request) (*http.Response, error) {
				calls.Add(1)
				body.ctx = r.Context()
				return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
			})}
			if timeout {
				httpClient.Timeout = 100 * time.Millisecond
			}
			sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
				APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
				HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid", APIVersion: "v1beta"},
			})
			require.NoError(t, err)
			client := adapter.NewClient(New(WithClient(sdk), WithProfiles("https://fixture.invalid")))
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			// Act.
			go func() {
				_, callErr := client.Execute(ctx, prompty.SimplePrompt("fixture"))
				result <- callErr
			}()
			<-started
			if !timeout {
				cancel()
			}
			err = <-result
			// Assert: original cancellation cause, closed body and no retry.
			if timeout {
				require.ErrorIs(t, err, context.DeadlineExceeded)
			} else {
				require.ErrorIs(t, err, context.Canceled)
			}
			require.True(t, body.closed.Load())
			require.Equal(t, int64(1), calls.Load())
			require.Same(t, httpClient, sdk.ClientConfig().HTTPClient)
		})
	}
}

type canceledSourceBody struct {
	ctx     context.Context
	started chan struct{}
	closed  atomic.Bool
}

func (b *canceledSourceBody) Read([]byte) (int, error) {
	close(b.started)
	<-b.ctx.Done()
	return 0, b.ctx.Err()
}

func (b *canceledSourceBody) Close() error { b.closed.Store(true); return nil }

func TestSDKPartRepresentationIgnoresCallerOriginalBody(t *testing.T) {
	t.Parallel()
	for _, original := range []string{
		`{"text":"different","thoughtSignature":"c2lnbmF0dXJl"}`,
		`{"text":"signed","thoughtSignature":"b3RoZXI="}`,
		`{"functionCall":{"name":"other","args":{}},"thoughtSignature":"c2lnbmF0dXJl"}`,
	} {
		t.Run(original, func(t *testing.T) {
			t.Parallel()
			// Arrange: valid signed original body, but not the SDK part at this index.
			raw := &genai.GenerateContentResponse{
				Candidates: []*genai.Candidate{{Content: &genai.Content{Parts: []*genai.Part{
					{Text: "signed", ThoughtSignature: []byte("signature")},
				}}, FinishReason: genai.FinishReasonStop}},
				SDKHTTPResponse: &genai.HTTPResponse{Body: `{"candidates":[{"content":{"parts":[` + original + `]}}]}`},
			}
			before, err := json.Marshal(raw)
			require.NoError(t, err)
			// Act.
			response, err := New().ParseResponse(raw)
			// Assert: canonical state uses SDK representation, never independent body attestation.
			require.NotNil(t, response)
			require.NoError(t, err)
			after, err := json.Marshal(raw)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}

func TestSanitizedResponseFailureRetainsCancellationWithoutRawCause(t *testing.T) {
	t.Parallel()
	// Arrange: cancellation concurrent with a body-bearing SDK decode failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	// Act.
	err := safeObservedResponseError(ctx, prompty.ErrSchemaJSONDecode)
	// Assert: safe classifications only; raw decoder errors are never unwrapped.
	require.ErrorIs(t, err, prompty.ErrProviderState)
	require.ErrorIs(t, err, context.Canceled)
	require.NotErrorIs(t, err, prompty.ErrSchemaJSONDecode)
}

func TestObservedClientDoesNotHideCallerTransportPanics(t *testing.T) {
	t.Parallel()
	// Arrange: an application transport panic is not a malformed provider response.
	httpClient := &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
		panic("caller transport failure")
	})}
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid", APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	client := adapter.NewClient(New(WithClient(sdk), WithProfiles("https://fixture.invalid")))
	// Act / Assert: no blanket recovery of host failures.
	require.PanicsWithValue(t, "caller transport failure", func() {
		_, _ = client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
	})
}
