package gemini

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log" //nolint:depguard // Characterize the pinned SDK process logger; slog cannot capture it.
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

// SDK-GENAI-BATCH-DEST: the pinned SDK reads metadata.output, not REST response.
// This passing characterization is evidence of the dependency limitation.
func TestSDKBatchRESTDestinationUnavailableCharacterization(t *testing.T) {
	t.Parallel()
	// Arrange: identical public operation and two provider output representations.
	for _, output := range []struct {
		body      string
		available bool
	}{
		{`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED"},"response":{"responsesFile":"files/output"}}`, false},
		{`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/output"}}}`, true},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = io.WriteString(w, output.body)
		}))
		sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
			APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
		})
		require.NoError(t, err)
		// Act: official public decoder, no adapter body observer.
		job, callErr := sdk.Batches.Get(t.Context(), "batches/fixture", nil)
		server.Close()
		// Assert: identity/state survive; destination is absent for the REST form.
		require.NoError(t, callErr)
		require.Equal(t, "batches/fixture", job.Name)
		require.Equal(t, genai.JobStateSucceeded, job.State)
		require.Equal(t, output.available, job.Dest != nil)
		if output.available {
			require.Equal(t, "files/output", job.Dest.FileName)
		}
	}
}

// SDK-GENAI-LOG: this SDK uses the process standard logger for scanner errors.
// Only the test temporarily captures that logger; production never reconfigures it.
func TestSDKScannerLoggingCharacterization(t *testing.T) {
	// Arrange: the top-level test is sequential, before parked parallel tests run.
	var logged bytes.Buffer
	previous := log.Writer()
	log.SetOutput(&logged)
	defer log.SetOutput(previous)
	private := errors.New("private-host-read-error")
	body := &sourceFailureBody{reader: strings.NewReader("data: {}\n\n"), err: private}
	sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{
		APIKey: "fixture", Backend: genai.BackendGeminiAPI,
		HTTPClient: &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: body}, nil
		})}, HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid", APIVersion: "v1beta"},
	})
	require.NoError(t, err)
	// Act: consume only the official iterator.
	var yielded error
	for _, err := range sdk.Models.GenerateContentStream(context.Background(), "fixture", genai.Text("fixture"), nil) {
		if err != nil {
			yielded = err
		}
	}
	// Assert: dependency logging remains observable and body ownership remains SDK-owned.
	require.ErrorIs(t, yielded, private)
	require.Contains(t, logged.String(), "private-host-read-error")
	require.True(t, body.closed.Load())
}
