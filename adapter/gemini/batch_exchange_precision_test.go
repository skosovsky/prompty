package gemini

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestBatchSDKRepresentationCharacterization(t *testing.T) {
	t.Parallel()
	for _, fileMode := range []bool{false, true} {
		t.Run(map[bool]string{false: "inline", true: "JSONL"}[fileMode], func(t *testing.T) {
			t.Parallel()
			// Arrange: each existing batch output path supplies the same numeric cases.
			arguments := []string{
				`{"integer":9007199254740993}`,
				`{"nested":[{"negative":-9007199254740993,"fraction":0.10000000000000001,"exponent":1.234567890123456789e2}]}`,
				`{}`,
			}
			var inline, lines []string
			for index, args := range arguments {
				id := []string{"one", "two", "three"}[index]
				response := `"response":{"candidates":[{"content":{"parts":[{"functionCall":{"id":"call","name":"lookup","args":` + args + `}}]},"finishReason":"STOP"}]}`
				inline = append(inline, `{"metadata":{"key":"`+id+`"},`+response+`}`)
				lines = append(lines, `{"key":"`+id+`",`+response+`}`)
			}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == http.MethodPost:
					_, _ = io.WriteString(w, `{"name":"batches/fixture","metadata":{"state":"JOB_STATE_PENDING"}}`)
				case fileMode:
					_, _ = io.WriteString(
						w,
						`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"responsesFile":"files/output"}}}`,
					)
				default:
					_, _ = io.WriteString(
						w,
						`{"name":"batches/fixture","metadata":{"state":"JOB_STATE_SUCCEEDED","output":{"inlinedResponses":{"inlinedResponses":[`+strings.Join(
							inline,
							",",
						)+`]}}}}`,
					)
				}
			}))
			t.Cleanup(server.Close)
			base, _ := batchOperationFixture(t, server)
			manager := NewBatches(
				"account-fixture",
				WithClient(base.client),
				WithModel("fixture"),
				WithProfiles(server.URL, base.profiles...),
			)
			if fileMode {
				files := &batchFixtureFiles{
					binding: prompty.BatchFileBinding{
						Provider:       "gemini",
						Endpoint:       server.URL,
						AccountBinding: "account-fixture",
					},
					output: strings.Join(lines, "\n") + "\n",
					status: http.StatusOK,
				}
				manager = NewFileBatches(
					"account-fixture",
					files,
					BatchFileOptions{},
					WithClient(base.client),
					WithModel("fixture"),
					WithProfiles(server.URL, base.profiles...),
				)
			}
			input, options := inlineBatchFixtureInputs()
			created, err := manager.Submit(t.Context(), input)
			require.NoError(t, err)
			observed, err := manager.Get(t.Context(), *created.Handle)
			require.NoError(t, err)
			// Act: native SDK operation source/JSONL download -> canonical batch results.
			result, err := manager.Results(t.Context(), *observed.Handle, options)
			// Assert.
			require.NoError(t, err)
			require.True(t, result.Completeness.CorrelationComplete)
			require.True(t, result.Completeness.DownloadComplete)
			require.Len(t, result.Items, 3)
			for index, item := range result.Items {
				// Both result paths project official SDK DTO numeric values.
				require.Equal(t, prompty.BatchItemSuccess, item.Outcome)
				expected := sdkCanonicalArgs(arguments[index])
				require.Equal(
					t,
					geminiJSONValue([]byte(expected)),
					geminiJSONValue([]byte(item.Response.Content[0].(prompty.ToolCallPart).Args)),
				)
			}
		})
	}
}
