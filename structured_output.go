package prompty

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"reflect"
	"strings"
)

const semanticFeedbackTemplate = "The JSON format is valid, but data violates business rules: %v. Fix it."

// Validatable allows caller-owned types to enforce post-unmarshal business rules.
type Validatable interface {
	Validate() error
}

// ExecuteWithStructuredOutput performs a single request to the LLM and parses the response as JSON into type T.
func ExecuteWithStructuredOutput[T any](
	ctx context.Context,
	invoker Invoker,
	exec *PromptExecution,
) (*T, error) {
	if invoker == nil {
		return nil, errors.New("structured output: invoker is nil")
	}

	workExec, err := prepareStructuredExecution[T](exec)
	if err != nil {
		return nil, err
	}
	resp, err := invoker.Execute(ctx, workExec)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("structured output: nil response")
	}

	assistantMsg := newAssistantMessageWithContent(resp.Content)
	rawText, err := resp.StrictText()
	if err != nil {
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      validationFeedbackText(err),
			Err:                 err,
		}
	}
	result, err := decodeStructuredOutput[T](rawText)
	if err != nil {
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      validationFeedbackText(err),
			Err:                 err,
		}
	}
	if err := validateStructuredValue(result); err != nil {
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      semanticFeedbackText(err),
			Err:                 err,
		}
	}
	return result, nil
}

func prepareStructuredExecution[T any](exec *PromptExecution) (*PromptExecution, error) {
	if exec == nil {
		return nil, errors.New("structured output: execution is nil")
	}

	workExec := clonePromptExecution(exec)
	if workExec.ResponseFormat != nil {
		return workExec, nil
	}

	schema, err := schemaForStructuredType[T]()
	if err != nil {
		return nil, fmt.Errorf("structured output: %w", err)
	}
	schemaDoc, err := MapToJSONDocument(schema)
	if err != nil {
		return nil, fmt.Errorf("structured output: %w", err)
	}
	workExec.ResponseFormat = &SchemaDefinition{
		Schema: schemaDoc,
	}
	return workExec, nil
}

func schemaForStructuredType[T any]() (map[string]any, error) {
	return extractSchemaFromType(reflect.TypeFor[T]())
}

func decodeStructuredOutput[T any](raw string) (*T, error) {
	var result T
	rawText := strings.TrimSpace(raw)
	dec := json.NewDecoder(bytes.NewReader([]byte(rawText)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&result); err != nil {
		if hasMarkdownCodeFence(rawText) {
			return nil, errors.New("structured output: markdown code fences are not supported (JSON-only)")
		}
		return nil, err
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		if err == nil {
			return nil, errors.New("structured output: trailing JSON after document")
		}
		return nil, err
	}
	if isNilStructuredValue(result) {
		return nil, errors.New("structured output: decoded nil result")
	}
	return &result, nil
}

func hasMarkdownCodeFence(raw string) bool {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "```") {
		return true
	}
	for _, marker := range []string{"```json", "```JSON", "```yaml", "```yml"} {
		if strings.Contains(raw, marker) {
			return true
		}
	}
	return false
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
		return errors.New("structured output: decoded nil result")
	}
	value := reflect.ValueOf(result).Elem()
	if !value.IsValid() {
		return nil
	}
	if value.Kind() == reflect.Pointer && value.IsNil() {
		return errors.New("structured output: decoded nil result")
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

func validationFeedbackText(validationError error) string {
	return fmt.Sprintf("JSON validation failed: %v. Please fix your output.", validationError)
}

func semanticFeedbackText(validationError error) string {
	return fmt.Sprintf(semanticFeedbackTemplate, validationError)
}
