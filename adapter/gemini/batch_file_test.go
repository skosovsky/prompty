package gemini

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type batchFixtureFiles struct {
	binding   prompty.BatchFileBinding
	input     string
	output    string
	status    int
	uploads   int
	downloads int
	closes    int
	closeErr  error
	uploadErr error
}

func (f *batchFixtureFiles) Binding() prompty.BatchFileBinding { return f.binding }

func (f *batchFixtureFiles) Upload(
	_ context.Context,
	input prompty.BatchFileUpload,
) (*prompty.BatchFileUploadResult, error) {
	f.uploads++
	data, err := io.ReadAll(input.Content)
	if err != nil {
		return nil, err
	}
	f.input = string(data)
	return &prompty.BatchFileUploadResult{Resource: prompty.BatchResourceRef{Kind: "input", ProviderID: "files/input"},
		SourceJSON: prompty.JSONDocument(`{"name":"files/input"}`), HTTPStatus: http.StatusOK}, f.uploadErr
}

func (f *batchFixtureFiles) Download(
	_ context.Context,
	_ prompty.BatchResourceRef,
) (*prompty.BatchFileDownload, error) {
	f.downloads++
	return &prompty.BatchFileDownload{
		Body:       &batchFixtureFileBody{Reader: strings.NewReader(f.output), files: f},
		HTTPStatus: f.status,
	}, nil
}

type batchFixtureFileBody struct {
	io.Reader

	files *batchFixtureFiles
}

func (b *batchFixtureFileBody) Close() error { b.files.closes++; return b.files.closeErr }

func TestFileBatchActualSDKSubmitInspectAndResults(t *testing.T) {
	t.Parallel()
	// Arrange: SDK job operations and an explicitly injected host file port.
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		calls = append(calls, r.Method+" "+r.URL.Path+" "+string(body))
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
			return
		}
		_, _ = io.WriteString(
			w,
			`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/output"}}}`,
		)
	}))
	defer server.Close()
	base, _ := batchOperationFixture(t, server)
	files := &batchFixtureFiles{
		binding: prompty.BatchFileBinding{Provider: "gemini", Endpoint: server.URL, AccountBinding: "account-fixture"},
		status:  http.StatusOK,
		output: "{\"key\":\"three\",\"response\":{\"promptFeedback\":{\"blockReason\":\"SAFETY\"}}}\n" +
			"{\"key\":\"two\",\"error\":{\"code\":3,\"message\":\"private\"}}\n" +
			"{\"key\":\"one\",\"extra\":9007199254740993,\"response\":{\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"label\"}]},\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":2,\"candidatesTokenCount\":1,\"totalTokenCount\":3}}}\n",
	}
	batches := NewFileBatches(
		"account-fixture",
		files,
		BatchFileOptions{},
		WithClient(base.client),
		WithModel("fixture"),
		WithProfiles(server.URL, base.profiles...),
	)
	input, options := inlineBatchFixtureInputs()
	// Act.
	created, err := batches.Submit(context.Background(), input)
	require.NoError(t, err)
	inspected, err := batches.Get(context.Background(), *created.Handle)
	require.NoError(t, err)
	wire, err := prompty.EncodeBatchHandle(*inspected.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeBatchHandle(wire)
	require.NoError(t, err)
	result, err := batches.Results(context.Background(), handle, options)
	// Assert: no real GenerateContent call or implicit polling, exact original source.
	require.NoError(t, err)
	require.Len(t, calls, 2)
	require.Contains(t, calls[0], ":batchGenerateContent")
	require.Contains(t, calls[0], `"fileName":"files/input"`)
	require.Contains(t, files.input, `"key":"one"`)
	require.Contains(t, files.input, `"contents"`)
	require.Contains(t, files.input, `"text":"classify one"`)
	require.Len(t, strings.Split(strings.TrimSpace(files.input), "\n"), 3)
	require.True(t, result.Completeness.DownloadComplete)
	require.True(t, result.Completeness.CorrelationComplete)
	require.Equal(t, prompty.BatchItemRefusal, result.Items[0].Outcome)
	require.Equal(t, prompty.BatchItemError, result.Items[1].Outcome)
	require.Equal(t, prompty.BatchItemSuccess, result.Items[2].Outcome)
	require.Equal(t, 3, result.Items[2].Response.Usage.TotalTokens)
	require.Contains(t, string(result.Items[2].SourceJSON), `"extra":9007199254740993`)
	require.Equal(t, 1, files.uploads)
	require.Equal(t, 1, files.downloads)
	require.Equal(t, 1, files.closes)
}

func TestFileBatchDownloadFailuresRetainKnownPrefix(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, suffix string
		status       int
		closeErr     error
		maxBytes     int64
		maxLine      int
		wantItems    int
		complete     bool
	}{
		{name: "malformed suffix", suffix: "{bad}\n", status: 200, wantItems: 1},
		{name: "close failure", status: 200, closeErr: errors.New("private close"), wantItems: 1},
		{name: "status failure", status: 403},
		{name: "total bound", status: 200, maxBytes: 10},
		{name: "line bound", status: 200, maxLine: 10},
		{name: "missing ids", status: 200, wantItems: 1, complete: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: a persisted output reference; reading must not inspect the job.
			server := httptest.NewServer(
				http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { t.Error("unexpected job request") }),
			)
			defer server.Close()
			base, _ := batchOperationFixture(t, server)
			input, options := inlineBatchFixtureInputs()
			handle := prompty.BatchHandle{
				Format:         prompty.BatchWireFormat,
				Provider:       "gemini",
				Endpoint:       server.URL,
				AccountBinding: "account-fixture",
				ProviderID:     "batches/fixture",
				Status:         prompty.BatchEnded,
				Resources:      []prompty.BatchResourceRef{{Kind: "output", ProviderID: "files/output"}},
			}
			for _, item := range input.Items {
				original := item.Execution.Clone()
				original.ModelOptions = &prompty.ModelOptions{Model: "fixture"}
				digest, err := prompty.BatchExecutionDigest(original)
				require.NoError(t, err)
				handle.Items = append(
					handle.Items,
					prompty.BatchItemBinding{
						ID:              item.ID,
						OriginalID:      item.ID,
						AttemptID:       item.AttemptID,
						Identity:        base.profiles[0].Identity,
						ExecutionDigest: digest,
					},
				)
			}
			files := &batchFixtureFiles{
				binding: prompty.BatchFileBinding{
					Provider:       "gemini",
					Endpoint:       server.URL,
					AccountBinding: "account-fixture",
				},
				status:   tc.status,
				closeErr: tc.closeErr,
				output:   "{\"key\":\"one\",\"error\":{\"code\":3}}\n" + tc.suffix,
			}
			batches := NewFileBatches(
				"account-fixture",
				files,
				BatchFileOptions{},
				WithClient(base.client),
				WithModel("fixture"),
				WithProfiles(server.URL, base.profiles...),
			)
			options.MaxBytes, options.MaxLineBytes = tc.maxBytes, tc.maxLine
			// Act.
			result, err := batches.Results(context.Background(), handle, options)
			// Assert: body closes once and correlation is separate from download.
			if tc.complete {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
			require.Len(t, result.Items, tc.wantItems)
			require.Equal(t, tc.complete, result.Completeness.DownloadComplete)
			require.False(t, result.Completeness.CorrelationComplete)
			require.Equal(t, "batches/fixture", result.Handle.ProviderID)
			require.Equal(t, 1, files.closes)
		})
	}
}

func TestFileBatchPreflightAndUnknownJobDoNotReplay(t *testing.T) {
	t.Parallel()
	// Arrange: a known upload followed by unknown remote job acceptance.
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = io.WriteString(w, `{"error":{"code":500,"message":"private"}}`)
	}))
	defer server.Close()
	base, _ := batchOperationFixture(t, server)
	files := &batchFixtureFiles{
		binding: prompty.BatchFileBinding{Provider: "gemini", Endpoint: server.URL, AccountBinding: "account-fixture"},
	}
	opts := []Option{WithClient(base.client), WithModel("fixture"), WithProfiles(server.URL, base.profiles...)}
	input, _ := inlineBatchFixtureInputs()
	// Act/Assert: explicit file selection never falls back to inline generation.
	nilPort := NewFileBatches("account-fixture", nil, BatchFileOptions{}, opts...)
	_, err := nilPort.Submit(context.Background(), input)
	require.Error(t, err)
	wrongPort := NewFileBatches("other-account", files, BatchFileOptions{}, opts...)
	_, err = wrongPort.Submit(context.Background(), input)
	require.ErrorIs(t, err, prompty.ErrBatchScope)
	bounded := NewFileBatches("account-fixture", files, BatchFileOptions{MaxInputBytes: 10, MaxLineBytes: 10}, opts...)
	_, err = bounded.Submit(context.Background(), input)
	require.Error(t, err)
	batches := NewFileBatches("account-fixture", files, BatchFileOptions{}, opts...)
	duplicate := input
	duplicate.Items = append([]prompty.BatchItem(nil), input.Items...)
	duplicate.Items[1].ID = duplicate.Items[0].ID
	_, err = batches.Submit(context.Background(), duplicate)
	require.Error(t, err)
	require.Zero(t, files.uploads)
	require.Zero(t, calls.Load())
	result, err := batches.Submit(context.Background(), input)
	require.ErrorIs(t, err, prompty.ErrBatchUnknown)
	require.Nil(t, result.Handle)
	require.True(t, result.SubmissionAttempted)
	require.Equal(t, []prompty.BatchResourceRef{{Kind: "input", ProviderID: "files/input"}}, result.Resources)
	require.Equal(t, 1, files.uploads)
	require.EqualValues(t, 1, calls.Load())
	// Upload failure likewise retains its known resource without submitting a job.
	files.uploadErr = errors.New("private upload failure")
	result, err = batches.Submit(context.Background(), input)
	require.ErrorIs(t, err, prompty.ErrBatchFileUnknown)
	require.NotContains(t, err.Error(), "private")
	require.False(t, result.SubmissionAttempted)
	require.Len(t, result.Resources, 1)
	require.EqualValues(t, 1, calls.Load())
}
