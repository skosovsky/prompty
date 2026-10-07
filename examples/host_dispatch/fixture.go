package hostdispatch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/skosovsky/toolsy"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

const lookupSchema = `{"type":"object","properties":{"id":{"type":"integer"}},"required":["id"],"additionalProperties":false}`

// LookupArgs is the application's BYOT input.
type LookupArgs struct {
	ID int64 `json:"id"`
}

// Validator uses the real consumer extractor without invoking any handler.
type Validator struct{ extractor *toolsy.Extractor[LookupArgs] }

// NewValidator constructs a pure caller decoder.
func NewValidator() (*Validator, error) {
	extractor, err := toolsy.NewExtractor[LookupArgs](false)
	if err != nil {
		return nil, fmt.Errorf("caller extractor: %w", err)
	}
	return &Validator{extractor: extractor}, nil
}

// ValidateToolCall checks caller-owned input independently of execution schema.
func (v *Validator) ValidateToolCall(_ string, args string) error {
	_, err := v.extractor.ParseAndValidate([]byte(args))
	if err != nil {
		return fmt.Errorf("caller arguments: %w", err)
	}
	return nil
}

// LookupTool transfers native identity and exact JSON into the real Session.
// Before is an optional host control gate, used by local approval tests only.
type LookupTool struct {
	Name            string
	Before          func(context.Context, toolsy.ToolInput, func(toolsy.Chunk) error) error
	Effect          func(toolsy.ToolInput)
	BusinessFailure bool
}

// Manifest describes original tool inputs without provider-specific lowering.
func (tool LookupTool) Manifest() toolsy.ToolManifest {
	return toolsy.ToolManifest{Name: tool.Name, Parameters: map[string]any{
		"type": "object", "properties": map[string]any{"id": map[string]any{"type": "integer"}},
		"required": []string{"id"}, "additionalProperties": false,
	}}
}

// Execute performs exactly one caller-selected operation; it never retries.
func (tool LookupTool) Execute(
	ctx context.Context,
	_ *toolsy.RunEnv,
	input toolsy.ToolInput,
	yield func(toolsy.Chunk) error,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if tool.Before != nil {
		if err := tool.Before(ctx, input, yield); err != nil {
			return err
		}
	}
	if tool.Effect != nil {
		tool.Effect(input.Clone())
	}
	if tool.BusinessFailure {
		return toolsy.NewValidationError("private diagnostic must not reach model")
	}
	return yield(
		toolsy.Chunk{
			Event:    toolsy.EventResult,
			Data:     append([]byte(nil), input.ArgsJSON...),
			MimeType: toolsy.MimeTypeJSON,
		},
	)
}

// Fixture injects a local HTTP provider into the real SDK and checked adapter.
// Recorded requests are owned snapshots protected against concurrent access.
type Fixture struct {
	Server      *httptest.Server
	Client      prompty.Invoker
	Destination prompty.ProfileIdentity
	mu          sync.Mutex
	requests    [][]byte
}

// NewFixture returns duplicate name/args with distinct IDs, then a final answer.
func NewFixture() *Fixture {
	fixture := &Fixture{}
	fixture.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		body, err := io.ReadAll(request.Body)
		if err != nil {
			http.Error(w, "read failed", http.StatusBadRequest)
			return
		}
		fixture.mu.Lock()
		fixture.requests = append(fixture.requests, body)
		count := len(fixture.requests)
		fixture.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if count == 1 {
			_, _ = io.WriteString(
				w,
				`{"id":"fixture","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","tool_calls":[{"id":"native-one","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}},{"id":"native-two","type":"function","function":{"name":"lookup","arguments":"{\"id\":9007199254740993}"}}]},"finish_reason":"tool_calls"}]}`,
			)
			return
		}
		_, _ = io.WriteString(
			w,
			`{"id":"fixture-final","model":"fixture","choices":[{"index":0,"message":{"role":"assistant","content":"done"},"finish_reason":"stop"}]}`,
		)
	}))
	sdk := openai.NewClient(
		option.WithBaseURL(fixture.Server.URL),
		option.WithAPIKey("offline-fixture"),
		option.WithMaxRetries(0),
	)
	fixture.Destination = prompty.ProfileIdentity{Provider: "openai", Endpoint: fixture.Server.URL, Model: "fixture"}
	profile := prompty.CapabilityProfile{Identity: fixture.Destination, Provenance: "offline SDK/HTTP fixture only",
		Capabilities: map[string]prompty.CapabilitySupport{prompty.CapabilityTools: prompty.CapabilitySupported}}
	fixture.Client = adapter.NewClient(
		openaiadapter.New(
			openaiadapter.WithClient(&sdk),
			openaiadapter.WithModel("fixture"),
			openaiadapter.WithProfiles(fixture.Server.URL, profile),
		),
	)
	return fixture
}

// Requests returns deep copies of observed HTTP requests.
func (fixture *Fixture) Requests() [][]byte {
	fixture.mu.Lock()
	defer fixture.mu.Unlock()
	out := make([][]byte, len(fixture.requests))
	for i, request := range fixture.requests {
		out[i] = append([]byte(nil), request...)
	}
	return out
}

// Execution builds a caller-owned scoped request from tool names.
func Execution(names ...string) (*prompty.PromptExecution, error) {
	execution := prompty.SimplePrompt("look up the exact integer twice")
	scope := prompty.ToolScope{}
	for _, name := range names {
		definition := prompty.ToolDefinition{Name: name, Parameters: prompty.JSONDocument(lookupSchema)}
		execution.Tools = append(execution.Tools, definition)
		scope.Allowed = append(scope.Allowed, prompty.ToolManifestFromDefinition(definition))
	}
	scoped, err := execution.WithToolScope(scope)
	if err != nil {
		return nil, fmt.Errorf("caller scope: %w", err)
	}
	return scoped, nil
}

// Demo executes the complete offline path, including context checkpoint and the
// next checked SDK request. It does not require network credentials.
func Demo(ctx context.Context) (string, error) {
	fixture := NewFixture()
	defer fixture.Server.Close()
	execution, err := Execution("lookup")
	if err != nil {
		return "", err
	}
	registry, err := toolsy.NewRegistryBuilder().Add(LookupTool{Name: "lookup"}).Build()
	if err != nil {
		return "", fmt.Errorf("caller registry: %w", err)
	}
	session, err := toolsy.NewSession(registry)
	if err != nil {
		return "", fmt.Errorf("caller session: %w", err)
	}
	validator, err := NewValidator()
	if err != nil {
		return "", err
	}
	response, dispatchErr := fixture.Client.Execute(ctx, execution.Clone())
	if dispatchErr != nil {
		return "", dispatchErr
	}
	checkpoint, err := SaveResponse(response, fixture.Destination)
	if err != nil {
		return "", err
	}
	restored, err := RestoreResponse(checkpoint, fixture.Destination)
	if err != nil {
		return "", err
	}
	batch, err := Admit(ctx, execution, restored, nil, validator, fixture.Destination)
	if err != nil {
		return "", err
	}
	if err = batch.Run(ctx, session); err != nil {
		return "", err
	}
	continuation, err := batch.Execution()
	if err != nil {
		return "", err
	}
	final, err := fixture.Client.Execute(ctx, continuation)
	if err != nil {
		return "", fmt.Errorf("caller continuation: %w", err)
	}
	if final.Outcome != prompty.OutcomeCompleted {
		return "", prompty.ErrResponseOutcome
	}
	text, err := final.StrictText()
	if err != nil {
		return "", fmt.Errorf("caller final text: %w", err)
	}
	return text, nil
}

// DecodeRequests decodes raw requests without losing dynamic integer precision.
func DecodeRequests(raw []byte) (map[string]any, error) {
	var out map[string]any
	// SDK arguments/results are JSON strings; callers keep those bytes separately.
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	err := decoder.Decode(&out)
	if err != nil {
		return nil, fmt.Errorf("fixture request: %w", err)
	}
	return out, nil
}
