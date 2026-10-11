// Offline independent consumer of the public checked adapter APIs.
package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ollama/ollama/api"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/adapter/anthropic"
	"github.com/skosovsky/prompty/adapter/gemini"
	"github.com/skosovsky/prompty/adapter/ollama"
	"github.com/skosovsky/prompty/adapter/openai"
)

func main() {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		var body string
		switch {
		case strings.HasSuffix(r.URL.Path, "/api/chat"):
			body = `{"model":"fixture","message":{"role":"assistant","content":"ok"},"done":true,"done_reason":"stop"}` + "\n"
		case strings.HasSuffix(r.URL.Path, "/chat/completions"):
			body = `{"id":"fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`
		case strings.HasSuffix(r.URL.Path, "/messages"):
			body = `{"id":"fixture","type":"message","model":"fixture","role":"assistant","content":[{"type":"text","text":"ok"}],"stop_reason":"end_turn"}`
		default:
			body = `{"modelVersion":"fixture","candidates":[{"content":{"role":"model","parts":[{"text":"ok"}]},"finishReason":"STOP"}]}`
		}
		_, _ = fmt.Fprint(w, body)
	}))
	defer server.Close()
	endpoint, err := url.Parse(server.URL)
	if err != nil {
		panic(err)
	}
	oa := openaisdk.NewClient(openaioption.WithAPIKey("fixture"), openaioption.WithBaseURL(server.URL), openaioption.WithHTTPClient(server.Client()))
	an := anthropicsdk.NewClient(anthropicoption.WithAPIKey("fixture"), anthropicoption.WithBaseURL(server.URL), anthropicoption.WithHTTPClient(server.Client()))
	ge, err := genai.NewClient(context.Background(), &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"}})
	if err != nil {
		panic(err)
	}
	clients := []struct {
		name   string
		client prompty.Invoker
	}{
		{"ollama", adapter.NewClient(ollama.New(ollama.WithClient(api.NewClient(endpoint, server.Client())), ollama.WithModel("fixture"), ollama.WithProfiles(server.URL)))},
		{"gemini", adapter.NewClient(gemini.New(gemini.WithClient(ge), gemini.WithModel("fixture"), gemini.WithProfiles(server.URL)))},
		{"openai", adapter.NewClient(openai.New(openai.WithClient(&oa), openai.WithModel("fixture"), openai.WithProfiles(server.URL)))},
		{"anthropic", adapter.NewClient(anthropic.New(anthropic.WithClient(&an), anthropic.WithModel("fixture"), anthropic.WithProfiles(server.URL)))},
	}
	for _, entry := range clients {
		response, callErr := entry.client.Execute(context.Background(), prompty.SimplePrompt("fixture"))
		if callErr != nil {
			panic(callErr)
		}
		text, textErr := prompty.StrictTextFromParts(response.Content)
		if textErr != nil || text != "ok" || response.Outcome != prompty.OutcomeCompleted {
			panic("unexpected canonical response")
		}
		fmt.Println(entry.name + ": ok")
	}
}
