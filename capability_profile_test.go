package prompty

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func validCapabilityProfileFixture() CapabilityProfile {
	var profile CapabilityProfile
	profile.Identity = ProfileIdentity{Provider: "fixture", Endpoint: "fixture.endpoint", Model: "fixture.model"}
	profile.Provenance = "caller conformance fixture"
	profile.Capabilities = map[string]CapabilitySupport{"caller.extension": CapabilityUnknown}
	return profile
}

func TestMalformedCapabilityProfileRuntimeAndWire(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name   string
		mutate func(*CapabilityProfile)
	}{
		{"provider", func(p *CapabilityProfile) { p.Identity.Provider = " " }},
		{"endpoint", func(p *CapabilityProfile) { p.Identity.Endpoint = "" }},
		{"model", func(p *CapabilityProfile) { p.Identity.Model = "\t" }},
		{"provenance", func(p *CapabilityProfile) { p.Provenance = "\n" }},
		{"capability key", func(p *CapabilityProfile) { p.Capabilities[" "] = CapabilitySupported }},
		{"capability state", func(p *CapabilityProfile) { p.Capabilities["caller.extension"] = "private-invalid-value" }},
		{"empty combination", func(p *CapabilityProfile) { p.ForbiddenCombinations = [][]string{{}} }},
		{"blank combination", func(p *CapabilityProfile) { p.ForbiddenCombinations = [][]string{{" "}} }},
		{"duplicate combination member", func(p *CapabilityProfile) { p.ForbiddenCombinations = [][]string{{"tools", "tools"}} }},
		{"input MIME", func(p *CapabilityProfile) { p.InputMIME = []string{" "} }},
		{"duplicate MIME", func(p *CapabilityProfile) { p.OutputMIME = []string{"text/plain", "text/plain"} }},
		{"schema null", func(p *CapabilityProfile) { p.ModelOptionsSchema = JSONDocument(`null`) }},
		{"schema array", func(p *CapabilityProfile) { p.ModelOptionsSchema = JSONDocument(`[]`) }},
		{"schema corrupt", func(p *CapabilityProfile) { p.ModelOptionsSchema = JSONDocument(`{"private":"invalid"`) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: invalid profile data should not be serialized as checked intent.
			profile := validCapabilityProfileFixture()
			fixture.mutate(&profile)
			execution := SimplePrompt("private prompt")
			execution.Requirements.Override = &profile
			var wire PromptExecutionWire
			wire.Format = ExecutionWireFormat
			wire.Requirements.Override = &profile
			// Act.
			validateErr := ValidateCapabilityProfile(profile)
			_, marshalErr := MarshalExecution(execution, WirePolicy{State: StatePreserve})
			_, restoreErr := UnmarshalExecution(wire)
			// Assert: field classifications only, no raw caller values.
			for _, err := range []error{validateErr, marshalErr, restoreErr} {
				require.ErrorIs(t, err, ErrCapabilityProfile)
				require.NotContains(t, err.Error(), "private")
			}
		})
	}
}

func TestValidCapabilityProfileWireRoundTrip(t *testing.T) {
	t.Parallel()
	for _, schema := range []JSONDocument{nil, JSONDocument(`true`), JSONDocument(`false`), JSONDocument(`{"type":"object"}`)} {
		t.Run(string(schema), func(t *testing.T) {
			t.Parallel()
			// Arrange: extension capabilities and explicit unknown knowledge remain portable.
			profile := validCapabilityProfileFixture()
			profile.ModelOptionsSchema = schema
			execution := SimplePrompt("fixture")
			execution.Requirements.Override = &profile
			// Act.
			wire, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
			require.NoError(t, err)
			encoded, err := json.Marshal(wire)
			require.NoError(t, err)
			restored, err := DecodeExecutionJSON(encoded)
			// Assert: no normalization or hidden catalog lookup.
			require.NoError(t, err)
			require.Equal(t, profile, *restored.Requirements.Override)
			restored.Requirements.Override.Capabilities["caller.extension"] = CapabilitySupported
			require.Equal(t, CapabilityUnknown, profile.Capabilities["caller.extension"])
		})
	}
}

func TestExecutionWireSchemaRejectsMalformedProfileKnowledge(t *testing.T) {
	t.Parallel()
	for _, fixture := range []struct {
		name   string
		mutate func(*CapabilityProfile)
	}{
		{"blank identity", func(p *CapabilityProfile) { p.Identity.Model = "\t" }},
		{"blank provenance", func(p *CapabilityProfile) { p.Provenance = " " }},
		{"blank capability", func(p *CapabilityProfile) { p.Capabilities[" "] = CapabilityUnknown }},
		{"invalid state", func(p *CapabilityProfile) { p.Capabilities["caller.extension"] = "invalid" }},
		{"empty combination", func(p *CapabilityProfile) { p.ForbiddenCombinations = [][]string{{}} }},
		{"duplicate combination", func(p *CapabilityProfile) { p.ForbiddenCombinations = [][]string{{"tools", "tools"}} }},
		{"blank MIME", func(p *CapabilityProfile) { p.InputMIME = []string{" "} }},
		{"invalid schema shape", func(p *CapabilityProfile) { p.ModelOptionsSchema = JSONDocument(`null`) }},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			t.Parallel()
			// Arrange: mutate already serialized intent as an external persisted payload would.
			profile := validCapabilityProfileFixture()
			execution := SimplePrompt("fixture")
			execution.Requirements.Override = &profile
			wire, err := MarshalExecution(execution, WirePolicy{State: StatePreserve})
			require.NoError(t, err)
			fixture.mutate(wire.Requirements.Override)
			encoded, err := json.Marshal(wire)
			require.NoError(t, err)
			// Act.
			_, decodeErr := DecodeExecutionJSON(encoded)
			// Assert: executable wire contract protects JSON consumers too.
			require.ErrorIs(t, decodeErr, ErrSchemaMismatch)
		})
	}
}

func TestCloneCapabilityProfilePreservesShapeAndIsolatesNestedCollections(t *testing.T) {
	t.Parallel()
	// Arrange: every mutable profile field is populated, including nested combinations.
	profile := validCapabilityProfileFixture()
	profile.ForbiddenCombinations = [][]string{{"tools", "caller.extension"}}
	profile.InputMIME = []string{"image/png"}
	profile.OutputMIME = []string{"text/plain"}
	profile.ModelOptionsSchema = JSONDocument(`{"type":"object"}`)
	// Act.
	clone := CloneCapabilityProfile(profile)
	clone.Capabilities["caller.extension"] = CapabilitySupported
	clone.ForbiddenCombinations[0][0] = "changed"
	clone.ForbiddenCombinations = append(clone.ForbiddenCombinations, []string{"changed"})
	clone.InputMIME[0], clone.OutputMIME[0] = "changed", "changed"
	clone.ModelOptionsSchema[0] = '['
	// Assert: mutation cannot change the original immutable knowledge snapshot.
	require.Equal(t, CapabilityUnknown, profile.Capabilities["caller.extension"])
	require.Equal(t, [][]string{{"tools", "caller.extension"}}, profile.ForbiddenCombinations)
	require.Equal(t, []string{"image/png"}, profile.InputMIME)
	require.Equal(t, []string{"text/plain"}, profile.OutputMIME)
	require.Equal(t, JSONDocument(`{"type":"object"}`), profile.ModelOptionsSchema)
	require.Nil(t, CloneCapabilityProfile(validCapabilityProfileFixture()).ForbiddenCombinations)
}
