package openai

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const cleanupFixtureSSE = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"one\"}}]}\n\n" +
	"data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"two\"}}]}\n\n"

var errCleanupFixtureRead = errors.New("fixture transport read failure")

type cleanupErrorReader struct{}

func (cleanupErrorReader) Read([]byte) (int, error) { return 0, errCleanupFixtureRead }

type cleanupBody struct {
	io.Reader

	closed atomic.Int32
}

func (b *cleanupBody) Close() error {
	b.closed.Add(1)
	return nil
}

type cleanupTransport struct {
	body  *cleanupBody
	calls atomic.Int32
}

func (t *cleanupTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": {"text/event-stream"}},
		Body: t.body, Request: request}, nil
}

func cleanupFixtureClient(reader io.Reader) (prompty.Invoker, *cleanupTransport) {
	return cleanupFixtureClientModel(reader, openai.ChatModelGPT4o)
}

func cleanupFixtureClientModel(reader io.Reader, model string) (prompty.Invoker, *cleanupTransport) {
	var body cleanupBody
	body.Reader = reader
	var transport cleanupTransport
	transport.body = &body
	sdk := openai.NewClient(option.WithAPIKey("fixture"), option.WithBaseURL("https://fixture.invalid"),
		option.WithHTTPClient(&http.Client{Transport: &transport}), option.WithMaxRetries(3))
	var profile prompty.CapabilityProfile
	profile.Identity = prompty.ProfileIdentity{
		Provider: "openai",
		Endpoint: "https://fixture.invalid",
		Model:    model,
	}
	profile.Provenance = "SDK HTTP body conformance fixture"
	profile.Capabilities = map[string]prompty.CapabilitySupport{
		prompty.CapabilityNativeStream: prompty.CapabilitySupported,
	}
	return adapter.NewClient(
		New(WithModel(model), WithClient(&sdk), WithProfiles(profile.Identity.Endpoint, profile)),
	), &transport
}

//nolint:gocognit // Keep the complete ordered provider fixture and its assertions in one scenario.
func TestNativeSDKConsumerStopClosesBodyWithoutReplay(t *testing.T) {
	t.Parallel()
	for _, stop := range []string{"break", "cancel", "close"} {
		t.Run(stop, func(t *testing.T) {
			t.Parallel()
			// Arrange: the real SDK owns an HTTP body with unread content after the first delta.
			client, transport := cleanupFixtureClient(strings.NewReader(cleanupFixtureSSE))
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stream := client.ExecuteStream(ctx, prompty.SimplePrompt("fixture"), prompty.StreamNative)
			require.Zero(t, transport.calls.Load(), "handle creation must remain lazy")
			// Act.
			deltas, terminals := 0, 0
			for event, err := range stream.Events() {
				if event.Kind == prompty.StreamTerminal {
					terminals++
					require.ErrorIs(t, err, context.Canceled)
					continue
				}
				require.NoError(t, err)
				if len(event.Content) == 0 {
					continue
				}
				deltas++
				if stop == "break" {
					break
				}
				if stop == "cancel" {
					cancel()
				} else {
					require.NoError(t, stream.Close())
				}
			}
			// Assert: SDK cleanup runs even when the caller never exhausts the response.
			require.Equal(t, int32(1), transport.calls.Load())
			require.Positive(t, transport.body.closed.Load())
			require.Equal(t, 1, deltas)
			require.Equal(t, prompty.StreamCanceled, stream.Status().State)
			require.ErrorIs(t, stream.Status().Err, context.Canceled)
			require.Len(t, stream.Status().Result.Content, 1)
			if stop == "break" {
				require.Zero(t, terminals, "consumer false cannot be followed by another yield")
			} else {
				require.Equal(t, 1, terminals)
			}
		})
	}
}

func TestNativeSDKReadFailureAfterTwoDeltasPreservesPartialAndClosesBody(t *testing.T) {
	t.Parallel()
	// Arrange: transport fails after two exposed deltas, with SDK retries configured globally.
	client, transport := cleanupFixtureClient(
		io.MultiReader(strings.NewReader(cleanupFixtureSSE), cleanupErrorReader{}),
	)
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	// Act.
	deltas, terminals := 0, 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, adapter.ErrInvalidResponse)
		} else if len(event.Content) != 0 {
			deltas++
		}
	}
	// Assert: no second completion, no loss of known content, no cancellation misclassification.
	require.Equal(t, 2, deltas)
	require.Equal(t, 1, terminals)
	require.Equal(t, int32(1), transport.calls.Load())
	require.Positive(t, transport.body.closed.Load())
	require.Equal(t, prompty.StreamFailed, stream.Status().State)
	require.NotErrorIs(t, stream.Status().Err, context.Canceled)
	text, err := stream.Status().Result.StrictText()
	require.NoError(t, err)
	require.Equal(t, "onetwo", text)
}

func TestNativeSDKRejectsToolIdentityReusedAcrossIndices(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeToolIDReuseConformance)
}

func nativeToolIDReuseConformance(t *testing.T, model string) {
	t.Helper()
	// Arrange: two logical calls share an ID and would otherwise merge into one valid object.
	sse := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"same","function":{"name":"lookup","arguments":""}},{"index":1,"id":"same","function":{"name":"lookup","arguments":"{}"}}]}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"
	client, transport := cleanupFixtureClientModel(strings.NewReader(sse), model)
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		if event.Kind == prompty.StreamTerminal {
			terminals++
			require.ErrorIs(t, err, adapter.ErrInvalidResponse)
		}
	}
	// Assert: ambiguity cannot become a successful executable call.
	require.Equal(t, 1, terminals)
	require.Equal(t, prompty.StreamFailed, stream.Status().State)
	require.Equal(t, int32(1), transport.calls.Load())
	require.Positive(t, transport.body.closed.Load())
}

func TestNativeUnknownFinishReasonRetainsIncompleteOutcomeAndUsage(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeUnknownFinishConformance)
}

func nativeUnknownFinishConformance(t *testing.T, model string) {
	t.Helper()
	// Arrange: unfamiliar finish is an observed outcome, not proof of successful content completion.
	sse := "data: " + `{"choices":[{"index":0,"delta":{"content":"known"}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"future_reason"}]}` + "\n\n" +
		"data: " + `{"choices":[],"usage":{"total_tokens":9}}` + "\n\n"
	client, transport := cleanupFixtureClientModel(strings.NewReader(sse), model)
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	// Act.
	terminals := 0
	for event, err := range stream.Events() {
		require.NoError(t, err)
		if event.Kind == prompty.StreamTerminal {
			terminals++
		}
	}
	status := stream.Status()
	// Assert: completed transport does not turn the provider outcome into success or lose trailing usage.
	require.Equal(t, prompty.StreamCompleted, status.State)
	require.Equal(t, prompty.OutcomeIncomplete, status.Result.Outcome)
	require.Equal(t, "future_reason", status.Result.FinishReason)
	require.Equal(t, 9, status.Result.Usage.TotalTokens)
	require.Equal(t, []prompty.UsageCounter{prompty.UsageTotal}, status.Result.Usage.Known)
	require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: "known"}}, status.Result.Content)
	require.Equal(t, 1, terminals)
	require.Equal(t, int32(1), transport.calls.Load())
	require.Positive(t, transport.body.closed.Load())
}

func TestNativeSDKRejectsMissingOrNegativeToolIndex(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeToolIndexConformance)
}

func nativeToolIndexConformance(t *testing.T, model string) {
	t.Helper()
	for _, call := range []string{
		`{"id":"private-id","function":{"name":"private-name","arguments":"{}"}}`,
		`{"index":null,"id":"private-id","function":{"name":"private-name","arguments":"{}"}}`,
		`{"index":-1,"id":"private-id","function":{"name":"private-name","arguments":"{}"}}`,
	} {
		// Arrange: absent/null index must not inherit the SDK scalar's default zero.
		sse := "data: " + `{"choices":[{"index":0,"delta":{"tool_calls":[` + call + `]}}]}` + "\n\n" +
			"data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n"
		client, transport := cleanupFixtureClientModel(strings.NewReader(sse), model)
		stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
		// Act.
		terminals := 0
		for event, err := range stream.Events() {
			if event.Kind == prompty.StreamTerminal {
				terminals++
				require.ErrorIs(t, err, adapter.ErrInvalidResponse)
				require.NotContains(t, err.Error(), "private")
			}
		}
		// Assert: malformed correlation never becomes a completed/executable call.
		require.Equal(t, prompty.StreamFailed, stream.Status().State)
		require.Empty(t, stream.Status().Result.Content)
		require.Equal(t, 1, terminals)
		require.Equal(t, int32(1), transport.calls.Load())
		require.Positive(t, transport.body.closed.Load())
	}
}

func TestNativeSDKRejectsAmbiguousChoicesAndConflictingTools(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeConflictingToolsConformance)
}

func nativeConflictingToolsConformance(t *testing.T, model string) {
	t.Helper()
	for _, test := range []struct {
		name   string
		frames []string
	}{
		{"multiple choices", []string{`{"choices":[{"index":0,"delta":{"content":"a"}},{"index":1,"delta":{"content":"b"}}]}`}},
		{"nonzero choice", []string{`{"choices":[{"index":1,"delta":{"content":"private"}}]}`}},
		{"missing choice index", []string{`{"choices":[{"delta":{"content":"private"}}]}`}},
		{"null choice index", []string{`{"choices":[{"index":null,"delta":{"content":"private"}}]}`}},
		{"uncorrelated tool", []string{`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"{}"}}]}}]}`}},
		{"conflicting tool ID", []string{
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"fn","arguments":""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"private","function":{"arguments":"{}"}}]}}]}`,
		}},
		{"conflicting tool name", []string{
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"fn","arguments":""}}]}}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"name":"private","arguments":"{}"}}]}}]}`,
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: SDK presence metadata must not manufacture choice/index correlation.
			sse := "data: " + strings.Join(test.frames, "\n\ndata: ") + "\n\n"
			client, transport := cleanupFixtureClientModel(strings.NewReader(sse), model)
			stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			// Act.
			terminals := 0
			for event, err := range stream.Events() {
				if event.Kind == prompty.StreamTerminal {
					terminals++
					require.ErrorIs(t, err, adapter.ErrInvalidResponse)
					require.NotContains(t, err.Error(), "private")
				}
			}
			// Assert: no success/replay and cleanup still closes the actual SDK-owned body.
			require.Equal(t, prompty.StreamFailed, stream.Status().State)
			require.Equal(t, 1, terminals)
			require.Equal(t, int32(1), transport.calls.Load())
			require.Positive(t, transport.body.closed.Load())
		})
	}
}

func TestNativeSDKEOFAndArgumentFailureRetainPartialWithoutReplay(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeArgumentFailureConformance)
}

func nativeArgumentFailureConformance(t *testing.T, model string) {
	t.Helper()
	for _, test := range []struct {
		name   string
		frames []string
		want   error
	}{
		{"missing finish", []string{`{"choices":[{"index":0,"delta":{"content":"partial"}}]}`}, prompty.ErrStreamProtocol},
		{"incomplete arguments", []string{
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"fn","arguments":"{"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}, prompty.ErrIncompleteToolCallArgs},
		{"malformed arguments", []string{
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"a","function":{"name":"fn","arguments":"bad"}}]}}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}`,
		}, prompty.ErrIncompleteToolCallArgs},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: append usage after finish, or after the delta when finish is absent.
			sse := "data: " + strings.Join(test.frames, "\n\ndata: ") + "\n\n" +
				"data: " + `{"choices":[],"usage":{"total_tokens":5}}` + "\n\n"
			client, transport := cleanupFixtureClientModel(strings.NewReader(sse), model)
			stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			// Act.
			terminals := 0
			for event, err := range stream.Events() {
				if event.Kind == prompty.StreamTerminal {
					terminals++
					require.ErrorIs(t, err, test.want)
				}
			}
			status := stream.Status()
			// Assert: neither protocol EOF nor materialization failure erases received parts/usage.
			require.Equal(t, prompty.StreamFailed, status.State)
			require.Len(t, status.Result.Content, 1)
			require.Equal(t, 5, status.Result.Usage.TotalTokens)
			require.Equal(t, 1, terminals)
			require.Equal(t, int32(1), transport.calls.Load())
			require.Positive(t, transport.body.closed.Load())
		})
	}
}

func TestNativeLogicalTextIdentityDoesNotDependOnTransportSegmentation(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeTextSegmentationConformance)
}

func nativeTextSegmentationConformance(t *testing.T, model string) {
	t.Helper()
	var messages []string
	for _, pieces := range [][]string{{"one", "two"}, {"onetwo"}} {
		// Arrange: the same logical text is split into different numbers of SDK deltas.
		var sse strings.Builder
		for _, piece := range pieces {
			_, _ = fmt.Fprintf(&sse, "data: "+`{"choices":[{"index":0,"delta":{"content":%q}}]}`+"\n\n", piece)
		}
		_, _ = sse.WriteString("data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n")
		client, transport := cleanupFixtureClientModel(strings.NewReader(sse.String()), model)
		stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
		messageID := stream.Status().Identity.MessageID
		messages = append(messages, messageID)
		// Act.
		for frame, err := range stream.Events() {
			require.NoError(t, err)
			require.Equal(t, messageID, frame.Identity.MessageID)
			if len(frame.Content) != 0 {
				require.Equal(t, []string{"content:0"}, frame.Identity.PartIDs)
			}
		}
		// Assert: fragment offsets differ, but every fragment belongs to the same logical part.
		status := stream.Status()
		require.Len(t, status.Identity.PartIDs, len(pieces))
		for _, id := range status.Identity.PartIDs {
			require.Equal(t, "content:0", id)
		}
		text, err := prompty.StrictTextFromParts(status.Result.Content)
		require.NoError(t, err)
		require.Equal(t, "onetwo", text)
		require.Equal(t, prompty.StreamCompleted, status.State)
		require.Equal(t, int32(1), transport.calls.Load())
		require.Positive(t, transport.body.closed.Load())
	}
	require.NotEqual(t, messages[0], messages[1], "local message identity is per invocation, not a provider ID")
}

func TestNativeLogicalTextAndRefusalPartsStaySeparate(t *testing.T) {
	t.Parallel()
	forNativeFixtureModels(t, nativeRefusalIdentityConformance)
}

func nativeRefusalIdentityConformance(t *testing.T, model string) {
	t.Helper()
	// Arrange: normal content and refusal text share the canonical Go type but not logical identity.
	sse := "data: " + `{"choices":[{"index":0,"delta":{"content":"one","refusal":"denied"}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{"content":"two","refusal":"again"}}]}` + "\n\n" +
		"data: " + `{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}` + "\n\n"
	client, transport := cleanupFixtureClientModel(strings.NewReader(sse), model)
	stream := client.ExecuteStream(context.Background(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
	// Act.
	data := 0
	for frame, err := range stream.Events() {
		require.NoError(t, err)
		if len(frame.Content) != 0 {
			data++
			require.Equal(t, []string{"content:0", "refusal:0"}, frame.Identity.PartIDs)
			require.Equal(t, 1, frame.Annotations[0].PartIndex)
		}
	}
	// Assert: neither logical part is merged away to obtain a single text identity.
	status := stream.Status()
	require.Equal(t, 2, data)
	require.Equal(t, prompty.OutcomeRefusal, status.Result.Outcome)
	require.Equal(t, []string{"content:0", "refusal:0", "content:0", "refusal:0"}, status.Identity.PartIDs)
	require.Len(t, status.Result.Content, 4)
	require.Equal(t, 1, status.Result.Annotations[0].PartIndex)
	require.Equal(t, 3, status.Result.Annotations[1].PartIndex)
	require.Equal(t, int32(1), transport.calls.Load())
}
