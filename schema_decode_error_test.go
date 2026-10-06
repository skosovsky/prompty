package prompty

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOriginalSchemaDistinguishesJSONDecodeFromConstraintMismatch(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"", "[", `{"value":}`, `{} {}`, `{} trailing`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			validator, err := NewJSONSchemaValidator(
				nil,
			).CompileSchema(JSONDocument(`{"type":"object","required":["value"]}`), "")
			require.NoError(t, err)
			// Act.
			err = validator.ValidateJSON(JSONDocument(raw))
			// Assert: invalid JSON is not an original constraint mismatch.
			require.ErrorIs(t, err, ErrSchemaJSONDecode)
			require.NotErrorIs(t, err, ErrSchemaMismatch)
			if raw == `{"value":}` {
				var syntax *json.SyntaxError
				require.ErrorAs(t, err, &syntax)
			}
			require.ErrorIs(t, validator.ValidateJSON(JSONDocument(`{}`)), ErrSchemaMismatch)
			require.NoError(t, validator.ValidateJSON(JSONDocument(`{"value":1}`)))
		})
	}
}

func TestStructuredJSONDecodeFailureIsNotRepairableValidation(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{`{"value":`, `{"value":}`, `{} {}`} {
		t.Run(raw, func(t *testing.T) {
			t.Parallel()
			// Arrange: a completed provider outcome cannot turn an invalid document into domain feedback.
			exec := SimplePrompt("fixture")
			exec.ResponseFormat = &SchemaDefinition{Schema: JSONDocument(`{"type":"object"}`)}
			calls := 0
			invoker := &scriptedInvoker{generate: func(context.Context, *PromptExecution) (*Response, error) {
				calls++
				return NewResponse([]ContentPart{TextPart{Text: raw}}), nil
			}}
			// Act.
			value, err := ExecuteWithStructuredOutput[struct{}](context.Background(), invoker, exec)
			// Assert: no ordinary repairable validation wrapper or implicit replay.
			require.Nil(t, value)
			require.ErrorIs(t, err, ErrSchemaJSONDecode)
			require.NotErrorIs(t, err, ErrSchemaMismatch)
			var validation *ValidationError
			require.NotErrorAs(t, err, &validation)
			require.Equal(t, 1, calls)
		})
	}
}

func TestStructuredExecutorDoesNotAppendFeedbackForInvalidDocument(t *testing.T) {
	t.Parallel()
	// Arrange: both explicit attempts return a truncated document, not a domain error.
	var seen []*PromptExecution
	invoker := &scriptedInvoker{generate: func(_ context.Context, exec *PromptExecution) (*Response, error) {
		seen = append(seen, exec.Clone())
		return NewResponse([]ContentPart{TextPart{Text: `{"value":`}}), nil
	}}
	source := SimplePrompt("fixture")
	step := NewStructuredExecutor[struct{}](invoker, source)
	// Act: the caller explicitly initiates both attempts; the closure must not repair/replay.
	_, first := step(context.Background())
	_, second := step(context.Background())
	// Assert.
	require.ErrorIs(t, first, ErrSchemaJSONDecode)
	require.ErrorIs(t, second, ErrSchemaJSONDecode)
	require.Len(t, seen, 2)
	require.Equal(t, source.Messages, seen[0].Messages)
	require.Equal(t, seen[0].Messages, seen[1].Messages)
}
