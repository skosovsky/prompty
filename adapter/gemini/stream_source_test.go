package gemini

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/genai"
)

func TestStreamSourceSDKReadAheadAndConsumerBreak(t *testing.T) {
	t.Parallel()
	// Arrange: all events fit in one transport read, but each callback must
	// observe only its own exact payload, not the SDK-rounded next event.
	first := `{"candidates":[{"content":{"parts":[{"text":"early"}]}}]}`
	second := `{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"n":9007199254740993}}}]},"finishReason":"STOP"}]}`
	for _, stop := range []bool{false, true} {
		t.Run(map[bool]string{false: "all", true: "break"}[stop], func(t *testing.T) {
			t.Parallel()
			body := &streamSourceFixtureBody{
				Reader: strings.NewReader("data: " + first + "\n\ndata: " + second + "\n\n"),
			}
			calls := 0
			sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
				APIKey: "fixture", Backend: genai.BackendGeminiAPI,
				HTTPClient: &http.Client{Transport: sourceFixtureTransport(func(*http.Request) (*http.Response, error) {
					calls++
					return &http.Response{
						StatusCode: http.StatusOK,
						Body:       body,
						Header:     http.Header{"Content-Type": {"text/event-stream"}},
					}, nil
				})},
				HTTPOptions: genai.HTTPOptions{BaseURL: "https://fixture.invalid", APIVersion: "v1beta"},
			})
			require.NoError(t, err)
			isolated := isolatedResponseClient(sdk)
			require.NotNil(t, isolated)
			source := &streamSource{limit: 1024}
			ctx := context.WithValue(t.Context(), streamSourceKey{}, source)
			var payloads []string
			// Act: use the real SDK scanner/converter and stop at the first callback.
			for response, callErr := range isolated.Models.GenerateContentStream(ctx, "fixture", []*genai.Content{
				{Role: "user", Parts: []*genai.Part{{Text: "fixture"}}},
			}, nil) {
				require.NoError(t, callErr)
				require.NotNil(t, response)
				payloads = append(payloads, string(source.payload))
				if stop {
					break
				}
			}
			// Assert: exact lexical number, one dispatch and transport closure.
			expected := []string{first, second}
			if stop {
				expected = expected[:1]
			}
			require.Equal(t, expected, payloads)
			require.Equal(t, 1, calls)
			require.Equal(t, 1, body.closes)
		})
	}
}

type streamSourceFixtureBody struct {
	*strings.Reader

	closes int
}

func (b *streamSourceFixtureBody) Close() error {
	b.closes++
	return nil
}

func TestStreamSourceRejectsInterruptedLine(t *testing.T) {
	t.Parallel()
	// Arrange: a read failure leaves an unaccepted partial data line.
	original := &sourceFailureBody{reader: strings.NewReader(`data: {"private":"partial"`)}
	source := &streamSource{limit: 1024}
	body := &streamSourceBody{ReadCloser: original, reader: bufio.NewReader(original), source: source}
	// Act.
	data, err := io.ReadAll(body)
	closeErr := body.Close()
	// Assert: preserve the transport failure without publishing partial source.
	require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	require.Empty(t, data)
	require.Empty(t, source.payload)
	require.NoError(t, closeErr)
	require.True(t, original.closed.Load())
}

func TestStreamSourceBoundAndSmallReads(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		line      string
		limit     int
		wantError bool
	}{
		{"exact", "data: {}\n", 9, false},
		{"excess", "data: {}\n", 8, true},
		{"unterminated", "data: {}", 8, false},
		{"invalid-bound", "data: {}", 0, true},
		{"large-line", "data: " + strings.Repeat("x", 5000) + "\n", 4096, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: force framing through one-byte SDK reads.
			source := &streamSource{limit: tc.limit}
			body := &streamSourceBody{ReadCloser: io.NopCloser(strings.NewReader(tc.line)),
				reader: bufio.NewReader(strings.NewReader(tc.line)), source: source}
			var received strings.Builder
			var terminal error
			// Act.
			for {
				var buf [1]byte
				n, err := body.Read(buf[:])
				received.Write(buf[:n])
				if err != nil {
					terminal = err
					break
				}
			}
			// Assert: no truncated payload or unchecked excess is delivered.
			if tc.wantError {
				require.ErrorIs(t, terminal, io.ErrShortBuffer)
				require.Empty(t, received.String())
				require.Empty(t, source.payload)
			} else {
				require.ErrorIs(t, terminal, io.EOF)
				require.Equal(t, tc.line, received.String())
				require.Equal(t, "{}", string(source.payload))
			}
		})
	}
}
