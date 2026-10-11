package gemini

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"

	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

func ExampleAdapter_multimodalResult() {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(
			w,
			`{"candidates":[{"content":{"role":"model","parts":[{"text":"done"}]},"finishReason":"STOP"}]}`,
		)
	}))
	defer server.Close()
	sdk, err := genai.NewClient(context.Background(), &genai.ClientConfig{
		APIKey: "offline", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(),
		HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"},
	})
	if err != nil {
		panic(err)
	}
	profile := prompty.CapabilityProfile{
		Identity:     prompty.ProfileIdentity{Provider: "gemini", Endpoint: server.URL, Model: "fixture"},
		Provenance:   "offline SDK fixture; verify your own endpoint/model",
		Capabilities: map[string]prompty.CapabilitySupport{CapabilityFunctionResultMedia: prompty.CapabilitySupported},
		InputMIME:    []string{"image/png"},
	}
	client := adapter.NewClient(New(WithClient(sdk), WithModel("fixture"), WithProfiles(server.URL, profile)))
	execution := prompty.SimplePrompt("inspect result")
	execution.Messages = append(execution.Messages,
		prompty.ChatMessage{Role: prompty.RoleAssistant, Content: []prompty.ContentPart{
			prompty.ToolCallPart{ID: "call_one", Name: "image", Args: "{}"},
		}},
		prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{
			prompty.ToolResultPart{ToolCallID: "call_one", Name: "image", Content: []prompty.ContentPart{
				prompty.TextPart{Text: "before"},
				prompty.MediaPart{MediaType: "image", MIMEType: "image/png", Data: []byte("fixture image")},
				prompty.TextPart{Text: "after"},
			}},
		}},
	)
	response, err := client.Execute(context.Background(), execution)
	if err != nil {
		panic(err)
	}
	fmt.Println(response.Content[0].(prompty.TextPart).Text)
	result := execution.Messages[2].Content[0].(prompty.ToolResultPart)
	result.Content[1] = prompty.MediaPart{MediaType: "image", MIMEType: "image/gif", Data: []byte("unsupported")}
	execution.Messages[2].Content[0] = result
	_, err = client.Execute(context.Background(), execution)
	fmt.Println("unsupported rejected:", err != nil, "requests:", calls.Load())
	// Output:
	// done
	// unsupported rejected: true requests: 1
}
