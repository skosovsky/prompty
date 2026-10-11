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

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
)

func batchOperationFixture(t *testing.T, server *httptest.Server) (*Adapter, prompty.BatchHandle) {
	t.Helper()
	client, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	identity := prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"}
	profile := prompty.CapabilityProfile{
		Identity: identity, Provenance: "local batch SDK fixture; no live conformance",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityBatchSubmit:  prompty.CapabilitySupported,
			prompty.CapabilityBatchResults: prompty.CapabilitySupported,
			prompty.CapabilityBatchInspect: prompty.CapabilitySupported,
			prompty.CapabilityBatchCancel:  prompty.CapabilitySupported,
		},
	}
	exec := prompty.SimplePrompt("fixture")
	digest, err := prompty.BatchExecutionDigest(exec)
	require.NoError(t, err)
	handle := prompty.BatchHandle{
		Format: prompty.BatchWireFormat, Provider: "gemini", Endpoint: server.URL,
		AccountBinding: "account-fixture", ProviderID: "batches/fixture", Status: prompty.BatchRunning,
		ProviderStatus: "JOB_STATE_RUNNING",
		Items: []prompty.BatchItemBinding{{ID: "one", OriginalID: "one", AttemptID: "first",
			Identity: identity, ExecutionDigest: digest}},
	}
	return New(WithClient(client), WithModel("fixture"), WithProfiles(server.URL, profile)), handle
}

func TestActualSDKBatchInspectionAndCancelAcknowledgement(t *testing.T) {
	t.Parallel()
	// Arrange: a real native operation resource and an empty cancel acknowledgement.
	requests := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests <- r.Method + " " + r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{}`)
			return
		}
		_, _ = io.WriteString(
			w,
			`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_RUNNING","model":"models/fixture","output":{"inlinedResponses":{"inlinedResponses":[{"metadata":{"key":"one"},"response":{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"n":9007199254740993}}}]}}]}}]}}}}`,
		)
	}))
	defer server.Close()
	base, handle := batchOperationFixture(t, server)
	// Act: inspect through the actual SDK, then cancel without implicit polling.
	inspected, err := base.batchOperation(context.Background(), "account-fixture", handle,
		prompty.BatchOperationOptions{}, false)
	require.NoError(t, err)
	canceled, err := base.batchOperation(context.Background(), "account-fixture", *inspected.Handle,
		prompty.BatchOperationOptions{}, true)
	// Assert: cancellation is only acknowledged and prior caller input is isolated.
	require.NoError(t, err)
	require.Equal(t, prompty.BatchRunning, inspected.Handle.Status)
	require.Equal(t, prompty.BatchCanceling, canceled.Handle.Status)
	require.Equal(t, prompty.BatchRunning, handle.Status)
	require.Equal(t, "JOB_STATE_RUNNING", canceled.Handle.ProviderStatus)
	require.Equal(t, "GET /v1beta/batches/fixture", <-requests)
	require.Equal(t, "POST /v1beta/batches/fixture:cancel", <-requests)
	require.Empty(t, requests)

	// SDK representation capture exposes the SDK numeric limitation.
	source, err := observeBatchOperation(
		context.Background(),
		defaultResponsePayloadLimit,
		func(ctx context.Context) (*genai.BatchJob, error) {
			return base.client.Batches.Get(ctx, handle.ProviderID, nil)
		},
	)
	require.NoError(t, err)
	require.Contains(t, string(batchSourceJSON(source)), `9007199254740992`)
	require.NotContains(t, string(batchSourceJSON(source)), `9007199254740993`)
}

func TestBatchNativeInspectionFaultsPreservePriorHandle(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name    string
		payload string
	}{
		{"foreign", `{"name":"batches/other","metadata":{"state":"JOB_STATE_SUCCEEDED"}}`},
		{"model", `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","model":"models/other"}}`},
		{"duplicate", `{"name":"batches/fixture","name":"batches/other","metadata":{"state":"JOB_STATE_SUCCEEDED"}}`},
		{"malformed", `{"name":"batches/fixture","metadata":{"state":{}}}`},
		{"unknown", `{"name":"batches/fixture","metadata":{"state":"FUTURE_STATUS"}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, tc.payload)
			}))
			defer server.Close()
			base, handle := batchOperationFixture(t, server)
			// Act.
			result, err := base.batchOperation(context.Background(), "account-fixture", handle,
				prompty.BatchOperationOptions{}, false)
			// Assert: no guessed status/identity and exactly one native call.
			require.Error(t, err)
			require.Equal(t, handle, *result.Handle)
			require.EqualValues(t, 1, calls.Load())
		})
	}
}

func TestBatchNativeSourceLimitAndMutationUnknown(t *testing.T) {
	t.Parallel()
	// Arrange: body overflow and retryable HTTP failure are different observations.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, `{"error":{"message":"PRIVATE_PROVIDER_ERROR","code":500}}`)
			return
		}
		_, _ = io.WriteString(
			w,
			`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_RUNNING"},"padding":"`+strings.Repeat(
				"x",
				2048,
			)+`"}`,
		)
	}))
	defer server.Close()
	base, handle := batchOperationFixture(t, server)
	base.responseLimit = 20
	// Act.
	result, err := base.batchOperation(context.Background(), "account-fixture", handle,
		prompty.BatchOperationOptions{}, false)
	require.ErrorIs(t, err, prompty.ErrBatchUnavailable)
	require.Equal(t, handle, *result.Handle)
	result, err = base.batchOperation(context.Background(), "account-fixture", handle,
		prompty.BatchOperationOptions{}, true)
	// Assert: unknown cancellation has no retry or provider error leakage.
	require.ErrorIs(t, err, prompty.ErrBatchUnknown)
	require.NotContains(t, err.Error(), "PRIVATE_PROVIDER_ERROR")
	require.Equal(t, prompty.BatchUnknown, result.Handle.Status)
	require.EqualValues(t, 2, calls.Load())
}

func TestBatchNativeScopeBeforeIOAndTerminalCancelRace(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	}))
	defer server.Close()
	base, handle := batchOperationFixture(t, server)
	// Act/Assert: wrong account and unsafe resource name cannot reach the SDK.
	_, err := base.batchOperation(context.Background(), "other", handle, prompty.BatchOperationOptions{}, false)
	require.ErrorIs(t, err, prompty.ErrBatchScope)
	unsafe := prompty.CloneBatchHandle(handle)
	unsafe.ProviderID = "batches/../secret"
	_, err = base.batchOperation(context.Background(), "account-fixture", unsafe, prompty.BatchOperationOptions{}, true)
	require.ErrorIs(t, err, prompty.ErrBatchScope)
	_, err = base.batchOperation(context.Background(), "account-fixture", handle, prompty.BatchOperationOptions{
		Capture: prompty.CaptureRequirement{Stages: []prompty.CaptureStage{prompty.CaptureProviderResponse},
			Accuracy: prompty.CaptureBytes},
	}, false)
	require.ErrorIs(t, err, prompty.ErrCaptureUnsupported)
	require.Zero(t, calls.Load())
	handle.Status = prompty.BatchEnded
	handle.ProviderStatus = "JOB_STATE_SUCCEEDED"
	result, err := base.batchOperation(context.Background(), "account-fixture", handle,
		prompty.BatchOperationOptions{}, true)
	require.NoError(t, err)
	require.Equal(t, prompty.BatchEnded, result.Handle.Status)
	encoded, err := prompty.EncodeBatchHandle(*result.Handle)
	require.NoError(t, err)
	require.True(t, json.Valid(encoded))
}
