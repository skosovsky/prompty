package prompty

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPartStateNonblankIdentity(t *testing.T) {
	t.Parallel()
	for _, field := range []string{"provider", "model", "codec", "valid-family", "valid"} {
		t.Run(field, func(t *testing.T) {
			t.Parallel()
			// Arrange: one canonical part with an independently owned envelope.
			parts := []ContentPart{TextPart{Text: "known"}}
			state := PartState{PartIndex: 0, Envelope: ProviderEnvelope{
				Format: ProviderStateFormat, Codec: "fixture/v1", Payload: []byte("state"),
				Scope: ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "known"},
			}}
			switch field {
			case "provider":
				state.Envelope.Scope.Provider = " \t"
			case "model":
				state.Envelope.Scope.Model = " \n"
				state.Envelope.Scope.ModelFamily = "\t"
			case "codec":
				state.Envelope.Codec = " \t"
			case "valid-family":
				state.Envelope.Scope.Model = " "
				state.Envelope.Scope.ModelFamily = "family"
			}
			// Act.
			err := ValidatePartAssociations(parts, []PartState{state}, nil)
			// Assert.
			if field == "valid" || field == "valid-family" {
				require.NoError(t, err)
			} else {
				require.ErrorIs(t, err, ErrProviderState)
			}
		})
	}
}

func TestPartAnnotationNonblankKindParity(t *testing.T) {
	t.Parallel()
	for _, kind := range []string{"", " \t\n", "citation"} {
		t.Run(kind, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			value := JSONDocument(`{"source":7}`)
			// Act.
			partErr := ValidatePartAssociations([]ContentPart{TextPart{Text: "known"}}, nil,
				[]PartAnnotation{{PartIndex: 0, Kind: kind, Value: value}})
			messageErr := ValidateMessageAnnotations(
				[]MessageAnnotation{{Kind: kind, Value: value, Scope: nil, Required: false}},
			)
			// Assert: part and message metadata agree on admission.
			if kind == "citation" {
				require.NoError(t, partErr)
				require.NoError(t, messageErr)
			} else {
				require.ErrorIs(t, partErr, ErrProviderState)
				require.ErrorIs(t, messageErr, ErrProviderState)
			}
		})
	}
}
