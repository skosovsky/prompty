package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/ollama/ollama/api"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestSyncRequiresOneResponseCallback(t *testing.T) {
	t.Parallel()
	for _, count := range []int{0, 1, 2} {
		t.Run(strconv.Itoa(count), func(t *testing.T) {
			t.Parallel()
			// Arrange: actual HTTP transport receives exactly the chosen number of frames.
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				for range count {
					_, _ = io.WriteString(
						w,
						`{"message":{"role":"assistant","content":"known"},"done":true,"done_reason":"stop"}`+"\n",
					)
				}
			}))
			defer server.Close()
			endpoint, err := url.Parse(server.URL)
			require.NoError(t, err)
			transport := api.NewClient(endpoint, server.Client())
			client := adapter.NewClient(New(WithClient(transport), WithModel("fixture"), WithProfiles(server.URL)))
			// Act.
			response, err := client.Execute(t.Context(), prompty.SimplePrompt("fixture"))
			// Assert.
			if count == 1 {
				require.NoError(t, err)
				text, e := response.StrictText()
				require.NoError(t, e)
				require.Equal(t, "known", text)
			} else {
				require.ErrorIs(t, err, adapter.ErrInvalidResponse)
				require.Nil(t, response)
			}
		})
	}
}
