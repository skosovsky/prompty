package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEndpointIdentityRejectsURLCredentialComponents(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{
		"https://private:secret@host.invalid/api", "//private:secret@host.invalid/api",
		"https://host.invalid/api?private=secret", "https://host.invalid/api?",
		"https://host.invalid/api#private-secret", "https://host.invalid/api#", "https://host.invalid/%private",
	} {
		t.Run(endpoint, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			profile := validCapabilityProfileFixture()
			profile.Identity.Endpoint = endpoint
			execution := SimplePrompt("fixture")
			execution.Requirements.Override = &profile
			response := stateFixture()
			response.ProviderState[0].Envelope.Scope.Endpoint = endpoint
			// Act.
			identityErr := ValidateEndpointIdentity(endpoint)
			profileErr := ValidateCapabilityProfile(profile)
			_, executionErr := MarshalExecution(execution, WirePolicy{State: StatePreserve})
			_, stateErr := MarshalResponse(response, WirePolicy{State: StatePreserve})
			// Assert: static field classification without echoed input.
			for _, err := range []error{identityErr, profileErr, executionErr} {
				require.ErrorIs(t, err, ErrCapabilityProfile)
				require.NotContains(t, err.Error(), endpoint)
				require.NotContains(t, err.Error(), "private")
			}
			require.ErrorIs(t, stateErr, ErrProviderState)
			require.NotContains(t, stateErr.Error(), endpoint)
		})
	}
}

func TestEndpointIdentityKeepsHostOwnedReferencesUnchanged(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"", "fixture", "urn:endpoint:opaque", "https://host.invalid/api/v1", "https://host.invalid/path%3Fencoded", "https://host.invalid/path@name"} {
		// Arrange/Act: optional empty identity and stable host-owned paths remain supported.
		err := ValidateEndpointIdentity(endpoint)
		// Assert.
		require.NoError(t, err)
	}
}

func TestSchemaCompilationRejectsCredentialEndpointBeforeMapper(t *testing.T) {
	t.Parallel()
	// Arrange: no report or mapper input may receive an authentication-bearing identity.
	request := compileFixtureRequest(`{"type":"object"}`)
	request.Profile.Endpoint = "https://private:secret@host.invalid"
	calls := 0
	mapper := schemaMapperFunc(func(input SchemaMappingRequest) (SchemaMapping, error) {
		calls++
		return SchemaMapping{Schema: input.Source}, nil
	})
	// Act.
	compiled, err := CompileProviderSchema(request, mapper)
	// Assert.
	require.Nil(t, compiled)
	require.ErrorIs(t, err, ErrCapabilityProfile)
	require.NotContains(t, err.Error(), "private")
	require.Zero(t, calls)
}

func TestResponseDiagnosticsRejectCredentialEndpointsOnMarshalAndRestore(t *testing.T) {
	t.Parallel()
	for _, schemaReport := range []bool{false, true} {
		// Arrange: mutate existing wire, not just a checked adapter's diagnostics.
		response := NewResponse([]ContentPart{TextPart{Text: "known"}})
		wire, err := MarshalResponse(response, WirePolicy{State: StatePreserve})
		require.NoError(t, err)
		const endpoint = "https://host.invalid?private=secret"
		if schemaReport {
			response.Diagnostics.Schemas = []SchemaCompilationReport{{Profile: ProfileIdentity{Endpoint: endpoint}}}
		} else {
			response.Diagnostics.Profile.Endpoint = endpoint
		}
		wire.Diagnostics = CloneExecutionDiagnostics(response.Diagnostics)
		// Act.
		_, marshalErr := LoggingResponse(response)
		_, restoreErr := UnmarshalResponse(wire)
		encoded, encodeErr := json.Marshal(wire)
		require.NoError(t, encodeErr)
		schemaErr := ValidateWireJSON(encoded, ResponseWireFormat)
		// Assert: Go gates and executable schema both reject these URL components.
		require.ErrorIs(t, marshalErr, ErrCapabilityProfile)
		require.ErrorIs(t, restoreErr, ErrCapabilityProfile)
		require.Error(t, schemaErr)
		require.NotContains(t, marshalErr.Error(), "secret")
		require.NotContains(t, restoreErr.Error(), "private")
	}
}
