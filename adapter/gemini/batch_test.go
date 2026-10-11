package gemini

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

const inlineBatchFixtureOutput = `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","model":"models/fixture","output":{"inlinedResponses":{"inlinedResponses":[{"metadata":{"key":"three"},"response":{"promptFeedback":{"blockReason":"SAFETY"}}},{"metadata":{"key":"two"},"error":{"code":3,"message":"PRIVATE_ITEM_ERROR"}},{"metadata":{"key":"one"},"response":{"candidates":[{"content":{"role":"model","parts":[{"text":"label"}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":2,"candidatesTokenCount":1,"totalTokenCount":3,"cachedContentTokenCount":0}}}]}}}}`

func inlineBatchFixtureInputs() (prompty.BatchSubmitRequest, prompty.BatchResultsOptions) {
	var input prompty.BatchSubmitRequest
	var options prompty.BatchResultsOptions
	for _, id := range []string{"one", "two", "three"} {
		exec := prompty.SimplePrompt("classify " + id)
		input.Items = append(input.Items, prompty.BatchItem{ID: id, AttemptID: "first", Execution: exec})
		options.Bindings = append(options.Bindings, prompty.BatchResultBinding{ID: id, Execution: exec})
	}
	return input, options
}

func TestActualSDKInlineBatchLifecycleAndCorrelation(t *testing.T) {
	t.Parallel()
	// Arrange: native create, inspect, cancel and inline out-of-order output.
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- r.Method + " " + r.URL.Path + " " + string(body)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, ":batchGenerateContent"):
			_, _ = io.WriteString(
				w,
				`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING","model":"models/fixture"}}`,
			)
		case strings.HasSuffix(r.URL.Path, ":cancel"):
			_, _ = io.WriteString(w, `{}`)
		default:
			_, _ = io.WriteString(w, inlineBatchFixtureOutput)
		}
	}))
	defer server.Close()
	base, _ := batchOperationFixture(t, server)
	batches := NewBatches("account-fixture", WithClient(base.client), WithModel("fixture"),
		WithProfiles(server.URL, base.profiles...))
	input, options := inlineBatchFixtureInputs()
	// Act.
	created, err := batches.Submit(context.Background(), input)
	require.NoError(t, err)
	wire, err := prompty.EncodeBatchHandle(*created.Handle)
	require.NoError(t, err)
	stored, err := prompty.DecodeBatchHandle(wire)
	require.NoError(t, err)
	// Accepted work remains inspectable without a new generation options payload.
	batches.base.profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	inspected, err := batches.Get(context.Background(), stored)
	require.NoError(t, err)
	canceled, err := batches.Cancel(context.Background(), *inspected.Handle)
	require.NoError(t, err)
	results, err := batches.Results(context.Background(), *canceled.Handle, options)
	// Assert: remote completion and per-item outcomes are independent.
	require.NoError(t, err)
	require.True(t, created.SubmissionAttempted)
	require.Equal(t, prompty.BatchQueued, created.Handle.Status)
	require.Equal(t, prompty.BatchEnded, canceled.Handle.Status)
	require.True(t, results.Completeness.DownloadComplete)
	require.True(t, results.Completeness.CorrelationComplete)
	require.Len(t, results.Items, 3)
	require.Equal(t, "three", results.Items[0].ID)
	require.Equal(t, prompty.BatchItemRefusal, results.Items[0].Outcome)
	require.Equal(t, prompty.BatchItemError, results.Items[1].Outcome)
	require.Equal(t, "provider_error", results.Items[1].ErrorKind)
	require.Equal(t, prompty.BatchItemSuccess, results.Items[2].Outcome)
	require.Equal(t, 3, results.Items[2].Response.Usage.TotalTokens)
	require.NotContains(t, results.Items[2].Response.Usage.Known, prompty.UsageCacheRead)
	create := <-requests
	require.True(t, strings.HasPrefix(create, "POST /v1beta/models/fixture:batchGenerateContent "))
	require.Contains(t, create, `"metadata":{"key":"one"}`)
	require.Contains(t, create, `"text":"classify one"`)
	require.Contains(t, <-requests, "GET /v1beta/batches/fixture")
	require.Contains(t, <-requests, "POST /v1beta/batches/fixture:cancel")
	require.Contains(t, <-requests, "GET /v1beta/batches/fixture")
}

func TestInlineBatchUnknownAcceptanceAndZeroIODuplicateOrRebind(t *testing.T) {
	t.Parallel()
	// Arrange: acceptance with a future state keeps a safe native identity.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"FUTURE_STATE"}}`)
	}))
	defer server.Close()
	base, handle := batchOperationFixture(t, server)
	batches := &Batches{base: base, account: "account-fixture"}
	input, options := inlineBatchFixtureInputs()
	// Act/Assert: unknown create is not automatically retried or reported successful.
	result, err := batches.Submit(context.Background(), input)
	require.ErrorIs(t, err, prompty.ErrBatchUnknown)
	require.Equal(t, "batches/fixture", result.Handle.ProviderID)
	require.Equal(t, prompty.BatchUnknown, result.Handle.Status)
	require.EqualValues(t, 1, calls.Load())
	input.Items[1].ID = input.Items[0].ID
	_, err = batches.Submit(context.Background(), input)
	require.ErrorIs(t, err, prompty.ErrBatch)
	handle.Status = prompty.BatchEnded
	options.Bindings = options.Bindings[:1]
	options.Bindings[0].Execution = prompty.SimplePrompt("changed")
	_, err = batches.Results(context.Background(), handle, options)
	require.ErrorIs(t, err, prompty.ErrBatchScope)
	require.EqualValues(t, 1, calls.Load())
}

func TestInlineBatchCompletenessMissingDuplicateAndUnexpected(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name                           string
		output                         string
		missing, duplicate, unexpected []string
	}{
		{"missing", strings.ReplaceAll(inlineBatchFixtureOutput,
			`{"metadata":{"key":"three"},"response":{"promptFeedback":{"blockReason":"SAFETY"}}},`, ""),
			[]string{"three"}, nil, nil},
		{"duplicate", strings.ReplaceAll(inlineBatchFixtureOutput, `"key":"one"`, `"key":"two"`),
			[]string{"one"}, []string{"two"}, nil},
		{"unexpected", strings.ReplaceAll(inlineBatchFixtureOutput, `"key":"one"`, `"key":"foreign"`),
			[]string{"one"}, nil, []string{"foreign"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if r.Method == http.MethodPost {
					_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED"}}`)
					return
				}
				_, _ = io.WriteString(w, tc.output)
			}))
			defer server.Close()
			base, _ := batchOperationFixture(t, server)
			manager := &Batches{base: base, account: "account-fixture"}
			input, bindings := inlineBatchFixtureInputs()
			created, err := manager.Submit(context.Background(), input)
			require.NoError(t, err)
			// Act.
			result, err := manager.Results(context.Background(), *created.Handle, bindings)
			// Assert: transport completeness is not caller correlation completeness.
			require.NoError(t, err)
			require.True(t, result.Completeness.DownloadComplete)
			require.False(t, result.Completeness.CorrelationComplete)
			require.Equal(t, tc.missing, result.Completeness.MissingIDs)
			require.Equal(t, tc.duplicate, result.Completeness.DuplicateIDs)
			require.Equal(t, tc.unexpected, result.Completeness.UnexpectedIDs)
		})
	}
}

func TestSDKInlineBatchMalformedConverterPanicCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: SDK conversion fails on the last item after reading complete JSON.
	payload := strings.ReplaceAll(inlineBatchFixtureOutput,
		`"candidates":[{"content":{"role":"model","parts":[{"text":"label"}]},"finishReason":"STOP"}]`,
		`"candidates":{}`)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED"}}`)
			return
		}
		_, _ = io.WriteString(w, payload)
	}))
	defer server.Close()
	base, _ := batchOperationFixture(t, server)
	manager := &Batches{base: base, account: "account-fixture"}
	input, bindings := inlineBatchFixtureInputs()
	created, err := manager.Submit(context.Background(), input)
	require.NoError(t, err)
	// Act/Assert: SDK-GENAI-STREAM (converter panic) is observed, not repaired.
	require.Panics(t, func() { _, _ = manager.Results(context.Background(), *created.Handle, bindings) })
}

func TestSDKInlineContentEncodingCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: actual SDK conversion uses []map entries and rounds interface numbers.
	bodies := make(chan string, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		bodies <- string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
	}))
	defer server.Close()
	base, _ := batchOperationFixture(t, server)
	var contents []*genai.Content
	decoder := json.NewDecoder(
		strings.NewReader(
			`[{"role":"model","parts":[{"functionCall":{"name":"lookup","args":{"n":9007199254740993,"d":0.1234567890123456789}}}]}]`,
		),
	)
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&contents))
	requests := []*genai.InlinedRequest{{Contents: contents, Metadata: map[string]string{"key": "one"}}}
	// Act.
	_, err := base.client.Batches.Create(context.Background(), "fixture", &genai.BatchJobSource{
		InlinedRequests: requests,
	}, nil)
	// Assert: the actual SDK request rounds dynamic numbers; no exactness repair exists.
	require.NoError(t, err)
	body := <-bodies
	require.Contains(t, body, `9007199254740992`)
	require.Contains(t, body, `0.12345678901234568`)
	require.NotContains(t, body, `9007199254740993`)
}

func TestBatchResultsDoNotRequireOriginalCacheLease(t *testing.T) {
	t.Parallel()
	// Arrange: cache was valid on submission but expires after remote acceptance.
	expires := time.Now().Add(time.Second)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			waitForCacheExpiry(expires)
			_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED"}}`)
			return
		}
		_, _ = io.WriteString(w, inlineBatchFixtureOutput)
	}))
	defer server.Close()
	base, _ := batchOperationFixture(t, server)
	profile := base.RuntimeDescriptor().Profiles[0]
	profile.Capabilities[prompty.CapabilityCacheReuse] = prompty.CapabilitySupported
	manager := NewBatches("account-fixture", WithClient(base.client), WithModel("fixture"),
		WithProfiles(server.URL, profile), WithCacheAccountBinding("account-fixture"))
	input, bindings := inlineBatchFixtureInputs()
	cache := cacheResourceTestHandle(server.URL)
	cache.ExpiresAt = expires
	for i := range input.Items {
		input.Items[i].Execution.CacheResource = &cache
	}
	created, err := manager.Submit(context.Background(), input)
	require.NoError(t, err)
	// Act.
	result, err := manager.Results(context.Background(), *created.Handle, bindings)
	// Assert: original remote lease expiry cannot prevent reading known results.
	require.NoError(t, err)
	require.Len(t, result.Items, 3)
	require.True(t, result.Completeness.CorrelationComplete)
	require.EqualValues(t, 2, calls.Load())
	// The same expired input is still invalid for a new Submit, with no recache/I/O.
	_, err = manager.Submit(context.Background(), input)
	require.ErrorIs(t, err, prompty.ErrCacheResourceExpired)
	require.EqualValues(t, 2, calls.Load())
}
