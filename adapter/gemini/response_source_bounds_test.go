package gemini

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestOriginalSignedResponseSourceBounds(t *testing.T) {
	t.Parallel()
	const body = `{"candidates":[{"content":{"role":"model","parts":[{"text":"signed","thoughtSignature":"c2lnbmF0dXJl"}]},"finishReason":"STOP"}]}`
	for _, limit := range []int{len(body), len(body) - 1, 0} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			t.Parallel()
			// Arrange: exact-limit success, no truncation accepted as original state.
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, body)
			}))
			t.Cleanup(server.Close)
			client := boundedResponseFixtureClient(t, server, limit)
			// Act.
			result, err := client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
			// Assert: a bound failure never publishes a false signed envelope.
			if limit == len(body) {
				require.NoError(t, err)
				require.Len(t, result.ProviderState, 1)
			} else {
				require.ErrorIs(t, err, prompty.ErrProviderState)
				require.Nil(t, result)
			}
			if limit == 0 {
				require.Zero(t, calls.Load())
			} else {
				require.Equal(t, int64(1), calls.Load())
			}
		})
	}
}

func TestSignedSourceDecodeOverflowHasSafeTypedError(t *testing.T) {
	t.Parallel()
	for _, limit := range []int{defaultResponsePayloadLimit, 32} {
		t.Run(strconv.Itoa(limit), func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK decoding fails before a normalized response exists.
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(
					w,
					`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"private-name","args":{"huge":1e1000}},"thoughtSignature":"c2lnbmF0dXJl"}]},"finishReason":"STOP"}]}`,
				)
			}))
			t.Cleanup(server.Close)
			client := boundedResponseFixtureClient(t, server, limit)
			// Act.
			result, err := client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
			// Assert: no provider body/signature echo and no second request.
			require.Nil(t, result)
			require.ErrorIs(t, err, prompty.ErrProviderState)
			require.NotContains(t, err.Error(), "private-name")
			require.NotContains(t, err.Error(), "c2lnbmF0dXJl")
			require.Equal(t, int64(1), calls.Load())
		})
	}
}

func boundedResponseFixtureClient(t *testing.T, server *httptest.Server, limit int) prompty.Invoker {
	t.Helper()
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	return adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL),
		WithResponsePayloadLimit(limit)))
}

func TestSharedSDKOriginalResponseSourcesAreInvocationLocal(t *testing.T) {
	t.Parallel()
	// Arrange: all concurrent calls share one injected SDK and adapter.
	var calls atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		var request struct {
			Contents []struct {
				Parts []struct {
					Text string `json:"text"`
				} `json:"parts"`
			} `json:"contents"`
		}
		if json.NewDecoder(r.Body).Decode(&request) != nil {
			http.Error(w, "fixture decode failure", http.StatusBadRequest)
			return
		}
		value := request.Contents[0].Parts[0].Text
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"big":`+
			value+`}},"thoughtSignature":"c2lnbmF0dXJl"}]},"finishReason":"STOP"}]}`)
	}))
	t.Cleanup(server.Close)
	client := boundedResponseFixtureClient(t, server, defaultResponsePayloadLimit)
	for index := range 20 {
		t.Run(strconv.Itoa(index), func(t *testing.T) {
			t.Parallel()
			value := strconv.FormatInt(int64(9007199254740993)+int64(index)*2, 10)
			// Act.
			result, err := client.Execute(context.Background(), prompty.SimplePrompt(value))
			// Assert: SDK-rounded nearby values cannot cross invocation boundaries.
			require.NoError(t, err)
			part := result.Content[0].(prompty.ToolCallPart)
			require.Equal(t, `{"big":`+value+`}`, part.Args)
		})
	}
	t.Cleanup(func() { require.Equal(t, int64(20), calls.Load()) })
}

func TestObservedSourceReadFailureClosesBodyWithoutReplay(t *testing.T) {
	t.Parallel()
	// Arrange: a caller transport exposes partial bytes and an authoritative read error.
	body := &sourceFailureBody{reader: strings.NewReader(`{"candidates":[`)}
	var calls atomic.Int64
	httpClient := &http.Client{Transport: sourceFixtureTransport(func(_ *http.Request) (*http.Response, error) {
		calls.Add(1)
		return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
	})}
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid", APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	client := adapter.NewClient(New(WithClient(sdk), WithProfiles("https://fixture.invalid")))
	// Act.
	result, err := client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
	// Assert: partial original bytes cannot become a response or another provider call.
	require.Nil(t, result)
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.True(t, body.closed.Load())
	require.Equal(t, int64(1), calls.Load())
}

type sourceFixtureTransport func(*http.Request) (*http.Response, error)

func (f sourceFixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type sourceFailureBody struct {
	reader *strings.Reader
	closed atomic.Bool
}

func (b *sourceFailureBody) Read(data []byte) (int, error) {
	if b.reader.Len() == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	return b.reader.Read(data)
}

func (b *sourceFailureBody) Close() error { b.closed.Store(true); return nil }

func TestObservedClientRetainsCallerRedirectDenial(t *testing.T) {
	t.Parallel()
	// Arrange: host authorization policy denies the redirected request.
	var targetCalls atomic.Int64
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		targetCalls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(target.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusTemporaryRedirect)
	}))
	t.Cleanup(server.Close)
	denied := errors.New("host denied redirect")
	httpClient := server.Client()
	httpClient.CheckRedirect = func(_ *http.Request, _ []*http.Request) error { return denied }
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: httpClient,
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	client := adapter.NewClient(New(WithClient(sdk), WithProfiles(server.URL)))
	// Act.
	result, err := client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
	// Assert: observation does not replace or weaken caller policy.
	require.Nil(t, result)
	require.ErrorIs(t, err, denied)
	require.Zero(t, targetCalls.Load())
}

func TestObservedSignedBodyAmbiguityNeverCreatesContinuation(t *testing.T) {
	t.Parallel()
	for _, part := range []string{
		`{"text":"lost","text":"kept","thoughtSignature":"c2lnbmF0dXJl"}`,
		`{"text":"signed","private_extension":null,"thoughtSignature":"c2lnbmF0dXJl"}`,
	} {
		t.Run(part, func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK silently normalizes the duplicate or unknown signed field.
			var calls atomic.Int64
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{"candidates":[{"content":{"parts":[`+part+`]},"finishReason":"STOP"}]}`)
			}))
			t.Cleanup(server.Close)
			client := boundedResponseFixtureClient(t, server, defaultResponsePayloadLimit)
			// Act.
			result, err := client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
			// Assert: typed failure rather than SDK-normalized signed history.
			require.Nil(t, result)
			require.ErrorIs(t, err, prompty.ErrProviderState)
			require.Equal(t, int64(1), calls.Load())
		})
	}
}
