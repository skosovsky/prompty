package main

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	anthropicoption "github.com/anthropics/anthropic-sdk-go/option"
	"github.com/ollama/ollama/api"
	openaisdk "github.com/openai/openai-go/v3"
	openaioption "github.com/openai/openai-go/v3/option"
	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	"github.com/skosovsky/prompty/adapter/anthropic"
	"github.com/skosovsky/prompty/adapter/gemini"
	"github.com/skosovsky/prompty/adapter/ollama"
	"github.com/skosovsky/prompty/adapter/openai"
	"google.golang.org/genai"
)

const ordinaryArgs = `{"message_ref":1,"patient_ref":"P1","service_ref":"S1","appointment_ref":"A1","zero":0,"fraction":0.25,"nested":[{"n":2}]}`
const exactArgs = `{"id":9007199254740993,"decimal":1.234567890123456789}`

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
func check(t *testing.T, ok bool, message string) {
	t.Helper()
	if !ok {
		t.Fatal(message)
	}
}

func sdkConsumer(t *testing.T, provider string, server *httptest.Server) prompty.Invoker {
	t.Helper()
	profile := prompty.CapabilityProfile{Identity: prompty.ProfileIdentity{Provider: provider, Endpoint: server.URL, Model: "fixture"}, Provenance: "isolated local consumer; no live acceptance", Capabilities: map[string]prompty.CapabilitySupport{
		prompty.CapabilityTools: prompty.CapabilitySupported, prompty.CapabilityProviderState: prompty.CapabilitySupported, prompty.CapabilityNativeStream: prompty.CapabilitySupported, prompty.CapabilityToolNameCorrelation: prompty.CapabilitySupported}}
	switch provider {
	case "gemini":
		sdk, err := genai.NewClient(t.Context(), &genai.ClientConfig{APIKey: "fixture", Backend: genai.BackendGeminiAPI, HTTPClient: server.Client(), HTTPOptions: genai.HTTPOptions{BaseURL: server.URL, APIVersion: "v1beta"}})
		must(t, err)
		return adapter.NewClient(gemini.New(gemini.WithClient(sdk), gemini.WithModel("fixture"), gemini.WithProfiles(server.URL, profile)))
	case "ollama":
		endpoint, err := url.Parse(server.URL)
		must(t, err)
		return adapter.NewClient(ollama.New(ollama.WithClient(api.NewClient(endpoint, server.Client())), ollama.WithModel("fixture"), ollama.WithProfiles(server.URL, profile)))
	case "openai":
		sdk := openaisdk.NewClient(openaioption.WithAPIKey("fixture"), openaioption.WithBaseURL(server.URL), openaioption.WithHTTPClient(server.Client()))
		return adapter.NewClient(openai.New(openai.WithClient(&sdk), openai.WithModel("fixture"), openai.WithProfiles(server.URL, profile)))
	case "anthropic":
		sdk := anthropicsdk.NewClient(anthropicoption.WithAPIKey("fixture"), anthropicoption.WithBaseURL(server.URL), anthropicoption.WithHTTPClient(server.Client()))
		return adapter.NewClient(anthropic.New(anthropic.WithClient(&sdk), anthropic.WithModel("fixture"), anthropic.WithProfiles(server.URL, profile)))
	default:
		t.Fatal("unknown fixture provider")
		return nil
	}
}
func consumerToolPayload(provider, args, usage string) string {
	switch provider {
	case "gemini":
		return `{"candidates":[{"content":{"role":"model","parts":[{"functionCall":{"id":"call-a","name":"lookup","args":` + args + `},"thoughtSignature":"AAH/"}]},"finishReason":"STOP"}]` + usage + `}`
	case "ollama":
		return `{"message":{"role":"assistant","tool_calls":[{"id":"call-a","function":{"name":"lookup","arguments":` + args + `}}]},"done":true,"done_reason":"stop"` + usage + `}`
	case "openai":
		encoded, _ := json.Marshal(args)
		return `{"id":"fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"call-a","type":"function","function":{"name":"lookup","arguments":` + string(encoded) + `}}]},"finish_reason":"tool_calls"}]` + usage + `}`
	default:
		return `{"id":"fixture","type":"message","model":"fixture","role":"assistant","content":[{"type":"tool_use","id":"call-a","name":"lookup","input":` + args + `}],"stop_reason":"tool_use"` + usage + `}`
	}
}
func consumerInvoke(t *testing.T, client prompty.Invoker, exec *prompty.PromptExecution, mode prompty.StreamMode) (*prompty.Response, error) {
	t.Helper()
	if mode == "" {
		return client.Execute(t.Context(), exec)
	}
	stream := client.ExecuteStream(t.Context(), exec, mode)
	for range stream.Events() {
	}
	return stream.Status().Result, stream.Status().Err
}
func consumerCanonical(t *testing.T, response *prompty.Response) string {
	t.Helper()
	check(t, response != nil && len(response.Content) == 1, "missing tool response")
	call, ok := response.Content[0].(prompty.ToolCallPart)
	check(t, ok, "not a tool call")
	check(t, call.ID == "call-a" && call.Name == "lookup", "identity changed")
	return call.Args
}
func TestConsumerOrdinaryNumericAndPresence(t *testing.T) {
	for _, provider := range []string{"gemini", "ollama", "openai", "anthropic"} {
		for _, present := range []bool{false, true} {
			t.Run(provider+map[bool]string{false: "/missing", true: "/zero"}[present], func(t *testing.T) {
				// Arrange: genuine SDK + domain values copied from audited consumer schemas.
				usage := ""
				if present {
					switch provider {
					case "gemini":
						usage = `,"usageMetadata":{"promptTokenCount":0,"candidatesTokenCount":0,"totalTokenCount":0}`
					case "ollama":
						usage = `,"prompt_eval_count":0,"eval_count":0`
					case "openai":
						usage = `,"usage":{"prompt_tokens":0,"completion_tokens":0,"total_tokens":0}`
					case "anthropic":
						usage = `,"usage":{"input_tokens":0,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`
					}
				}
				args := ordinaryArgs
				if provider == "openai" || provider == "anthropic" {
					args = exactArgs
				}
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, consumerToolPayload(provider, args, usage)+"\n")
				}))
				defer server.Close()
				client := sdkConsumer(t, provider, server)
				// Act.
				response, err := client.Execute(t.Context(), prompty.SimplePrompt("fixture"))
				// Assert: SDK raw-capable providers retain exact values; ambiguous zeros stay unknown.
				must(t, err)
				actual := consumerCanonical(t, response)
				var expected any
				must(t, json.Unmarshal([]byte(args), &expected))
				encoded, err := json.Marshal(expected)
				must(t, err)
				if provider == "openai" || provider == "anthropic" {
					check(t, actual == args, "available exact arguments lost")
				} else {
					check(t, actual == string(encoded), "ordinary SDK value changed")
				}
				known := slices.Contains(response.Usage.Known, prompty.UsagePrompt)
				check(t, known == (present && (provider == "openai" || provider == "anthropic")), "usage presence changed")
			})
		}
	}
}
func TestConsumerOrdinaryContinuationAndGates(t *testing.T) {
	for _, provider := range []string{"gemini", "ollama"} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
			t.Run(provider+"/"+string(mode), func(t *testing.T) {
				// Arrange: capture subsequent SDK request; fixture signatures test transfer only.
				var calls atomic.Int32
				bodies := make(chan string, 8)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					if provider == "gemini" && mode == prompty.StreamNative {
						w.Header().Set("Content-Type", "text/event-stream")
					}
					body, _ := io.ReadAll(r.Body)
					bodies <- string(body)
					calls.Add(1)
					payload := consumerToolPayload(provider, ordinaryArgs, "")
					if provider == "gemini" && mode == prompty.StreamNative {
						payload = "data: " + payload + "\n"
					}
					_, _ = io.WriteString(w, payload+"\n")
				}))
				defer server.Close()
				client := sdkConsumer(t, provider, server)
				// Act: response wire then execution wire persistence, then public SDK request.
				response, err := consumerInvoke(t, client, prompty.SimplePrompt("fixture"), mode)
				must(t, err)
				wire, err := prompty.MarshalResponse(response, prompty.WirePolicy{State: prompty.StatePreserve})
				must(t, err)
				restored, err := prompty.UnmarshalResponse(wire)
				must(t, err)
				message, err := restored.AssistantMessage()
				must(t, err)
				exec := prompty.SimplePrompt("fixture").AddMessage(message).AddMessage(prompty.ChatMessage{Role: prompty.RoleTool, Content: []prompty.ContentPart{prompty.ToolResultPart{ToolCallID: "call-a", Name: "lookup", Content: []prompty.ContentPart{prompty.TextPart{Text: "ok"}}}}})
				executionWire, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
				must(t, err)
				exec, err = prompty.UnmarshalExecution(executionWire)
				must(t, err)
				_, err = consumerInvoke(t, client, exec, mode)
				must(t, err)
				// Assert: association/signature and ordinary values, no consumer-side repair.
				<-bodies
				next := <-bodies
				check(t, calls.Load() == 2, "unexpected redispatch")
				check(t, strings.Contains(next, `"message_ref":1`) && strings.Contains(next, `"patient_ref":"P1"`) && strings.Contains(next, `"id":"call-a"`), "continuation value/identity lost")
				if provider != "gemini" {
					return
				}
				check(t, strings.Contains(next, `"thoughtSignature":"AAH/"`), "signature lost")
				for _, gate := range []string{"destination", "model", "expiry", "required", "association"} {
					candidate := exec.Clone()
					expected := prompty.ErrProviderState
					switch gate {
					case "destination":
						candidate.Messages[1].ProviderState[0].Envelope.Scope.Endpoint = "https://foreign.invalid"
						expected = prompty.ErrStateDestination
					case "model":
						candidate.Messages[1].ProviderState[0].Envelope.Scope.Model = "other"
						expected = prompty.ErrStateDestination
					case "expiry":
						expired := time.Now().Add(-time.Minute)
						candidate.Messages[1].ProviderState[0].Envelope.ExpiresAt = &expired
						expected = prompty.ErrStateExpired
					case "required":
						omitted, err := prompty.MarshalResponse(restored, prompty.WirePolicy{State: prompty.StateOmit})
						must(t, err)
						unavailable, err := prompty.UnmarshalResponse(omitted)
						must(t, err)
						candidate.Messages[1], err = unavailable.AssistantMessage()
						must(t, err)
						expected = prompty.ErrStateUnavailable
					case "association":
						call := candidate.Messages[1].Content[0].(prompty.ToolCallPart)
						call.Args = `{"message_ref":2}`
						candidate.Messages[1].Content[0] = call
					}
					_, err := consumerInvoke(t, client, candidate, mode)
					check(t, errors.Is(err, expected), "state gate failed: "+gate)
					check(t, calls.Load() == 2, "gate dispatched")
				}
			})
		}
	}
}
func TestConsumerNativeTrailingAndPartialAccounting(t *testing.T) {
	for _, provider := range []string{"gemini", "ollama"} {
		for _, failure := range []bool{false, true} {
			for _, zero := range []bool{false, true} {
				t.Run(provider+map[bool]string{false: "/finish", true: "/error"}[failure]+map[bool]string{false: "/nonzero", true: "/zero"}[zero], func(t *testing.T) {
					// Arrange: one cumulative usage observation; malformed next record must retain it.
					prompt, completion, total := 7, 2, 9
					if zero {
						prompt, completion, total = 0, 0, 0
					}
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						if provider == "gemini" {
							_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]},\"finishReason\":\"STOP\"}]}\n\n")
							payload, _ := json.Marshal(map[string]any{"usageMetadata": map[string]int{"promptTokenCount": prompt, "candidatesTokenCount": completion, "totalTokenCount": total}})
							_, _ = io.WriteString(w, "data: "+string(payload)+"\n\n")
							if failure {
								_, _ = io.WriteString(w, "data: {broken\n\n")
							}
						} else {
							first, _ := json.Marshal(map[string]any{"message": map[string]string{"content": "ok"}, "prompt_eval_count": prompt, "eval_count": completion})
							_, _ = io.WriteString(w, string(first)+"\n")
							if failure {
								_, _ = io.WriteString(w, "{broken\n")
							} else {
								last, _ := json.Marshal(map[string]any{"done": true, "done_reason": "stop", "prompt_eval_count": prompt, "eval_count": completion})
								_, _ = io.WriteString(w, string(last)+"\n")
							}
						}
					}))
					defer server.Close()
					client := sdkConsumer(t, provider, server)
					// Act.
					result, err := consumerInvoke(t, client, prompty.SimplePrompt("fixture"), prompty.StreamNative)
					// Assert: no invented cost/total or duplicate addition; unknown remains unknown.
					if failure {
						check(t, err != nil, "error frame silently succeeded")
					} else {
						must(t, err)
					}
					check(t, result != nil, "partial accounting lost")
					check(t, result.Usage.PromptTokens == prompt && result.Usage.CompletionTokens == completion, "cumulative usage double counted/lost")
					check(t, slices.Contains(result.Usage.Known, prompty.UsagePrompt) == !zero, "scalar zero presence invented")
				})
			}
		}
	}
}

func TestConsumerStopRetainsOnlyObservedUsage(t *testing.T) {
	for _, provider := range []string{"gemini", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			// Arrange: later usage is deliberately different and must never become observed evidence.
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				calls.Add(1)
				if provider == "gemini" {
					w.Header().Set("Content-Type", "text/event-stream")
					_, _ = io.WriteString(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"early\"}]}}],\"usageMetadata\":{\"promptTokenCount\":7}}\n\n")
					_, _ = io.WriteString(w, "data: {\"candidates\":[{\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":99}}\n\n")
				} else {
					_, _ = io.WriteString(w, `{"message":{"content":"early"},"prompt_eval_count":7}`+"\n"+`{"done":true,"done_reason":"stop","prompt_eval_count":99}`+"\n")
				}
			}))
			defer server.Close()
			client := sdkConsumer(t, provider, server)
			// Act: stop the public consumer iterator after the first SDK frame.
			stream := client.ExecuteStream(t.Context(), prompty.SimplePrompt("fixture"), prompty.StreamNative)
			observed := false
			for frame, err := range stream.Events() {
				must(t, err)
				if frame.Usage.PromptTokens == 7 {
					observed = true
					break
				}
			}
			stream.Close()
			// Assert: canceled exchange retains observed evidence, no invented later usage or replay.
			check(t, observed, "usage not observed")
			check(t, stream.Status().Result.Usage.PromptTokens == 7, "unobserved usage consumed")
			check(t, stream.Status().Result.ContinuationUnavailable, "stopped exchange continuable")
			check(t, calls.Load() == 1, "consumer stop replayed request")
		})
	}
}
