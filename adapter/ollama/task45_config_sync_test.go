package ollama

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func TestHTTPChatConfigurationCategory(t *testing.T) {
	t.Parallel()
	// Arrange: constructor validation happens before response IO.
	endpoint := &url.URL{Scheme: "http", Host: "fixture.invalid"}
	for _, test := range []struct {
		endpoint *url.URL
		client   *http.Client
		limit    int
	}{
		{nil, http.DefaultClient, 1}, {endpoint, nil, 1}, {endpoint, http.DefaultClient, 0}, {&url.URL{Scheme: "file", Host: "fixture.invalid"}, http.DefaultClient, 1},
	} {
		// Act.
		client, err := NewHTTPChatClient(test.endpoint, test.client, test.limit)
		// Assert.
		require.ErrorIs(t, err, ErrInvalidHTTPConfig)
		require.NotErrorIs(t, err, adapter.ErrInvalidResponse)
		require.Nil(t, client)
	}
	client, err := NewHTTPChatClient(endpoint, http.DefaultClient, 1)
	require.NoError(t, err)
	require.NotNil(t, client)
}

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
			transport, err := NewHTTPChatClient(endpoint, server.Client(), 4096)
			require.NoError(t, err)
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
