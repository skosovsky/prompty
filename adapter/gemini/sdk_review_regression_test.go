package gemini

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestSDKRepresentationLimitRetainsUsageAcrossModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK already decoded known usage; only our representation materialization fails.
			var attempts atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				attempts.Add(1)
				payload := `{"modelVersion":"fixture","candidates":[{"content":{"parts":[{"text":"oversized","thoughtSignature":"c2lnbmF0dXJl"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":7}}`
				if r.URL.Query().Get("alt") == "sse" {
					payload = "data: " + payload + "\n\n"
				}
				_, _ = io.WriteString(w, payload)
			}))
			defer server.Close()
			sdk, err := genai.NewClient(
				t.Context(),
				&genai.ClientConfig{
					APIKey:      "fixture",
					Backend:     genai.BackendGeminiAPI,
					HTTPClient:  server.Client(),
					HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
				},
			)
			require.NoError(t, err)
			profile := prompty.CapabilityProfile{
				Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
				Provenance: "SDK limit regression",
				Capabilities: map[string]prompty.CapabilitySupport{
					prompty.CapabilityNativeStream: prompty.CapabilitySupported,
				},
			}
			client := adapter.NewClient(
				New(
					WithClient(sdk),
					WithModel("fixture"),
					WithProfiles(server.URL, profile),
					WithResponsePayloadLimit(1),
				),
			)
			// Act.
			var result *prompty.Response
			if mode == "" {
				result, err = client.Execute(t.Context(), prompty.SimplePrompt("fixture"))
			} else {
				stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), mode)
				for range stream.Events() {
				}
				require.Equal(t, prompty.StreamFailed, stream.Status().State)
				result, err = stream.Status().Result, stream.Status().Err
			}
			// Assert: no oversized content/private state/success, but known billing evidence survives.
			require.ErrorIs(t, err, prompty.ErrProviderState)
			require.NotNil(t, result)
			require.Equal(t, 7, result.Usage.PromptTokens)
			require.Contains(t, result.Usage.Known, prompty.UsagePrompt)
			require.Empty(t, result.Content)
			require.Empty(t, result.ProviderState)
			require.Empty(t, result.Outcome)
			require.True(t, result.ContinuationUnavailable)
			require.Equal(t, int32(1), attempts.Load())
		})
	}
}

//nolint:gocognit // Compare three native mutation ports and known/unknown rejection classes in one actual SDK matrix.
func TestSDKBatchKnownRejectionAndUnknownAcceptance(t *testing.T) {
	t.Parallel()
	for _, operation := range []string{"inline", "file", "cancel"} {
		t.Run(operation, func(t *testing.T) {
			t.Parallel()
			for _, code := range []int{400, 403, 408, 429, 503} {
				t.Run(strconv.Itoa(code), func(t *testing.T) {
					t.Parallel()
					// Arrange: actual public SDK APIError metadata, no provider body parser.
					var attempts atomic.Int32
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						attempts.Add(1)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(code)
						_, _ = fmt.Fprintf(
							w,
							`{"error":{"code":%d,"message":"private-provider","status":"INVALID_ARGUMENT"}}`,
							code,
						)
					}))
					defer server.Close()
					base, handle := batchOperationFixture(t, server)
					manager := &Batches{base: base, account: "account-fixture"}
					input, _ := inlineBatchFixtureInputs()
					if operation == "file" {
						manager.fileMode = true
						manager.files = &batchFixtureFiles{
							binding: prompty.BatchFileBinding{
								Provider:       "gemini",
								Endpoint:       server.URL,
								AccountBinding: "account-fixture",
							},
						}
					}
					// Act.
					var result *prompty.BatchResult
					var err error
					if operation == "cancel" {
						result, err = manager.Cancel(t.Context(), handle)
					} else {
						result, err = manager.Submit(t.Context(), input)
					}
					// Assert: known rejection stays distinct from unknown acceptance, without replay/privacy leaks.
					if code == 400 || code == 403 {
						require.ErrorIs(t, err, prompty.ErrBatchRejected)
						require.NotErrorIs(t, err, prompty.ErrBatchUnknown)
						if operation == "cancel" {
							require.Equal(t, handle.Status, result.Handle.Status)
						}
					} else {
						require.ErrorIs(t, err, prompty.ErrBatchUnknown)
					}
					require.NotContains(t, err.Error(), "private-provider")
					require.Equal(t, int32(1), attempts.Load())
				})
			}
		})
	}
}

func TestSDKCacheKnownIdentitySurvivesMaterializationFailure(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, expiry, model string
		limit               int
	}{
		{"missing-expiry", "", "models/fixture", defaultResponsePayloadLimit},
		{"foreign-model", `,"expireTime":"2030-01-01T00:00:00Z"`, "models/foreign", defaultResponsePayloadLimit},
		{"projection-limit", `,"expireTime":"2030-01-01T00:00:00Z"`, "models/fixture", 1},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: a genuine SDK DTO exposes identity/count independently of invalid materialization.
			var creates, gets atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodGet {
					gets.Add(1)
					_, _ = io.WriteString(
						w,
						`{"name":"cachedContents/accepted","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z"}`,
					)
					return
				}
				creates.Add(1)
				w.WriteHeader(http.StatusCreated)
				_, _ = fmt.Fprintf(
					w,
					`{"name":"cachedContents/accepted","model":%q,"usageMetadata":{"totalTokenCount":7}%s}`,
					fixture.model,
					fixture.expiry,
				)
			}))
			defer server.Close()
			manager := cacheResourceFixtureManager(t, server)
			manager.base.responseLimit = fixture.limit
			// Act.
			result, err := manager.Create(
				t.Context(),
				prompty.CacheResourceCreateRequest{
					Execution: prompty.SimplePrompt("fixture"),
					Expiry:    prompty.CacheResourceExpiry{TTL: time.Hour},
				},
			)
			// Assert: available identity/count survive without usable cache state or replay.
			require.ErrorIs(t, err, prompty.ErrCacheResourceUnknown)
			require.NotNil(t, result.Handle)
			require.Equal(t, "cachedContents/accepted", result.Handle.ProviderID)
			require.Equal(t, prompty.CacheResourceUnknown, result.Handle.State)
			require.NotNil(t, result.TokenCount)
			require.Equal(t, int64(7), *result.TokenCount)
			encoded, encodeErr := prompty.EncodeCacheResourceHandle(*result.Handle)
			require.NoError(t, encodeErr)
			restored, decodeErr := prompty.DecodeCacheResourceHandle(encoded)
			require.NoError(t, decodeErr)
			require.ErrorIs(
				t,
				prompty.ValidateCacheResourceReuse(restored, restored.Identity, restored.AccountBinding, time.Now()),
				prompty.ErrCacheResourceExpired,
			)
			manager.base.responseLimit = defaultResponsePayloadLimit
			inspected, inspectErr := manager.Get(t.Context(), restored)
			require.NoError(t, inspectErr)
			require.Equal(t, prompty.CacheResourceActive, inspected.Handle.State)
			require.Equal(t, int32(1), creates.Load())
			require.Equal(t, int32(1), gets.Load())
		})
	}
}

func TestSDKCacheCaptureDoesNotInventSuccessfulHTTPStatus(t *testing.T) {
	t.Parallel()
	// Arrange: a successful 201 has no exact status field in the SDK DTO.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(
			w,
			`{"name":"cachedContents/accepted","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z"}`,
		)
	}))
	defer server.Close()
	manager := cacheResourceFixtureManager(t, server)
	sink := &cacheCaptureSink{}
	// Act.
	_, err := manager.Create(
		t.Context(),
		prompty.CacheResourceCreateRequest{
			Execution: captureCacheExecution(sink),
			Expiry:    prompty.CacheResourceExpiry{TTL: time.Hour},
		},
	)
	// Assert: semantic SDK capture exists, status is unknown.
	require.NoError(t, err)
	require.Len(t, sink.records, 5)
	require.Nil(t, sink.records[2].Projection.Metadata.HTTPStatus)
}
