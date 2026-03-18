package prompty

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
)

var schemaProviderType = reflect.TypeFor[SchemaProvider]()

const semanticRetryTemplate = "The JSON format is valid, but data violates business rules: %v. Fix it."

// SchemaProvider allows caller-owned types to provide a JSON Schema for strict structured output.
type SchemaProvider interface {
	JSONSchema() map[string]any
}

// Validatable allows caller-owned types to enforce post-unmarshal business rules.
type Validatable interface {
	Validate() error
}

// stripMarkdownJSON removes markdown code block wrappers (e.g. ```json ... ```) before JSON parsing.
func stripMarkdownJSON(s string) string {
	s = strings.TrimSpace(s)
	if block, ok := extractFencedBlock(s, "```json", true); ok {
		return block
	}
	if block, ok := extractFencedBlock(s, "```", false); ok {
		return block
	}
	return s
}

func extractFencedBlock(s, marker string, caseInsensitive bool) (string, bool) {
	search := s
	if caseInsensitive {
		search = strings.ToLower(s)
	}
	idx := strings.Index(search, marker)
	if idx < 0 {
		return "", false
	}

	start := idx + len(marker)
	if rest := s[start:]; rest != "" {
		if newline := strings.IndexByte(rest, '\n'); newline >= 0 {
			start += newline + 1
		}
	}

	end := strings.Index(s[start:], "```")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(s[start : start+end]), true
}

// ExecuteWithStructuredOutput performs a request to the LLM and attempts to parse the response as JSON into type T.
// On JSON validation error, it adds a pair of messages to PromptExecution (assistant with the "bad" output
// and user with the error text) and retries up to maxRetries times.
//
// maxRetries is the number of retry attempts on JSON validation error. Total API calls = maxRetries + 1
// (e.g. maxRetries=3 means up to 4 calls). LLM responses wrapped in markdown (```json ... ```)
// are automatically stripped before parsing.
func ExecuteWithStructuredOutput[T any](
	ctx context.Context,
	client LLMClient,
	exec *PromptExecution,
	maxRetries int,
) (*T, error) {
	if client == nil {
		return nil, fmt.Errorf("structured output: client is nil")
	}

	workExec, err := prepareStructuredExecution[T](exec)
	if err != nil {
		return nil, err
	}
	if maxRetries < 0 {
		maxRetries = 0
	}

	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		resp, err := client.Generate(ctx, workExec)
		if err != nil {
			return nil, err
		}
		if resp == nil {
			return nil, fmt.Errorf("structured output: nil response")
		}

		result, err := decodeStructuredOutput[T](resp.Text())
		if err != nil {
			lastErr = err
			workExec = workExec.appendRetryFeedback(resp.Text(), validationRetryFeedbackText(err))
			continue
		}
		if err := validateStructuredValue(result); err != nil {
			lastErr = err
			workExec = workExec.appendRetryFeedback(resp.Text(), fmt.Sprintf(semanticRetryTemplate, err))
			continue
		}
		return result, nil
	}

	return nil, fmt.Errorf("structured output: validation failed after %d retries: %w", maxRetries+1, lastErr)
}

func prepareStructuredExecution[T any](exec *PromptExecution) (*PromptExecution, error) {
	if exec == nil {
		return nil, fmt.Errorf("structured output: execution is nil")
	}

	workExec := clonePromptExecution(exec)
	if workExec.ResponseFormat != nil {
		return workExec, nil
	}

	schema, ok := schemaForType[T]()
	if !ok {
		return workExec, nil
	}
	workExec.ResponseFormat = &SchemaDefinition{
		Schema: cloneMapAny(schema),
	}
	return workExec, nil
}

func schemaForType[T any]() (map[string]any, bool) {
	t := reflect.TypeFor[T]()
	if provider, ok := schemaProviderForType(t); ok {
		return cloneMapAny(provider.JSONSchema()), true
	}
	if t.Kind() != reflect.Pointer {
		if provider, ok := schemaProviderForType(reflect.PointerTo(t)); ok {
			return cloneMapAny(provider.JSONSchema()), true
		}
	}
	return nil, false
}

func schemaProviderForType(t reflect.Type) (SchemaProvider, bool) {
	if !t.Implements(schemaProviderType) {
		return nil, false
	}

	var value reflect.Value
	if t.Kind() == reflect.Pointer {
		value = reflect.New(t.Elem())
	} else {
		value = reflect.New(t).Elem()
	}
	provider, ok := value.Interface().(SchemaProvider)
	return provider, ok
}

func decodeStructuredOutput[T any](raw string) (*T, error) {
	var result T
	rawText := stripMarkdownJSON(raw)
	if err := json.Unmarshal([]byte(rawText), &result); err != nil {
		return nil, err
	}
	if isNilStructuredValue(result) {
		return nil, fmt.Errorf("structured output: decoded nil result")
	}
	return &result, nil
}

func isNilStructuredValue[T any](value T) bool {
	rv := reflect.ValueOf(value)
	if !rv.IsValid() {
		return false
	}
	switch rv.Kind() {
	case reflect.Pointer, reflect.Map, reflect.Slice, reflect.Interface:
		return rv.IsNil()
	default:
		return false
	}
}

func validateStructuredValue[T any](result *T) error {
	if result == nil {
		return fmt.Errorf("structured output: decoded nil result")
	}
	value := reflect.ValueOf(result).Elem()
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return fmt.Errorf("structured output: decoded nil result")
	}
	if validatable, ok := validatableFromValue(value); ok {
		return validatable.Validate()
	}
	return nil
}

func validatableFromValue(value reflect.Value) (Validatable, bool) {
	if !value.IsValid() {
		return nil, false
	}
	if value.CanInterface() {
		if validatable, ok := value.Interface().(Validatable); ok {
			return validatable, true
		}
	}
	if value.Kind() != reflect.Pointer && value.CanAddr() && value.Addr().CanInterface() {
		if validatable, ok := value.Addr().Interface().(Validatable); ok {
			return validatable, true
		}
	}
	return nil, false
}
