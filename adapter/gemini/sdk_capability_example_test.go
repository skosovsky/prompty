package gemini

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Ordinary numbers use the official SDK value; exact IDs/decimals need string schemas.
func ExampleAdapter_sdkNumericRepresentation() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintln(
			w,
			`{"candidates":[{"content":{"parts":[{"functionCall":{"name":"lookup","args":{"message_ref":1,"fraction":0.25,"patient_ref":"P1"}}}]},"finishReason":"STOP"}],"usageMetadata":{"promptTokenCount":0}}`,
		)
	}))
	defer server.Close()
	sdk, err := genai.NewClient(
		context.Background(),
		&genai.ClientConfig{
			APIKey:      "fixture",
			Backend:     genai.BackendGeminiAPI,
			HTTPClient:  server.Client(),
			HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
		},
	)
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
