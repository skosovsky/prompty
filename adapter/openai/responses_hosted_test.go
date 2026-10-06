package openai

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

const hostedOutputFixture = `{"id":"resp_fixture","model":"fixture","status":"completed","output":[{"type":"web_search_call","id":"ws_fixture","status":"completed","action":{"type":"search","queries":["reference"],"sources":[{"type":"url","url":"https://example.org/source","future":"retained"}]}},{"type":"message","id":"msg_fixture","role":"assistant","status":"completed","content":[{"type":"output_text","text":"answer","annotations":[{"type":"url_citation","url":"https://example.org/source","title":"source","start_index":0,"end_index":6}]}]},{"type":"file_search_call","id":"fs_fixture","status":"completed","queries":["doc","doc"],"results":[{"file_id":"file_fixture","filename":"doc","text":"found","score":0,"attributes":{"sequence":9007199254740993},"future":true}]}],"usage":{"input_tokens":10,"output_tokens":2,"total_tokens":12}}`

func hostedFixtureAdapter(server *httptest.Server) *Responses {
	adp := responseFixtureAdapter(server)
	adp.base.profiles[0].Capabilities[prompty.CapabilityHostedWebSearch] = prompty.CapabilitySupported
	adp.base.profiles[0].Capabilities[prompty.CapabilityHostedFileSearch] = prompty.CapabilitySupported
	return adp
}

func TestResponsesHostedActualSDKWireAndExplicitContinuation(t *testing.T) {
	t.Parallel()
	// Arrange.
	requests := make(chan string, 3)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, _ := io.ReadAll(request.Body)
		requests <- request.Method + " " + request.URL.String() + " " + string(body)
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(string(body), "previous_response_id") {
			writeResponseFixture(w, "completed", "fresh")
			return
		}
		_, _ = io.WriteString(w, hostedOutputFixture)
	}))
	defer server.Close()
	adp := hostedFixtureAdapter(server)
	tools := []prompty.HostedToolRequest{
		{
			Kind:     HostedWebSearch,
			Settings: []byte(`{"external_web_access":false,"filters":{"allowed_domains":["example.org"]}}`),
		},
		{
			Kind: HostedFileSearch,
			Settings: []byte(
				`{"vector_store_ids":["vs_fixture"],"filters":{"type":"eq","key":"sequence","value":9007199254740993}}`,
			),
		},
	}
	// Act.
	created, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("search"), Store: new(false), HostedTools: tools,
	})
	require.NoError(t, err)
	handleJSON, err := prompty.EncodeRemoteResponseHandle(*created.Handle)
	require.NoError(t, err)
	handle, err := prompty.DecodeRemoteResponseHandle(handleJSON)
	require.NoError(t, err)
	wire, err := prompty.MarshalResponse(created.Response, prompty.WirePolicy{State: prompty.StatePreserve})
	require.NoError(t, err)
	encoded, err := json.Marshal(wire)
	require.NoError(t, err)
	decoded, err := prompty.DecodeResponseJSON(encoded)
	require.NoError(t, err)
	restored := decoded
	items, err := HostedSearchItems(restored)
	require.NoError(t, err)
	_, err = adp.Get(t.Context(), handle)
	require.NoError(t, err)
	_, err = adp.Create(t.Context(), prompty.RemoteResponseRequest{
		Execution: prompty.SimplePrompt("next"), Store: new(false), Previous: &handle,
	})
	require.NoError(t, err)
	// Assert.
	require.Len(t, items, 2)
	require.Equal(t, 0, items[0].OutputIndex)
	require.Equal(t, 2, items[1].OutputIndex)
	require.Equal(t, HostedCoverageAvailable, items[0].Coverage)
	require.Contains(t, string(items[0].Sources[0].SourceJSON), `"future":"retained"`)
	require.Equal(t, json.Number("0"), *items[1].Results[0].Score)
	require.Contains(t, string(items[1].Results[0].Attributes), "9007199254740993")
	require.Len(t, restored.Content, 1)
	require.Equal(t, 0, restored.Annotations[0].PartIndex)
	history, err := restored.AssistantMessage()
	require.NoError(t, err)
	require.Equal(t, restored.MessageAnnotations, history.MessageAnnotations)
	first, second, third := <-requests, <-requests, <-requests
	require.Contains(t, first, `"external_web_access":false`)
	require.Contains(t, first, `"value":9007199254740993`)
	require.Contains(t, first, "web_search_call.action.sources")
	require.Contains(t, first, "file_search_call.results")
	require.Contains(t, second, "include")
	require.Contains(t, second, "file_search_call.results")
	require.Contains(t, third, `"tools":[]`)
	require.Contains(t, third, `"instructions":""`)
	require.NotContains(t, third, "vector_store_ids")
	items[0].SourceJSON[0] = 'x'
	again, err := HostedSearchItems(restored)
	require.NoError(t, err)
	require.Equal(t, byte('{'), again[0].SourceJSON[0])
}

func TestResponsesHostedInvalidSettingsBeforeIO(t *testing.T) {
	t.Parallel()
	for _, settings := range []string{
		`null`, `{"type":"file_search"}`, `{"external_web_access":false,"external_web_access":true}`,
		`{"External_web_access":false}`, `{"external_web_access":null}`, `{"filters":{"allowed_domains":[""]}}`,
	} {
		t.Run(settings, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeResponseFixture(w, "completed", "unexpected")
			}))
			defer server.Close()
			adp := hostedFixtureAdapter(server)
			// Act.
			_, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{
				Execution: prompty.SimplePrompt("search"), Store: new(false),
				HostedTools: []prompty.HostedToolRequest{{Kind: HostedWebSearch, Settings: []byte(settings)}},
			})
			// Assert.
			require.ErrorIs(t, err, prompty.ErrRemoteResponse)
			require.Zero(t, calls.Load())
		})
	}
}

func TestHostedCoverageDoesNotInventUnavailableResults(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		suffix   string
		status   string
		coverage HostedSearchCoverage
	}{
		{"", "completed", HostedCoverageUnknown},
		{`,"results":null`, "completed", HostedCoverageUnavailable},
		{`,"results":[]`, "completed", HostedCoverageAvailable},
		{`,"results":[]`, "incomplete", HostedCoveragePartial},
	} {
		t.Run(string(fixture.coverage), func(t *testing.T) {
			t.Parallel()
			// Arrange.
			raw := `{"id":"fs_fixture","type":"file_search_call","status":"` + fixture.status + `","queries":[]` + fixture.suffix + `}`
			response := prompty.NewResponse(nil)
			// Act.
			err := preserveHostedItem(0, raw, []string{HostedFileSearch}, response)
			require.NoError(t, err)
			items, err := HostedSearchItems(response)
			// Assert.
			require.NoError(t, err)
			require.Equal(t, fixture.coverage, items[0].Coverage)
		})
	}
}

func TestResponsesHostedCapabilitiesFailBeforeIO(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{HostedWebSearch, HostedFileSearch, "custom-hosted"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				writeResponseFixture(w, "completed", "unexpected")
			}))
			defer server.Close()
			adp := responseFixtureAdapter(server)
			// Act.
			_, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{
				Execution: prompty.SimplePrompt("search"), Store: new(false),
				HostedTools: []prompty.HostedToolRequest{{Kind: kind}},
			})
			// Assert.
			require.Error(t, err)
			require.Zero(t, calls.Load())
		})
	}
}

func TestHostedMalformedItemsDoNotBecomeSuccessfulResponse(t *testing.T) {
	t.Parallel()
	for _, replacement := range []string{`"results":{}`, `"results":[{"file_id":"file_fixture","score":1.1}]`} {
		t.Run(replacement, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			raw := strings.Replace(
				hostedOutputFixture,
				`"results":[{"file_id":"file_fixture","filename":"doc","text":"found","score":0,"attributes":{"sequence":9007199254740993},"future":true}]`,
				replacement,
				1,
			)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, raw)
			}))
			defer server.Close()
			adp := hostedFixtureAdapter(server)
			// Act.
			result, err := adp.Create(t.Context(), prompty.RemoteResponseRequest{
				Execution: prompty.SimplePrompt("search"), Store: new(false),
				HostedTools: []prompty.HostedToolRequest{{Kind: HostedWebSearch},
					{Kind: HostedFileSearch, Settings: []byte(`{"vector_store_ids":["vs_fixture"]}`)}},
			})
			// Assert.
			require.ErrorIs(t, err, prompty.ErrRemoteResponse)
			require.Equal(t, prompty.RemoteResponseCompleted, result.Handle.Status)
			require.Equal(t, prompty.OutcomeIncomplete, result.Response.Outcome)
			require.Equal(t, 12, result.Response.Usage.TotalTokens)
			require.Len(t, result.Response.Content, 1)
		})
	}
}

func TestHostedNumberAndFilterFidelity(t *testing.T) {
	t.Parallel()
	// Arrange.
	accepted := []string{"0", "1", "0.99999999999999999999999999999999999999", "1e-10000"}
	rejected := []string{"-1e-10000", "1.00000000000000000000000000000000000001", "1e10000", "null", `"0"`}
	// Act / Assert.
	for _, value := range accepted {
		require.True(t, hostedNumberRange([]byte(value), true), value)
	}
	for _, value := range rejected {
		require.False(t, hostedNumberRange([]byte(value), true), value)
	}
	require.True(t, validHostedFilter([]byte(`{"type":"eq","key":"sequence","value":9007199254740993}`), 0))
	require.False(t, validHostedFilter([]byte(`{"type":"and","filters":[],"value":1}`), 0))
	_, err := validateHostedFileSettings(
		[]byte(`{"vector_store_ids":["vs_fixture"],"ranking_options":{"score_threshold":1.000000000000000000000001}}`),
	)
	require.Error(t, err)
}

func TestHostedProjectionUsesExactProviderFields(t *testing.T) {
	t.Parallel()
	// Arrange: unknown casing must survive in raw JSON, not replace known fields.
	rawResult := []byte(
		`{"file_id":"file_exact","FILE_ID":"shadow","score":0,"Score":1,"attributes":{"integer":9007199254740993}}`,
	)
	rawSource := []byte(`{"type":"url","Type":"shadow","url":"https://example.org","URL":"shadow"}`)
	// Act.
	result, resultErr := parseHostedFileResult(rawResult)
	source, sourceErr := parseHostedSource(rawSource)
	_, aliasErr := parseHostedFileResult([]byte(`{"FILE_ID":"only-alias"}`))
	// Assert.
	require.NoError(t, resultErr)
	require.NoError(t, sourceErr)
	require.Equal(t, "file_exact", result.FileID)
	require.Equal(t, json.Number("0"), *result.Score)
	require.Equal(t, "url", source.Type)
	require.Equal(t, "https://example.org", source.URL)
	require.Equal(t, rawResult, []byte(result.SourceJSON))
	require.ErrorIs(t, aliasErr, prompty.ErrRemoteResponse)
}
