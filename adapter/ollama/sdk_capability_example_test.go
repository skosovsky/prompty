package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/ollama/ollama/api"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Ordinary numbers use the official SDK value; exact IDs/decimals need string schemas.
func ExampleAdapter_sdkNumericRepresentation() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(
			w,
			`{"message":{"tool_calls":[{"function":{"name":"lookup","arguments":{"message_ref":1,"fraction":0.25,"patient_ref":"P1"}}}]},"done":true,"done_reason":"stop","prompt_eval_count":0}`,
		)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	sdk := api.NewClient(endpoint, server.Client())
	if err != nil {
		fmt.Println(err)
		return
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL)))
	response, err := client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
	if err != nil {
		fmt.Println(err)
		return
	}
	fmt.Println(response.Content[0].(prompty.ToolCallPart).Args)
	fmt.Println(response.Usage.Availability())
	// Output:
	// {"fraction":0.25,"message_ref":1,"patient_ref":"P1"}
	// unknown
}
