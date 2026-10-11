package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openaisdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty/adapter"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

func TestDirectBYOTCheckedHTTP(t *testing.T) {
	t.Parallel()
	// Arrange: materialize the exact quickstart input and observe the actual SDK request.
	request := make(chan observedRequest, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body observedRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		request <- body
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(
			[]byte(
				`{"id":"fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"4"},"finish_reason":"stop"}]}`,
			),
		)
	}))
	defer server.Close()
	execution, err := materialize(t.Context(), Question{Text: "What is 2+2?"})
	require.NoError(t, err)
	sdk := openaisdk.NewClient(
		option.WithAPIKey("fixture"),
		option.WithBaseURL(server.URL),
		option.WithHTTPClient(server.Client()),
	)
	client := adapter.NewClient(
		openaiadapter.New(
			openaiadapter.WithClient(&sdk),
			openaiadapter.WithModel("fixture"),
			openaiadapter.WithProfiles(server.URL),
		),
	)
	// Act.
	response, err := client.Execute(t.Context(), execution)
	// Assert.
	require.NoError(t, err)
	text, err := response.StrictText()
	require.NoError(t, err)
	require.Equal(t, "4", text)
	body := <-request
	require.Len(t, body.Messages, 2)
	require.Equal(t, "user", body.Messages[1].Role)
	require.Equal(t, "What is 2+2?", body.Messages[1].Content)
}

type observedRequest struct {
	Messages []observedMessage `json:"messages"`
}

type observedMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}
