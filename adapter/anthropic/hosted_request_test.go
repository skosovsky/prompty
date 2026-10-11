package anthropic

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func hostedRequestFixture(t *testing.T) (*Adapter, *atomic.Int32, <-chan []byte) {
	t.Helper()
	requests := make(chan []byte, 1)
	calls := new(atomic.Int32)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		calls.Add(1)
		body, _ := io.ReadAll(request.Body)
		requests <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"id":"msg_fixture","type":"message","role":"assistant","model":"fixture","content":[{"type":"text","text":"answer"}],"stop_reason":"end_turn","usage":{"input_tokens":1,"output_tokens":1,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}}`,
		)
	}))
	t.Cleanup(server.Close)
	sdk := anthropic.NewClient(option.WithBaseURL(server.URL), option.WithAPIKey("fixture"))
	identity := prompty.ProfileIdentity{Provider: "anthropic", Endpoint: server.URL, Model: "fixture"}
	return New(WithModel("fixture"), WithClient(&sdk), WithProfiles(server.URL,
		prompty.CapabilityProfile{Identity: identity, Provenance: "actual SDK fixture, not live conformance",
			Capabilities: map[string]prompty.CapabilitySupport{
				prompty.CapabilityHostedWebSearch: prompty.CapabilitySupported,
				prompty.CapabilityHostedWebFetch:  prompty.CapabilitySupported,
				prompty.CapabilityTools:           prompty.CapabilitySupported,
			}})), calls, requests
}

func TestHostedDeclarationsActualSDKAndOwnedSettings(t *testing.T) {
	t.Parallel()
	// Arrange: explicit versioned tools and one unrelated local function.
	a, calls, requests := hostedRequestFixture(t)
	exec := prompty.SimplePrompt("fixture")
	exec.HostedTools = []prompty.HostedToolRequest{
		{
			Kind: HostedWebSearch,
			Settings: prompty.JSONDocument(
				`{"max_uses":2,"allowed_domains":["example.com/blog"],"user_location":{"type":"approximate","city":"Nha Trang","country":"VN","timezone":"Asia/Ho_Chi_Minh"}}`,
			),
		},
		{
			Kind: HostedWebFetch,
			Settings: prompty.JSONDocument(
				`{"max_uses":1,"blocked_domains":["private.example.com"],"max_content_tokens":9007199254740993,"citations":{"enabled":false}}`,
			),
		},
	}
	exec.Tools = []prompty.ToolDefinition{{Name: "lookup", Parameters: prompty.JSONDocument(`{"type":"object"}`)}}
	// Act: Translate owns SDK values; later input mutation cannot change the request.
	params, err := a.Translate(exec)
	require.NoError(t, err)
	exec.HostedTools[0].Settings[2] = 'X'
	_, err = a.Execute(t.Context(), params)
	// Assert: the actual SDK sends fixed variants, exact integer, and explicit false.
	require.NoError(t, err)
	require.EqualValues(t, 1, calls.Load())
	var request struct {
		Tools []json.RawMessage `json:"tools"`
	}
	require.NoError(t, json.Unmarshal(<-requests, &request))
	require.Len(t, request.Tools, 3)
	require.JSONEq(
		t,
		`{"type":"web_search_20250305","name":"web_search","max_uses":2,"allowed_domains":["example.com/blog"],"user_location":{"type":"approximate","city":"Nha Trang","country":"VN","timezone":"Asia/Ho_Chi_Minh"}}`,
		string(request.Tools[0]),
	)
	require.JSONEq(
		t,
		`{"type":"web_fetch_20250910","name":"web_fetch","max_uses":1,"blocked_domains":["private.example.com"],"max_content_tokens":9007199254740993,"citations":{"enabled":false}}`,
		string(request.Tools[1]),
	)
	require.Contains(t, string(request.Tools[2]), `"name":"lookup"`)
}

func TestHostedDeclarationsInvalidIntentHasZeroIO(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		name, kind, settings string
		modify               func(*Adapter, *prompty.PromptExecution)
		want                 error
	}{
		{"unknown variant", "anthropic.web_search_20260209", `{}`, nil, adapter.ErrUnsupportedCapability},
		{"reserved override", HostedWebSearch, `{"type":"web_search_20260209"}`, nil, ErrHostedTool},
		{"code execution", HostedWebFetch, `{"allowed_callers":["code_execution_20260120"]}`, nil, ErrHostedTool},
		{"duplicate field", HostedWebSearch, `{"max_uses":1,"max_uses":2}`, nil, ErrHostedTool},
		{"nested duplicate", HostedWebSearch, `{"user_location":{"type":"approximate","city":"A","city":"B"}}`, nil, ErrHostedTool},
		{"null citations", HostedWebFetch, `{"citations":{"enabled":null}}`, nil, ErrHostedTool},
		{"missing citations", HostedWebFetch, `{"citations":{}}`, nil, ErrHostedTool},
		{"fractional limit", HostedWebFetch, `{"max_content_tokens":1.5}`, nil, ErrHostedTool},
		{"overflow", HostedWebSearch, `{"max_uses":9223372036854775808}`, nil, ErrHostedTool},
		{"zero", HostedWebSearch, `{"max_uses":0}`, nil, ErrHostedTool},
		{"domain conflict", HostedWebSearch, `{"allowed_domains":["example.com"],"blocked_domains":["example.org"]}`, nil, ErrHostedTool},
		{"URL domain", HostedWebFetch, `{"allowed_domains":["https://example.com"]}`, nil, ErrHostedTool},
		{"empty domains", HostedWebFetch, `{"allowed_domains":[]}`, nil, ErrHostedTool},
		{"precise location", HostedWebSearch, `{"user_location":{"type":"precise","city":"A"}}`, nil, ErrHostedTool},
		{"local collision", HostedWebSearch, `{}`, func(_ *Adapter, exec *prompty.PromptExecution) {
			exec.Tools = []prompty.ToolDefinition{{Name: "web_search"}}
		}, ErrHostedTool},
		{"unknown profile", HostedWebSearch, `{}`, func(a *Adapter, _ *prompty.PromptExecution) { a.profiles = nil }, adapter.ErrUnknownProfile},
		{"unsupported profile", HostedWebFetch, `{}`, func(a *Adapter, _ *prompty.PromptExecution) {
			a.profiles[0].Capabilities[prompty.CapabilityHostedWebFetch] = prompty.CapabilityUnsupported
		}, adapter.ErrUnsupportedCapability},
		{"forbidden combination", HostedWebFetch, `{}`, func(a *Adapter, exec *prompty.PromptExecution) {
			exec.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
			a.profiles[0].ForbiddenCombinations = [][]string{{prompty.CapabilityHostedWebFetch, prompty.CapabilityTools}}
		}, adapter.ErrUnsupportedCombination},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			a, calls, _ := hostedRequestFixture(t)
			exec := prompty.SimplePrompt("fixture")
			exec.HostedTools = []prompty.HostedToolRequest{
				{Kind: test.kind, Settings: prompty.JSONDocument(test.settings)},
			}
			if test.modify != nil {
				test.modify(a, exec)
			}
			// Act: both direct Translate and checked dispatch reject invalid intent.
			_, directErr := a.Translate(exec)
			_, err := adapter.NewClient(a).Execute(t.Context(), exec)
			// Assert.
			require.ErrorIs(t, directErr, test.want)
			require.ErrorIs(t, err, test.want)
			require.Zero(t, calls.Load())
		})
	}
}
