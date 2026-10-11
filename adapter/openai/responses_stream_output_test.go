package openai

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func outputResponseSSEFixture(document, status string, refusal bool) []string {
	kind, field, deltaEvent, doneEvent := "output_text", "text", "response.output_text.delta", "response.output_text.done"
	if refusal {
		kind, field, deltaEvent, doneEvent = "refusal", "refusal", "response.refusal.delta", "response.refusal.done"
	}
	encoded, _ := json.Marshal(document)
	item := fmt.Sprintf(
		`{"id":"msg_fixture","type":"message","role":"assistant","status":"completed","content":[{"type":%q,%q:%s,"annotations":[]}]}`,
		kind,
		field,
		encoded,
	)
	return []string{
		`{"type":"response.created","response":{"id":"resp_fixture","model":"fixture","status":"in_progress"}}`,
		`{"type":"response.output_item.added","output_index":0,"item":{"id":"msg_fixture","type":"message","role":"assistant","status":"in_progress","content":[]}}`,
		fmt.Sprintf(
			`{"type":"response.content_part.added","output_index":0,"item_id":"msg_fixture","content_index":0,"part":{"type":%q,%q:""}}`,
			kind,
			field,
		),
		fmt.Sprintf(
			`{"type":%q,"output_index":0,"item_id":"msg_fixture","content_index":0,"delta":%s}`,
			deltaEvent,
			encoded,
		),
		fmt.Sprintf(
			`{"type":%q,"output_index":0,"item_id":"msg_fixture","content_index":0,%q:%s}`,
			doneEvent,
			field,
			encoded,
		),
		`{"type":"response.output_item.done","output_index":0,"item":` + item + `}`,
		fmt.Sprintf(
			`{"type":"response.%s","response":{"id":"resp_fixture","model":"fixture","status":%q,"output":[%s]}}`,
			status,
			status,
			item,
		),
	}
}

func TestResponsesCreateStreamOriginalOutputAndOutcomes(t *testing.T) {
	t.Parallel()
	for _, scenario := range []string{"completed", "invalid", "not JSON", "incomplete", "failed", "refusal"} {
		t.Run(scenario, func(t *testing.T) {
			t.Parallel()
			// Arrange: only completed ordinary text is eligible for schema success.
			status, document := "completed", `{"id":12}`
			var want error
			switch scenario {
			case "invalid":
				document = `{"id":"bad"}`
				want = prompty.ErrSchemaMismatch
			case "not JSON":
				document = "not JSON"
				want = prompty.ErrSchemaJSONDecode
			case "incomplete", "failed":
				status = scenario
				want = prompty.ErrResponseOutcome
			case "refusal":
				want = prompty.ErrResponseOutcome
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				writeResponseSSEFixture(w, outputResponseSSEFixture(document, status, scenario == "refusal"))
			}))
			defer server.Close()
			manager := responseFixtureAdapter(server)
			manager.base.profiles[0].Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilitySupported
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{
				Name: "answer",
				Schema: []byte(
					`{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`,
				),
			}
			// Act.
			stream := manager.CreateStream(
				t.Context(),
				prompty.RemoteResponseRequest{Execution: exec, Store: new(false)},
			)
			var observed error
			for _, err := range stream.Events() {
				if err != nil {
					observed = err
				}
			}
			// Assert: rejection retains the actual native text and known handle.
			if want == nil {
				require.NoError(t, observed)
			} else {
				require.ErrorIs(t, observed, want)
			}
			require.NotNil(t, stream.RemoteResult().Handle)
			require.Equal(
				t,
				[]prompty.ContentPart{prompty.TextPart{Text: document}},
				stream.RemoteResult().Response.Content,
			)
			if scenario == "refusal" {
				require.Equal(t, prompty.OutcomeRefusal, stream.RemoteResult().Response.Outcome)
			}
		})
	}
}
