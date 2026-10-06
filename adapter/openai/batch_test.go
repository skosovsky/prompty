package openai

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

const batchSuccessLine = `{"id":"batch_req_one","custom_id":"one","response":{"status_code":200,"request_id":"req_fixture","body":{"id":"chatcmpl_fixture","object":"chat.completion","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"answer"},"finish_reason":"stop"}],"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7,"prompt_tokens_details":{"cached_tokens":2}}}},"error":null}`

const batchExpiredLine = `{"id":"batch_req_three","custom_id":"three","response":null,"error":{"code":"batch_expired","message":"private-reason"}}`

const batchErrorLine = `{"id":"batch_req_two","custom_id":"two","response":{"status_code":400,"request_id":"req_error","body":{"error":{"type":"invalid_request_error","message":"private-error"}}},"error":null}`

func batchFixtureManager(server *httptest.Server) *Batches {
	client := sdk.NewClient(
		option.WithBaseURL(server.URL+"/v1"),
		option.WithAPIKey("fixture-secret"),
		option.WithHTTPClient(server.Client()),
	)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "openai", Endpoint: server.URL + "/v1", Model: "fixture"},
		Provenance: "local SDK file-backed batch fixture, not live model support",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityBatchSubmit:      prompty.CapabilitySupported,
			prompty.CapabilityBatchInspect:     prompty.CapabilitySupported,
			prompty.CapabilityBatchCancel:      prompty.CapabilitySupported,
			prompty.CapabilityBatchResults:     prompty.CapabilitySupported,
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
		},
	}
	return NewBatches(
		"host-account",
		NewBatchFiles("host-account", server.URL+"/v1", &client),
		WithClient(&client),
		WithModel("fixture"),
		WithProfiles(server.URL+"/v1", profile),
	)
}

func batchFixtureInput() prompty.BatchSubmitRequest {
	var input prompty.BatchSubmitRequest
	for _, id := range []string{"one", "two", "three"} {
		input.Items = append(
			input.Items,
			prompty.BatchItem{ID: id, AttemptID: "attempt-1", Execution: prompty.SimplePrompt("document-" + id)},
		)
	}
	return input
}

func batchFixtureBindings(input prompty.BatchSubmitRequest) prompty.BatchResultsOptions {
	var options prompty.BatchResultsOptions
	for _, item := range input.Items {
		options.Bindings = append(options.Bindings, prompty.BatchResultBinding{ID: item.ID, Execution: item.Execution})
	}
	return options
}

func writeBatchUploadFixture(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(
		w,
		`{"id":"file-input","object":"file","purpose":"batch","filename":"batch.jsonl","bytes":100,"created_at":1}`,
	)
}

func writeBatchFixture(w http.ResponseWriter, status string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(
		w,
		`{"id":"batch_fixture","object":"batch","endpoint":"/v1/chat/completions","input_file_id":"file-input","completion_window":"24h","status":%q,"error_file_id":"file-errors","output_file_id":"file-output","model":"fixture"}`,
		status,
	)
}

func TestBatchesActualSDKFilesReverseCorrelationAndLifecycle(t *testing.T) {
	t.Parallel()
	// Arrange.
	requests := make(chan string, 6)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.URL.Path == "/v1/files" {
			file, header, err := r.FormFile("file")
			if err != nil {
				http.Error(w, "bad multipart", http.StatusBadRequest)
				return
			}
			body, _ = io.ReadAll(file)
			_ = file.Close()
			if header.Filename != "batch.jsonl" || r.FormValue("purpose") != "batch" {
				http.Error(w, "wrong upload", http.StatusBadRequest)
				return
			}
		} else {
			body, _ = io.ReadAll(r.Body)
		}
		requests <- r.Method + " " + r.URL.Path + " " + string(body)
		switch r.URL.Path {
		case "/v1/files":
			writeBatchUploadFixture(w)
		case "/v1/batches":
			writeBatchFixture(w, "validating")
		case "/v1/files/file-errors/content":
			_, _ = fmt.Fprintln(w, batchExpiredLine)
			_, _ = fmt.Fprintln(w, batchErrorLine)
		case "/v1/files/file-output/content":
			_, _ = fmt.Fprintln(w, batchSuccessLine)
		default:
			writeBatchFixture(w, "completed")
		}
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	// Act.
	submitted, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	document, err := prompty.EncodeBatchHandle(*submitted.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeBatchHandle(document)
	require.NoError(t, err)
	// Generation-only required fields must not prevent observing accepted work.
	manager.base.profiles[0].ModelOptionsSchema = []byte(`{"type":"object","required":["max_tokens"]}`)
	inspected, err := manager.Get(t.Context(), handle)
	require.NoError(t, err)
	canceled, err := manager.Cancel(t.Context(), *inspected.Handle)
	require.NoError(t, err)
	result, err := manager.Results(t.Context(), *canceled.Handle, batchFixtureBindings(input))
	// Assert.
	require.NoError(t, err)
	require.True(t, submitted.SubmissionAttempted)
	require.Equal(t, prompty.BatchQueued, submitted.Handle.Status)
	require.Equal(t, "validating", submitted.Handle.ProviderStatus)
	require.Equal(t, prompty.BatchEnded, result.Handle.Status)
	require.True(t, result.Completeness.DownloadComplete)
	require.True(t, result.Completeness.CorrelationComplete)
	require.Len(t, result.Items, 3)
	require.Equal(t, prompty.BatchItemExpired, result.Items[0].Outcome)
	require.Equal(t, prompty.BatchItemError, result.Items[1].Outcome)
	require.Equal(t, prompty.BatchItemSuccess, result.Items[2].Outcome)
	require.Equal(t, 7, result.Items[2].Response.Usage.TotalTokens)
	upload, submit := <-requests, <-requests
	require.Contains(t, upload, "POST /v1/files ")
	require.Contains(t, upload, `"custom_id":"one"`)
	require.Contains(t, upload, `"url":"/v1/chat/completions"`)
	require.Contains(t, submit, `"input_file_id":"file-input"`)
	require.Contains(t, submit, `"completion_window":"24h"`)
	require.Contains(t, <-requests, "GET /v1/batches/batch_fixture ")
	require.Contains(t, <-requests, "POST /v1/batches/batch_fixture/cancel ")
	require.Contains(t, <-requests, "GET /v1/files/file-errors/content ")
	require.Contains(t, <-requests, "GET /v1/files/file-output/content ")
}

func TestBatchesPartialAndDuplicateFilesKeepKnownStatus(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"truncated", "transport", "duplicate", "missing"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/files":
					writeBatchUploadFixture(w)
				case "/v1/files/file-errors/content":
					_, _ = fmt.Fprintln(w, batchExpiredLine)
					_, _ = fmt.Fprintln(w, batchErrorLine)
				case "/v1/files/file-output/content":
					lines := batchSuccessLine + "\n"
					switch mode {
					case "truncated":
						lines += `{"custom_id":`
					case "transport":
						w.Header().Set("Content-Length", strconv.Itoa(len(lines)+10))
					case "duplicate":
						lines += batchSuccessLine + "\n"
					case "missing":
						lines = ""
					}
					_, _ = io.WriteString(w, lines)
				default:
					writeBatchFixture(w, "completed")
				}
			}))
			defer server.Close()
			manager, input := batchFixtureManager(server), batchFixtureInput()
			submitted, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			// Act.
			result, err := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
			// Assert.
			require.ErrorIs(t, err, prompty.ErrBatchResults)
			require.Equal(t, prompty.BatchEnded, result.Handle.Status)
			require.False(t, result.Completeness.CorrelationComplete)
			require.Equal(t, mode == "duplicate" || mode == "missing", result.Completeness.DownloadComplete)
			require.GreaterOrEqual(t, len(result.Items), 2)
		})
	}
}

func TestBatchesUnknownUploadAndSubmissionNeverRetry(t *testing.T) {
	t.Parallel()
	for _, phase := range []string{"upload", "submit"} {
		t.Run(phase, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var uploads, submits atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v1/files" {
					uploads.Add(1)
					if phase == "submit" {
						writeBatchUploadFixture(w)
						return
					}
				} else {
					submits.Add(1)
				}
				http.Error(w, "private-sdk-error", http.StatusInternalServerError)
			}))
			defer server.Close()
			manager := batchFixtureManager(server)
			// Act.
			result, err := manager.Submit(t.Context(), batchFixtureInput())
			// Assert.
			require.NotContains(t, err.Error(), "private-sdk-error")
			require.Equal(t, int32(1), uploads.Load())
			if phase == "upload" {
				require.ErrorIs(t, err, prompty.ErrBatchFileUnknown)
				require.Zero(t, submits.Load())
				require.False(t, result.SubmissionAttempted)
			} else {
				require.ErrorIs(t, err, prompty.ErrBatchUnknown)
				require.Equal(t, int32(1), submits.Load())
				require.True(t, result.SubmissionAttempted)
				require.Equal(t, "file-input", result.Resources[0].ProviderID)
			}
		})
	}
}

func TestBatchesInvalidIDsAndBindingsDoNotUpload(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeBatchUploadFixture(w) }),
	)
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	input.Items[1].ID = input.Items[0].ID
	// Act.
	_, duplicateErr := manager.Submit(t.Context(), input)
	manager.files = NewBatchFiles("other-account", server.URL+"/v1", manager.base.client)
	_, scopeErr := manager.Submit(t.Context(), batchFixtureInput())
	// Assert.
	require.ErrorIs(t, duplicateErr, prompty.ErrBatch)
	require.ErrorIs(t, scopeErr, prompty.ErrBatchScope)
	require.Zero(t, calls.Load())
}

func TestBatchesResourceCloneDoesNotAliasSource(t *testing.T) {
	t.Parallel()
	// Arrange.
	handle := prompty.BatchHandle{Resources: []prompty.BatchResourceRef{{Kind: "input", ProviderID: "file-input"}}}
	// Act.
	cloned := prompty.CloneBatchHandle(handle)
	cloned.Resources[0].ProviderID = "changed"
	// Assert.
	require.Equal(t, "file-input", handle.Resources[0].ProviderID)
	require.NotContains(t, handle.Resources[0].ProviderID, "changed")
}

func TestBatchesOriginalSchemaNumericFidelityAndRefusal(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"valid", "invalid", "refusal"} {
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			text := `{\"value\":9007199254740993}`
			if mode == "invalid" {
				text = `{\"value\":9007199254740992}`
			}
			line := strings.Replace(batchSuccessLine, `"content":"answer"`, `"content":"`+text+`"`, 1)
			if mode == "refusal" {
				line = strings.Replace(line, `"role":"assistant"`, `"role":"assistant","refusal":"cannot comply"`, 1)
			}
			uploads := make(chan string, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/v1/files":
					file, _, err := r.FormFile("file")
					if err != nil {
						http.Error(w, "multipart", http.StatusBadRequest)
						return
					}
					body, _ := io.ReadAll(file)
					_ = file.Close()
					uploads <- string(body)
					writeBatchUploadFixture(w)
				case "/v1/files/file-errors/content":
					return
				case "/v1/files/file-output/content":
					_, _ = fmt.Fprintln(w, line)
				default:
					writeBatchFixture(w, "completed")
				}
			}))
			defer server.Close()
			manager, input := batchFixtureManager(server), batchFixtureInput()
			input.Items = input.Items[:1]
			input.Items[0].Execution.ResponseFormat = &prompty.SchemaDefinition{
				Name: "extraction",
				Schema: []byte(
					`{"type":"object","properties":{"value":{"type":"integer","enum":[9007199254740993]}},"required":["value"],"additionalProperties":false}`,
				),
			}
			submitted, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			// Act.
			result, err := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
			// Assert: download/correlation succeeds independently from validation.
			require.NoError(t, err)
			require.True(t, result.Completeness.CorrelationComplete)
			require.Contains(t, <-uploads, `"enum":[9007199254740993]`)
			require.Equal(t, 7, result.Items[0].Response.Usage.TotalTokens)
			switch mode {
			case "valid":
				require.Equal(t, prompty.BatchItemSuccess, result.Items[0].Outcome)
			case "invalid":
				require.Equal(t, prompty.BatchItemIncomplete, result.Items[0].Outcome)
			case "refusal":
				require.Equal(t, prompty.BatchItemRefusal, result.Items[0].Outcome)
			}
		})
	}
}

func TestBatchesUnknownAcceptedIdentityIsReconcilable(t *testing.T) {
	t.Parallel()
	// Arrange.
	var submissions atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/files" {
			writeBatchUploadFixture(w)
			return
		}
		submissions.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"batch_accepted","object":"batch","endpoint":"/v1/chat/completions","input_file_id":"file-input","status":"future_unknown"}`,
		)
	}))
	defer server.Close()
	manager := batchFixtureManager(server)
	// Act.
	result, err := manager.Submit(t.Context(), batchFixtureInput())
	// Assert.
	require.ErrorIs(t, err, prompty.ErrBatchUnknown)
	require.True(t, result.SubmissionAttempted)
	require.Equal(t, "batch_accepted", result.Handle.ProviderID)
	require.Equal(t, prompty.BatchUnknown, result.Handle.Status)
	require.Equal(t, "file-input", result.Resources[0].ProviderID)
	require.Equal(t, int32(1), submissions.Load())
}

func TestBatchesNativeTruncationExpiryAndUnknownUsage(t *testing.T) {
	t.Parallel()
	// Arrange: provider length truncation, expired item and success without usage.
	truncated := strings.Replace(batchSuccessLine, `"finish_reason":"stop"`, `"finish_reason":"length"`, 1)
	unknownUsage := strings.Replace(batchSuccessLine, `"custom_id":"one"`, `"custom_id":"two"`, 1)
	unknownUsage = strings.Replace(
		unknownUsage,
		`,"usage":{"prompt_tokens":5,"completion_tokens":2,"total_tokens":7,"prompt_tokens_details":{"cached_tokens":2}}`,
		"",
		1,
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		switch request.URL.Path {
		case "/v1/files":
			writeBatchUploadFixture(w)
		case "/v1/files/file-errors/content":
			_, _ = fmt.Fprintln(w, batchExpiredLine)
		case "/v1/files/file-output/content":
			_, _ = fmt.Fprintln(w, unknownUsage)
			_, _ = fmt.Fprintln(w, truncated)
		default:
			writeBatchFixture(w, "completed")
		}
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	created, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	// Act.
	result, err := manager.Results(t.Context(), *created.Handle, batchFixtureBindings(input))
	// Assert: native outcome and independently absent usage are preserved.
	require.NoError(t, err)
	require.True(t, result.Completeness.CorrelationComplete)
	require.True(t, result.Completeness.DownloadComplete)
	require.Len(t, result.Items, 3)
	require.Equal(t, prompty.BatchItemExpired, result.Items[0].Outcome)
	require.Nil(t, result.Items[0].Response)
	require.Equal(t, prompty.BatchItemSuccess, result.Items[1].Outcome)
	require.Equal(t, prompty.UsageUnknown, result.Items[1].Response.Usage.Availability())
	require.Empty(t, result.Items[1].Response.Usage.Known)
	require.Equal(t, prompty.BatchItemIncomplete, result.Items[2].Outcome)
	require.Equal(t, "length", result.Items[2].Response.FinishReason)
	require.Equal(t, prompty.OutcomeIncomplete, result.Items[2].Response.Outcome)
	require.Equal(t, 7, result.Items[2].Response.Usage.TotalTokens)
}
