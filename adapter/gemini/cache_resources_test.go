package gemini

import (
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/skosovsky/prompty/adapter"
)

func cacheResourceFixtureManager(t *testing.T, server *httptest.Server) *CacheResources {
	t.Helper()
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local cache SDK fixture, not live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityCacheCreate:  prompty.CapabilitySupported,
			prompty.CapabilityCacheInspect: prompty.CapabilitySupported,
			prompty.CapabilityCacheUpdate:  prompty.CapabilitySupported,
			prompty.CapabilityCacheDelete:  prompty.CapabilitySupported,
		},
	}
	return NewCacheResources(
		"account-fixture",
		WithClient(sdk),
		WithModel("fixture"),
		WithProfiles(server.URL, profile),
	)
}

func TestActualSDKCacheResourceLifecycle(t *testing.T) {
	t.Parallel()
	// Arrange: actual protocol SDK requests and presence-bearing resource metadata.
	requests := make(chan string, 4)
	expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- r.Method + " " + r.URL.Path + " " + string(body)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = fmt.Fprintf(
			w,
			`{"name":"cachedContents/fixture","model":"models/fixture","expireTime":%q,"usageMetadata":{"totalTokenCount":0}}`,
			expiry,
		)
	}))
	defer server.Close()
	manager := cacheResourceFixtureManager(t, server)
	// Cache content creation is not generation and cannot supply required max_tokens.
	manager.base.profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	exec := prompty.SimplePrompt("immutable document")
	original := exec.Clone()
	// Act.
	created, err := manager.Create(
		context.Background(),
		prompty.CacheResourceCreateRequest{Execution: exec, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
	)
	require.NoError(t, err)
	wire, err := prompty.EncodeCacheResourceHandle(*created.Handle)
	require.NoError(t, err)
	restored, err := prompty.DecodeCacheResourceHandle(wire)
	require.NoError(t, err)
	inspected, err := manager.Get(context.Background(), restored)
	require.NoError(t, err)
	updated, err := manager.UpdateExpiry(
		context.Background(),
		*inspected.Handle,
		prompty.CacheResourceExpiry{TTL: 2 * time.Hour},
	)
	require.NoError(t, err)
	deleted, err := manager.Delete(context.Background(), *updated.Handle)
	// Assert: one explicit request per operation, content identity unchanged.
	require.NoError(t, err)
	require.Equal(t, prompty.CacheResourceActive, created.Handle.State)
	require.Equal(t, prompty.CacheResourceDeleted, deleted.Handle.State)
	require.Equal(t, created.Handle.ContentDigest, deleted.Handle.ContentDigest)
	require.Nil(t, created.TokenCount)
	require.Equal(t, original, exec)
	create := <-requests
	require.True(t, strings.HasPrefix(create, "POST /v1beta/cachedContents "))
	require.Contains(t, create, `"ttl":"3600s"`)
	require.Contains(t, create, `immutable document`)
	require.True(t, strings.HasPrefix(<-requests, "GET /v1beta/cachedContents/fixture "))
	update := <-requests
	require.True(t, strings.HasPrefix(update, "PATCH /v1beta/cachedContents/fixture "))
	require.Contains(t, update, `"ttl":"7200s"`)
	require.True(t, strings.HasPrefix(<-requests, "DELETE /v1beta/cachedContents/fixture "))
}

func cacheResourceTestHandle(endpoint string) prompty.CacheResourceHandle {
	return prompty.CacheResourceHandle{
		Format:         prompty.CacheResourceWireFormat,
		Identity:       prompty.ProfileIdentity{Provider: "gemini", Endpoint: endpoint, Model: "fixture"},
		AccountBinding: "account-fixture", ProviderID: "cachedContents/fixture", ContentDigest: strings.Repeat("a", 64),
		ExpiresAt: time.Now().Add(time.Hour).UTC(), State: prompty.CacheResourceActive,
	}
}

func TestCacheResourceUnknownCreateDeleteNoRetry(t *testing.T) {
	t.Parallel()
	// Arrange: acceptance cannot be determined from a server failure.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	manager := cacheResourceFixtureManager(t, server)
	// Act.
	created, err := manager.Create(
		context.Background(),
		prompty.CacheResourceCreateRequest{
			Execution: prompty.SimplePrompt("document"),
			Expiry:    prompty.CacheResourceExpiry{TTL: time.Hour},
		},
	)
	// Assert: no invented identity and no retry of unknown creation.
	require.ErrorIs(t, err, prompty.ErrCacheResourceUnknown)
	require.NotNil(t, created)
	require.Nil(t, created.Handle)
	require.EqualValues(t, 1, calls.Load())
	handle := cacheResourceTestHandle(server.URL)
	deleted, err := manager.Delete(context.Background(), handle)
	require.ErrorIs(t, err, prompty.ErrCacheResourceUnknown)
	require.Equal(t, prompty.CacheResourceUnknown, deleted.Handle.State)
	require.Equal(t, handle.ProviderID, deleted.Handle.ProviderID)
	require.Equal(t, prompty.CacheResourceActive, handle.State)
	require.EqualValues(t, 2, calls.Load())
}

func TestCacheResourceScopedPreflight(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	manager := cacheResourceFixtureManager(t, server)
	handle := cacheResourceTestHandle(server.URL)
	for _, mutate := range []func(*prompty.CacheResourceHandle){
		func(h *prompty.CacheResourceHandle) { h.Identity.Endpoint = "https://other.invalid" },
		func(h *prompty.CacheResourceHandle) { h.AccountBinding = "other" },
		func(h *prompty.CacheResourceHandle) { h.ProviderID = "cachedContents/../secrets" },
		func(h *prompty.CacheResourceHandle) { h.Identity.Model = "other" },
	} {
		copyHandle := handle
		mutate(&copyHandle)
		// Act.
		_, err := manager.Get(context.Background(), copyHandle)
		// Assert.
		require.Error(t, err)
	}
	require.Zero(t, calls.Load())
	// Generation-only configuration is not silently converted to immutable inputs.
	exec := prompty.SimplePrompt("document")
	exec.ModelOptions = &prompty.ModelOptions{Temperature: new(0.5)}
	_, err := manager.Create(
		context.Background(),
		prompty.CacheResourceCreateRequest{Execution: exec, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
	)
	require.ErrorIs(t, err, prompty.ErrCacheRequirement)
	require.Zero(t, calls.Load())
}

func TestCacheResourcePresenceAndMalformedResponse(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, payload string
		known         bool
		fails         bool
	}{
		{"unknown", `{}`, false, false},
		{"known zero", `{"totalTokenCount":0}`, false, false},
		{"negative", `{"totalTokenCount":-1}`, false, true},
		{"null", `{"totalTokenCount":null}`, false, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = fmt.Fprintf(
					w,
					`{"name":"cachedContents/fixture","model":"models/fixture","expireTime":%q,"usageMetadata":%s}`,
					expiry,
					test.payload,
				)
			}))
			defer server.Close()
			manager := cacheResourceFixtureManager(t, server)
			// Act.
			result, err := manager.Get(context.Background(), cacheResourceTestHandle(server.URL))
			// Assert.
			if test.fails {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
			require.Equal(t, test.known, result.TokenCount != nil)
			if result.TokenCount != nil {
				_, marshalErr := json.Marshal(result)
				require.NoError(t, marshalErr)
			}
		})
	}
}

func TestActualSDKCacheGenerationReuseAndInvalidHandles(t *testing.T) {
	t.Parallel()
	// Arrange: create immutable content once, then ask without resending it.
	var calls atomic.Int32
	requests := make(chan []byte, 3)
	var creates atomic.Int32
	expiry := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(r.Body)
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "generateContent") {
			_, _ = io.WriteString(
				w,
				`{"candidates":[{"content":{"role":"model","parts":[{"text":"answer"}]},"finishReason":"STOP"}],`+
					`"usageMetadata":{"promptTokenCount":10,"candidatesTokenCount":2,"totalTokenCount":12,"cachedContentTokenCount":3}}`,
			)
			return
		}
		creates.Add(1)
		_, _ = fmt.Fprintf(w, `{"name":"cachedContents/fixture","model":"models/fixture","expireTime":%q}`, expiry)
	}))
	defer server.Close()
	manager := cacheResourceFixtureManager(t, server)
	created, err := manager.Create(context.Background(), prompty.CacheResourceCreateRequest{
		Execution: prompty.SimplePrompt(
			"immutable large document",
		), Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}})
	require.NoError(t, err)
	originalHandle := *created.Handle
	profile := manager.base.RuntimeDescriptor().Profiles[0]
	profile.Capabilities[prompty.CapabilityCacheReuse] = prompty.CapabilitySupported
	profile.Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
	adp := New(
		WithClient(manager.base.client),
		WithModel("fixture"),
		WithProfiles(server.URL, profile),
		WithCacheAccountBinding("account-fixture"),
	)
	exec := prompty.SimplePrompt("question")
	exec.CacheResource = created.Handle
	wire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	restored, err := prompty.UnmarshalExecution(wire)
	require.NoError(t, err)
	// Act.
	response, err := adapter.NewClient(adp).Execute(context.Background(), restored)
	// Assert: no create/upload replay; cached usage is a subset, not added twice.
	require.NoError(t, err)
	require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
	require.Equal(t, 12, response.Usage.TotalTokens)
	require.Equal(t, 3, response.Usage.PromptTokensCached)
	require.Contains(t, response.Usage.Known, prompty.UsageCacheRead)
	<-requests
	var request map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.JSONEq(t, `"cachedContents/fixture"`, string(request["cachedContent"]))
	require.NotContains(t, string(request["contents"]), "immutable large document")
	require.EqualValues(t, 2, calls.Load())
	// Arrange: independent question, same immutable document/config binding.
	second := prompty.SimplePrompt("independent second question")
	secondHandle := *created.Handle
	second.CacheResource = &secondHandle
	// Act.
	secondResponse, err := adapter.NewClient(adp).Execute(context.Background(), second)
	// Assert: exactly one resource creation and no first-question/document replay.
	require.NoError(t, err)
	require.Equal(t, 12, secondResponse.Usage.TotalTokens)
	require.Equal(t, 3, secondResponse.Usage.PromptTokensCached)
	request = nil
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.JSONEq(t, `"cachedContents/fixture"`, string(request["cachedContent"]))
	require.Contains(t, string(request["contents"]), "independent second question")
	require.NotContains(t, string(request["contents"]), "immutable large document")
	require.Equal(t, originalHandle, *created.Handle)
	require.Equal(t, originalHandle, *restored.CacheResource)
	require.EqualValues(t, 1, creates.Load())
	require.EqualValues(t, 3, calls.Load())
	for _, mutate := range []func(*prompty.CacheResourceHandle){
		func(h *prompty.CacheResourceHandle) { h.ExpiresAt = time.Now().Add(-time.Second) },
		func(h *prompty.CacheResourceHandle) { h.Identity.Model = "other" },
		func(h *prompty.CacheResourceHandle) { h.Identity.Endpoint = "https://other.invalid" },
		func(h *prompty.CacheResourceHandle) { h.AccountBinding = "other" },
		func(h *prompty.CacheResourceHandle) { h.State = prompty.CacheResourceUnknown },
		func(h *prompty.CacheResourceHandle) { h.State = prompty.CacheResourceDeleted },
	} {
		bad := exec.Clone()
		mutate(bad.CacheResource)
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			_, prepareErr := adapter.Prepare(adp, bad, mode)
			require.Error(t, prepareErr)
		}
	}
	require.EqualValues(t, 3, calls.Load())
	// Rebinding transport without account metadata must not grant reuse.
	_, err = adapter.Prepare(
		New(WithClient(manager.base.client), WithModel("fixture"), WithProfiles(server.URL, profile)),
		restored,
		"",
	)
	require.ErrorIs(t, err, prompty.ErrCacheResourceScope)
}

func TestCacheResourceFaultResponses(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, body string
		limit      int
	}{
		{"wrong resource", `{"name":"cachedContents/other","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z"}`, 4096},
		{"wrong model", `{"name":"cachedContents/fixture","model":"models/other","expireTime":"2030-01-01T00:00:00Z"}`, 4096},
		{"case alias", `{"Name":"cachedContents/fixture","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z"}`, 4096},
		{"duplicate identity", `{"name":"cachedContents/other","name":"cachedContents/fixture","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z"}`, 4096},
		{"partial json", `{"name":"cachedContents/fixture"`, 4096},
		{"bounded", `{"name":"cachedContents/fixture","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z"}`, 20},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: unsafe/partial response cannot replace a previously known identity.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, test.body)
			}))
			defer server.Close()
			manager := cacheResourceFixtureManager(t, server)
			manager.base.responseLimit = test.limit
			handle := cacheResourceTestHandle(server.URL)
			// Act.
			result, err := manager.UpdateExpiry(
				context.Background(),
				handle,
				prompty.CacheResourceExpiry{TTL: time.Hour},
			)
			// Assert: unknown mutation preserves a reconcilable handle and has no retry.
			if test.name == "case alias" || test.name == "duplicate identity" {
				// SDK-GENAI-SOURCE: normalized SDK DTO cannot reveal original key ambiguity.
				require.NoError(t, err)
				require.Equal(t, prompty.CacheResourceActive, result.Handle.State)
			} else {
				require.ErrorIs(t, err, prompty.ErrCacheResourceUnknown)
				require.Equal(t, prompty.CacheResourceUnknown, result.Handle.State)
			}
			require.Equal(t, handle.ProviderID, result.Handle.ProviderID)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestSDKCacheDecodeFailureDoesNotRecoverIdentity(t *testing.T) {
	t.Parallel()
	// Arrange: a complete accepted response has identity but an SDK-unrepresentable counter.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"name":"cachedContents/accepted","model":"models/fixture","expireTime":"2030-01-01T00:00:00Z",`+
				`"usageMetadata":{"totalTokenCount":2147483648}}`,
		)
	}))
	defer server.Close()
	manager := cacheResourceFixtureManager(t, server)
	// Act.
	result, err := manager.Create(context.Background(), prompty.CacheResourceCreateRequest{
		Execution: prompty.SimplePrompt("document"), Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}})
	// Assert: SDK-MUTATION-ID has no returned DTO identity; acceptance stays unknown.
	require.ErrorIs(t, err, prompty.ErrCacheResourceUnknown)
	require.Nil(t, result.Handle)
}

func TestSDKCacheContentNumericSerializationCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: cache creation is another existing SDK serialization path.
	requestBody := make(chan []byte, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "read", http.StatusInternalServerError)
			return
		}
		requestBody <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(
			w,
			`{"name":"cachedContents/fixture","model":"models/fixture","expireTime":%q}`,
			time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
		)
	}))
	t.Cleanup(server.Close)
	manager := cacheResourceFixtureManager(t, server)
	args := `{"nested":[{"integer":9007199254740993,"fraction":0.10000000000000001}]}`
	execution := prompty.SimplePrompt("fixture").
		AddMessage(prompty.ChatMessage{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{prompty.ToolCallPart{ID: "call", Name: "lookup", Args: args}}})
	// Act.
	result, err := manager.Create(
		t.Context(),
		prompty.CacheResourceCreateRequest{Execution: execution, Expiry: prompty.CacheResourceExpiry{TTL: time.Hour}},
	)
	// Assert: the actual SDK cache body retains exact values and ID.
	require.NoError(t, err)
	require.NotNil(t, result.Handle)
	native := geminiJSONValue(<-requestBody).(map[string]any)
	contents := native["contents"].([]any)
	require.Len(t, contents, 2)
	call := contents[1].(map[string]any)["parts"].([]any)[0].(map[string]any)["functionCall"].(map[string]any)
	require.Equal(t, "call", call["id"])
	require.Equal(t, geminiJSONValue([]byte(`{"nested":[{"integer":9007199254740992,"fraction":0.1}]}`)), call["args"])
}
