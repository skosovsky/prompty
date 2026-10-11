package anthropic

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func batchFixtureManager(server *httptest.Server) *MessageBatches {
	client := sdk.NewClient(
		option.WithBaseURL(server.URL),
		option.WithAPIKey("fixture-secret"),
		option.WithHTTPClient(server.Client()),
	)
	profile := prompty.CapabilityProfile{
		Identity:   prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"},
		Provenance: "local SDK batch fixture, not live model verification",
		Capabilities: map[string]prompty.CapabilitySupport{
			prompty.CapabilityBatchSubmit:      prompty.CapabilitySupported,
			prompty.CapabilityBatchInspect:     prompty.CapabilitySupported,
			prompty.CapabilityBatchCancel:      prompty.CapabilitySupported,
			prompty.CapabilityBatchResults:     prompty.CapabilitySupported,
			prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
		},
	}
	return NewMessageBatches(
		"host-account",
		WithModel("fixture"),
		WithClient(&client),
		WithProfiles(server.URL, profile),
	)
}

func batchFixtureInput() prompty.BatchSubmitRequest {
	input := prompty.BatchSubmitRequest{}
	for _, id := range []string{"one", "two", "three"} {
		input.Items = append(
			input.Items,
			prompty.BatchItem{ID: id, AttemptID: "first", Execution: prompty.SimplePrompt("document-" + id)},
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

func writeBatchFixture(w http.ResponseWriter, status string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(
		w,
		`{"id":"msgbatch_fixture","type":"message_batch","processing_status":%q,"request_counts":{"processing":0,"succeeded":1,"errored":1,"expired":1,"canceled":0}}`,
		status,
	)
}

const batchSuccessLine = `{"custom_id":"one","result":{"type":"succeeded","message":{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}}}`
const batchExpiredLine = `{"custom_id":"three","result":{"type":"expired"}}`

const batchErrorLine = `{"custom_id":"two","result":{"type":"errored","error":{"type":"error","error":{"type":"invalid_request_error","message":"fixture-private"}}}}`

func TestMessageBatchesActualSDKReverseOrderLifecycle(t *testing.T) {
	t.Parallel()
	// Arrange.
	requests := make(chan string, 4)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		requests <- r.Method + " " + r.URL.Path + " " + string(body)
		if strings.HasSuffix(r.URL.Path, "/results") {
			_, _ = fmt.Fprintln(w, batchExpiredLine)
			_, _ = fmt.Fprintln(w, batchErrorLine)
			_, _ = fmt.Fprintln(w, batchSuccessLine)
			return
		}
		status := "ended"
		if r.URL.Path == "/v1/messages/batches" {
			status = "in_progress"
		}
		writeBatchFixture(w, status)
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
	require.Equal(t, prompty.BatchEnded, result.Handle.Status)
	require.True(t, result.Completeness.DownloadComplete)
	require.True(t, result.Completeness.CorrelationComplete)
	require.Len(t, result.Items, 3)
	require.Equal(t, prompty.BatchItemExpired, result.Items[0].Outcome)
	require.Equal(t, prompty.BatchItemError, result.Items[1].Outcome)
	require.Equal(t, prompty.BatchItemSuccess, result.Items[2].Outcome)
	require.Equal(t, "three", result.Items[0].Binding.OriginalID)
	require.Equal(t, 7, result.Items[2].Response.Usage.TotalTokens)
	require.Contains(t, <-requests, "POST /v1/messages/batches ")
	require.Contains(t, <-requests, "GET /v1/messages/batches/msgbatch_fixture ")
	require.Contains(t, <-requests, "POST /v1/messages/batches/msgbatch_fixture/cancel ")
	require.Contains(t, <-requests, "GET /v1/messages/batches/msgbatch_fixture/results ")
}

func TestMessageBatchesCorrelationGapsAndPartialDownload(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name, lines      string
		downloadComplete bool
	}{
		{"missing", batchSuccessLine + "\n", true},
		{"duplicate", batchSuccessLine + "\n" + batchSuccessLine + "\n" + batchExpiredLine + "\n" + batchErrorLine + "\n", true},
		{"truncated", batchSuccessLine + "\n" + `{"custom_id":`, false},
		{"transport", batchSuccessLine + "\n" + batchExpiredLine + "\n" + batchErrorLine + "\n", false},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/results") {
					if fixture.name == "transport" {
						w.Header().Set("Content-Length", strconv.Itoa(len(fixture.lines)+10))
					}
					_, _ = io.WriteString(w, fixture.lines)
					return
				}
				writeBatchFixture(w, "ended")
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
			require.Equal(t, fixture.downloadComplete, result.Completeness.DownloadComplete)
			require.False(t, result.Completeness.CorrelationComplete)
			require.NotEmpty(t, result.Items)
			require.Equal(t, "one", result.Items[0].ID)
		})
	}
}

func TestSDKMessageBatchIteratorLargeLinesAndReadErrors(t *testing.T) {
	t.Parallel()
	for _, limited := range []bool{false, true} {
		t.Run(strconv.FormatBool(limited), func(t *testing.T) {
			t.Parallel()
			// Arrange: the pinned SDK supports >64 KiB records; adapter line policy remains explicit.
			large := strings.Replace(batchSuccessLine, "answer", strings.Repeat("a", 70000), 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/results") {
					_, _ = fmt.Fprintln(w, batchExpiredLine)
					_, _ = fmt.Fprintln(w, large)
					_, _ = fmt.Fprintln(w, batchErrorLine)
					return
				}
				writeBatchFixture(w, "ended")
			}))
			defer server.Close()
			manager, input := batchFixtureManager(server), batchFixtureInput()
			submitted, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			options := batchFixtureBindings(input)
			if limited {
				options.MaxLineBytes = 65536
			}
			// Act.
			result, err := manager.Results(t.Context(), *submitted.Handle, options)
			// Assert.
			if limited {
				require.ErrorIs(t, err, prompty.ErrBatchResults)
				require.Len(t, result.Items, 1)
				require.False(t, result.Completeness.DownloadComplete)
			} else {
				require.NoError(t, err)
				require.True(t, result.Completeness.CorrelationComplete)
				require.Len(t, result.Items, 3)
				require.Len(t, result.Items[1].Response.Content[0].(prompty.TextPart).Text, 70000)
			}
			require.Equal(t, prompty.BatchEnded, result.Handle.Status)
		})
	}
}

func TestMessageBatchesUnknownAndPreflightDoNotRetry(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		http.Error(w, "fixture-secret", http.StatusInternalServerError)
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	invalid := batchFixtureInput()
	invalid.Items[1].ID = invalid.Items[0].ID
	// Act.
	_, invalidErr := manager.Submit(t.Context(), invalid)
	require.Zero(t, calls.Load())
	_, err := manager.Submit(context.Background(), input)
	// Assert.
	require.ErrorIs(t, invalidErr, prompty.ErrBatch)
	require.ErrorIs(t, err, prompty.ErrBatchUnknown)
	require.NotContains(t, err.Error(), "fixture-secret")
	require.Equal(t, int32(1), calls.Load())
}

func TestMessageBatchesOriginalSchemaAndRefusalArePerItem(t *testing.T) {
	t.Parallel()
	for _, outcome := range []string{"invalid schema", "refusal"} {
		t.Run(outcome, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			line := strings.Replace(batchSuccessLine, `"text":"answer"`, `"text":"{\"value\":\"wrong\"}"`, 1)
			if outcome == "refusal" {
				line = strings.Replace(line, `"stop_reason":"end_turn"`, `"stop_reason":"refusal"`, 1)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/results") {
					_, _ = fmt.Fprintln(w, line)
					return
				}
				writeBatchFixture(w, "ended")
			}))
			defer server.Close()
			manager, input := batchFixtureManager(server), batchFixtureInput()
			input.Items = input.Items[:1]
			input.Items[0].Execution.ResponseFormat = &prompty.SchemaDefinition{
				Name: "extraction",
				Schema: []byte(
					`{"type":"object","properties":{"value":{"type":"string","enum":["allowed"]}},"required":["value"],"additionalProperties":false}`,
				),
			}
			submitted, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			// Act.
			result, err := manager.Results(t.Context(), *submitted.Handle, batchFixtureBindings(input))
			// Assert: retrieval can be complete without checked item success.
			require.NoError(t, err)
			require.True(t, result.Completeness.CorrelationComplete)
			require.Len(t, result.Items, 1)
			require.Equal(t, 7, result.Items[0].Response.Usage.TotalTokens)
			if outcome == "refusal" {
				require.Equal(t, prompty.BatchItemRefusal, result.Items[0].Outcome)
			} else {
				require.Equal(t, prompty.BatchItemIncomplete, result.Items[0].Outcome)
			}
		})
	}
}

func TestMessageBatchesChangedResultContractAndForeignHandleFailBeforeIO(t *testing.T) {
	t.Parallel()
	// Arrange.
	var calls atomic.Int32
	server := httptest.NewServer(
		http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); writeBatchFixture(w, "ended") }),
	)
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	submitted, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	foreign := prompty.CloneBatchHandle(*submitted.Handle)
	foreign.AccountBinding = "other-host"
	options := batchFixtureBindings(input)
	options.Bindings[0].Execution = prompty.SimplePrompt("changed document")
	// Act.
	_, scopeErr := manager.Get(t.Context(), foreign)
	_, bindingErr := manager.Results(t.Context(), *submitted.Handle, options)
	// Assert.
	require.ErrorIs(t, scopeErr, prompty.ErrBatchScope)
	require.ErrorIs(t, bindingErr, prompty.ErrBatchScope)
	require.Equal(t, int32(1), calls.Load())
}

func TestMessageBatchesNativeTruncationCancellationAndUnknownUsage(t *testing.T) {
	t.Parallel()
	// Arrange: transport success is independent of provider generation completion.
	truncated := strings.Replace(batchSuccessLine, `"stop_reason":"end_turn"`, `"stop_reason":"max_tokens"`, 1)
	unknownUsage := strings.Replace(batchSuccessLine, `"custom_id":"one"`, `"custom_id":"three"`, 1)
	unknownUsage = strings.Replace(
		unknownUsage,
		`,"usage":{"input_tokens":5,"output_tokens":2,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`,
		"",
		1,
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		if strings.HasSuffix(request.URL.Path, "/results") {
			_, _ = fmt.Fprintln(w, unknownUsage)
			_, _ = fmt.Fprintln(w, `{"custom_id":"two","result":{"type":"canceled"}}`)
			_, _ = fmt.Fprintln(w, truncated)
			return
		}
		writeBatchFixture(w, "ended")
	}))
	defer server.Close()
	manager, input := batchFixtureManager(server), batchFixtureInput()
	created, err := manager.Submit(t.Context(), input)
	require.NoError(t, err)
	// Act.
	result, err := manager.Results(t.Context(), *created.Handle, batchFixtureBindings(input))
	// Assert: all IDs are retrieved, but not all generations succeeded.
	require.NoError(t, err)
	require.True(t, result.Completeness.CorrelationComplete)
	require.True(t, result.Completeness.DownloadComplete)
	require.Len(t, result.Items, 3)
	require.Equal(t, prompty.BatchItemSuccess, result.Items[0].Outcome)
	require.Equal(t, prompty.UsageUnknown, result.Items[0].Response.Usage.Availability())
	require.Empty(t, result.Items[0].Response.Usage.Known)
	require.Equal(t, prompty.BatchItemCanceled, result.Items[1].Outcome)
	require.Nil(t, result.Items[1].Response)
	require.Equal(t, prompty.BatchItemIncomplete, result.Items[2].Outcome)
	require.Equal(t, "max_tokens", result.Items[2].Response.FinishReason)
	require.Equal(t, prompty.OutcomeIncomplete, result.Items[2].Response.Outcome)
	require.Equal(t, 7, result.Items[2].Response.Usage.TotalTokens)
}

// SDK-MUTATION-ID: bytes containing an ID are not SDK-returned identity evidence.
func TestSDKMalformedBatchIdentityUnavailableCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: accepted response cannot be decoded by the pinned official SDK.
	var attempts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		attempts.Add(1)
		_, _ = io.WriteString(w, `{"id":"msgbatch_accepted",`)
	}))
	defer server.Close()
	// Act.
	result, err := batchFixtureManager(server).Submit(t.Context(), batchFixtureInput())
	// Assert: unknown acceptance, no body recovery, no retry.
	require.ErrorIs(t, err, prompty.ErrBatchUnknown)
	require.True(t, result.SubmissionAttempted)
	require.Nil(t, result.Handle)
	require.Equal(t, int32(1), attempts.Load())
}
