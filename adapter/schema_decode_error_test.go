package adapter

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestCheckedJSONDecodeFailureKeepsResultAcrossModes(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamBuffered, prompty.StreamNative} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: provider completed transport but returned invalid JSON.
			fixture := &nativeLoweringFixture{
				loweringFixture: &loweringFixture{
					checkedFixture: &checkedFixture{descriptor: fixtureDescriptor()},
					response:       prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: `{"value":}`}}),
				},
				value: `{"value":}`,
			}
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{
				Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`),
			}
			exec.Requirements.Schema = prompty.SchemaPolicy{
				Mode:              prompty.SchemaLowering,
				AllowedTransforms: []string{"fixture.drop-minimum"},
			}
			// Act.
			response, err := executeToolSchemaFixture(t, NewClient(fixture), exec, mode)
			// Assert: decoding does not erase the provider's known outcome or replay.
			require.ErrorIs(t, err, prompty.ErrSchemaJSONDecode)
			require.NotErrorIs(t, err, prompty.ErrSchemaMismatch)
			require.NotErrorIs(t, err, prompty.ErrResponseOutcome)
			require.NotNil(t, response)
			require.Equal(t, prompty.OutcomeCompleted, response.Outcome)
			require.Equal(t, []prompty.ContentPart{prompty.TextPart{Text: `{"value":}`}}, response.Content)
			require.Len(t, response.Diagnostics.Schemas, 1)
			require.Equal(t, 1, fixture.calls)
			require.Equal(t, captureErrorParse, captureStreamErrorKind(err))
			require.Equal(t, captureErrorParse, captureStreamErrorKind(&prompty.ValidationError{Err: err}))
		})
	}
}
