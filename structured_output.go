package prompty

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

const semanticRetryTemplate = "The JSON format is valid, but data violates business rules: %v. Fix it."

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

	end, ok := findClosingFenceOffset(s[start:])
	if !ok {
		return "", false
	}
	return strings.TrimSpace(s[start : start+end]), true
}

func findClosingFenceOffset(s string) (int, bool) {
	offset := 0
	for {
		line := s[offset:]
		lineEnd := strings.IndexByte(line, '\n')
		segment := line
		if lineEnd >= 0 {
			segment = line[:lineEnd]
		}
		if strings.TrimSpace(segment) == "```" {
			return offset, true
		}
		if lineEnd < 0 {
			return 0, false
		}
		offset += lineEnd + 1
	}
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
	resp, err := invoker.Generate(ctx, workExec)
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("structured output: nil response")
	}

	assistantMsg := newAssistantMessageWithContent(resp.Content)
	result, err := decodeStructuredOutput[T](resp.Text())
	if err != nil {
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      validationRetryFeedbackText(err),
			Err:                 err,
		}
	}
	if err := validateStructuredValue(result); err != nil {
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      semanticValidationFeedbackText(err),
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
	workExec.ResponseFormat = &SchemaDefinition{
		Schema: schema,
	}
	return workExec, nil
}

func schemaForStructuredType[T any]() (map[string]any, error) {
	return extractSchemaFromType(reflect.TypeFor[T]())
}

func decodeStructuredOutput[T any](raw string) (*T, error) {
	var result T
	rawText := stripMarkdownJSON(raw)
	if err := json.Unmarshal([]byte(rawText), &result); err != nil {
		return nil, err
	}
	if isNilStructuredValue(result) {
		return nil, errors.New("structured output: decoded nil result")
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

func validationRetryFeedbackText(validationError error) string {
	return fmt.Sprintf("JSON validation failed: %v. Please fix your output.", validationError)
}

func semanticValidationFeedbackText(validationError error) string {
	return fmt.Sprintf(semanticRetryTemplate, validationError)
}
