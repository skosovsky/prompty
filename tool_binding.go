package prompty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

// ToolHandler executes a typed tool with decoded arguments.
type ToolHandler[TArgs any, TResult any] interface {
	Name() string
	Handle(ctx context.Context, args TArgs) (TResult, error)
}

// TypedTool binds a handler to JSON Schema derived from TArgs.
type TypedTool[TArgs any, TResult any] struct {
	name      string
	handler   func(context.Context, TArgs) (TResult, error)
	schema    JSONDocument
	dialect   string
	validator ValueValidator
}

// NewTypedTool registers a tool with schema inferred from TArgs.
func NewTypedTool[TArgs any, TResult any](
	name string,
	handler func(context.Context, TArgs) (TResult, error),
) (*TypedTool[TArgs, TResult], error) {
	if strings.TrimSpace(name) == "" {
		return nil, errors.New("typed tool: name is required")
	}
	if handler == nil {
		return nil, errors.New("typed tool: handler is nil")
	}
	var zero TArgs
	schemaMap, err := ExtractSchema(zero)
	if err != nil {
		return nil, fmt.Errorf("typed tool %q: %w", name, err)
	}
	schemaDoc, err := MapToJSONDocument(schemaMap)
	if err != nil {
		return nil, fmt.Errorf("typed tool %q: %w", name, err)
	}
	compiled, err := NewJSONSchemaValidator(nil).CompileSchema(schemaDoc, schemaDeclaredDialect(schemaMap))
	if err != nil {
		return nil, fmt.Errorf("typed tool %q: %w", name, err)
	}
	return &TypedTool[TArgs, TResult]{
		name:      name,
		handler:   handler,
		schema:    schemaDoc,
		dialect:   schemaDeclaredDialect(schemaMap),
		validator: compiled,
	}, nil
}

// Name returns the tool name exposed to the model.
func (t *TypedTool[TArgs, TResult]) Name() string {
	if t == nil {
		return ""
	}
	return t.name
}

// Definition returns the tool definition for PromptExecution.
func (t *TypedTool[TArgs, TResult]) Definition() ToolDefinition {
	return ToolDefinition{
		Name:         t.name,
		Description:  "",
		Dialect:      t.dialect,
		Parameters:   CloneJSONDocument(t.schema),
		Capabilities: nil,
	}
}

// ValidateArgs parses tool call JSON into TArgs without invoking the handler.
func (t *TypedTool[TArgs, TResult]) ValidateArgs(argsJSON string) error {
	if t == nil {
		return errors.New("typed tool: nil")
	}
	if err := t.validator.ValidateJSON(JSONDocument(argsJSON)); err != nil {
		return err
	}
	_, err := DecodeToolArgs[TArgs](argsJSON)
	return err
}

// DecodeToolArgs parses tool call JSON into TArgs with unknown-field rejection.
// Dynamic interface/map values retain [json.Number]; explicit numeric DTO fields
// retain the caller's selected Go conversion semantics.
func DecodeToolArgs[TArgs any](argsJSON string) (TArgs, error) {
	var out TArgs
	dec := json.NewDecoder(bytes.NewReader([]byte(argsJSON)))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, fmt.Errorf("typed tool args: %w", err)
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return out, errors.New("typed tool args: trailing JSON after first value")
		}
		return out, fmt.Errorf("typed tool args: %w", err)
	}
	return out, nil
}

// Invoke decodes args and runs the handler (fail-closed on unknown JSON fields).
func (t *TypedTool[TArgs, TResult]) Invoke(ctx context.Context, argsJSON string) (TResult, error) {
	result, _, err := t.invoke(ctx, argsJSON)
	return result, err
}
func (t *TypedTool[TArgs, TResult]) invoke(ctx context.Context, argsJSON string) (TResult, bool, error) {
	var zero TResult
	if t == nil {
		return zero, false, errors.New("typed tool: nil")
	}
	if err := ctx.Err(); err != nil {
		return zero, false, err
	}
	if err := t.ValidateArgs(argsJSON); err != nil {
		return zero, false, err
	}
	args, err := DecodeToolArgs[TArgs](argsJSON)
	if err != nil {
		return zero, false, err
	}
	if err := ctx.Err(); err != nil {
		return zero, false, err
	}
	result, handlerErr := t.handler(ctx, args)
	return result, true, errors.Join(handlerErr, ctx.Err())
}

// TypedToolRegistry validates and dispatches tool calls for registered typed tools.
// Complete registration before concurrent invocation; calls do not retain contexts.
type TypedToolRegistry struct {
	byName      map[string]toolInvoker
	definitions []ToolDefinition
}

type toolInvoker interface {
	name() string
	validateArgs(argsJSON string) error
	invoke(ctx context.Context, argsJSON string) (json.RawMessage, error)
}

type typedToolInvoker[TArgs any, TResult any] struct {
	tool *TypedTool[TArgs, TResult]
}

func (w typedToolInvoker[TArgs, TResult]) name() string { return w.tool.Name() }

func (w typedToolInvoker[TArgs, TResult]) validateArgs(argsJSON string) error {
	return w.tool.ValidateArgs(argsJSON)
}

func (w typedToolInvoker[TArgs, TResult]) invoke(ctx context.Context, argsJSON string) (json.RawMessage, error) {
	result, started, err := w.tool.invoke(ctx, argsJSON)
	if !started {
		return nil, err
	}
	raw, encodeErr := encodeToolResult(result)
	return raw, errors.Join(err, encodeErr)
}

// NewTypedToolRegistry creates an empty registry.
func NewTypedToolRegistry() *TypedToolRegistry {
	return &TypedToolRegistry{
		byName:      make(map[string]toolInvoker),
		definitions: make([]ToolDefinition, 0),
	}
}

// RegisterTool adds a typed tool to the registry.
func RegisterTool[TArgs any, TResult any](r *TypedToolRegistry, tool *TypedTool[TArgs, TResult]) error {
	if r == nil {
		return errors.New("typed tool registry: nil")
	}
	if tool == nil {
		return errors.New("typed tool registry: nil tool")
	}
	if _, exists := r.byName[tool.Name()]; exists {
		return fmt.Errorf("typed tool registry: duplicate tool %q", tool.Name())
	}
	r.byName[tool.Name()] = typedToolInvoker[TArgs, TResult]{tool: tool}
	r.definitions = append(r.definitions, tool.Definition())
	return nil
}

// ValidateToolCall implements ToolValidator with strict JSON decoding (no handler execution).
func (r *TypedToolRegistry) ValidateToolCall(name string, argsJSON string) error {
	if r == nil {
		return errors.New("typed tool registry: nil")
	}
	inv, ok := r.byName[name]
	if !ok {
		return fmt.Errorf("typed tool registry: unknown tool %q", name)
	}
	return inv.validateArgs(argsJSON)
}

// InvokeTool decodes args and executes the registered handler.
func (r *TypedToolRegistry) InvokeTool(ctx context.Context, name string, argsJSON string) (json.RawMessage, error) {
	if r == nil {
		return nil, errors.New("typed tool registry: nil")
	}
	inv, ok := r.byName[name]
	if !ok {
		return nil, fmt.Errorf("typed tool registry: unknown tool %q", name)
	}
	return inv.invoke(ctx, argsJSON)
}

func encodeToolResult[T any](result T) (json.RawMessage, error) {
	switch v := any(result).(type) {
	case nil:
		return nil, nil
	case json.RawMessage:
		if len(v) > 0 && !json.Valid(v) {
			return nil, errors.New("typed tool: tool result JSON is invalid")
		}
		return v, nil
	case string:
		return json.Marshal(v)
	case []byte:
		if !json.Valid(v) {
			return nil, errors.New("typed tool: tool result bytes are not valid JSON")
		}
		return json.RawMessage(v), nil
	default:
		data, err := json.Marshal(v)
		if err != nil {
			return nil, fmt.Errorf("typed tool: marshal result: %w", err)
		}
		return data, nil
	}
}

// Definitions returns tool definitions for all registered tools.
func (r *TypedToolRegistry) Definitions() []ToolDefinition {
	if r == nil {
		return nil
	}
	return cloneToolDefinitions(r.definitions)
}
