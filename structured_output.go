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
	validator, err := compileSchemaSnapshot(
		workExec.Validator,
		workExec.ResponseFormat.Schema,
		workExec.ResponseFormat.Dialect,
	)
	if err != nil {
		return nil, err
	}
	if validator == nil {
		return nil, ErrSchemaValidatorRequired
	}
	if boundaryErr := validateDispatchBoundary(ctx, workExec); boundaryErr != nil {
		return nil, boundaryErr
	}
	resp, err := invoker.Execute(ctx, workExec.Clone())
	if err != nil {
		return nil, err
	}
	if resp == nil {
		return nil, errors.New("structured output: nil response")
	}
	if metadataErr := ValidateResponseMetadata(resp); metadataErr != nil {
		return nil, metadataErr
	}
	if resp.Outcome != OutcomeCompleted {
		return nil, &OutcomeError{Outcome: resp.Outcome, FinishReason: resp.FinishReason}
	}

	assistantMsg, err := resp.AssistantMessage()
	if err != nil {
		return nil, err
	}
	rawText, err := resp.StrictText()
	if err != nil {
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      validationFeedbackText(err),
			Err:                 err,
		}
	}
	if validationErr := validator.ValidateJSON(JSONDocument(rawText)); validationErr != nil {
		if errors.Is(validationErr, ErrSchemaJSONDecode) {
			return nil, validationErr
		}
		return nil, &ValidationError{
			RawAssistantMessage: &assistantMsg,
			FeedbackPrompt:      validationFeedbackText(validationErr),
			Err:                 validationErr,
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
	if scopeErr := ValidateExecutionToolScope(workExec); scopeErr != nil {
		return nil, scopeErr
	}
	if workExec.Validator == nil {
		workExec.Validator = NewJSONSchemaValidator(nil)
	}
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
	workExec.ResponseFormat = &SchemaDefinition{Name: "", Description: "",
		Schema:  schemaDoc,
		Dialect: schemaDeclaredDialect(schema),
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
	dec.UseNumber()
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
	case reflect.Invalid,
		reflect.Bool,
		reflect.Int,
		reflect.Int8,
		reflect.Int16,
		reflect.Int32,
		reflect.Int64,
		reflect.Uint,
		reflect.Uint8,
		reflect.Uint16,
		reflect.Uint32,
		reflect.Uint64,
		reflect.Uintptr,
		reflect.Float32,
		reflect.Float64,
		reflect.Complex64,
		reflect.Complex128,
		reflect.Array,
		reflect.Chan,
		reflect.Func,
		reflect.String,
		reflect.Struct,
		reflect.UnsafePointer:
		fallthrough
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
		if validatable, ok := reflect.TypeAssert[Validatable](value); ok {
			return validatable, true
		}
	}
	if value.Kind() != reflect.Pointer && value.CanAddr() && value.Addr().CanInterface() {
		if validatable, ok := reflect.TypeAssert[Validatable](value.Addr()); ok {
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
