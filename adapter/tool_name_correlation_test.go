package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestNameCorrelationOverrideCannotInventMapper(t *testing.T) {
	t.Parallel()
	for _, override := range []bool{false, true} {
		// Arrange: profile knowledge supports the requirement, but the adapter lacks the port.
		fixture := &checkedFixture{descriptor: fixtureDescriptor()}
		fixture.descriptor.Profiles[0].Capabilities[prompty.CapabilityToolNameCorrelation] = prompty.CapabilitySupported
		execution := prompty.SimplePrompt("fixture")
		execution.Requirements.Capabilities = []string{prompty.CapabilityToolNameCorrelation}
		if override {
			profile := prompty.CloneCapabilityProfile(fixture.descriptor.Profiles[0])
			execution.Requirements.Override = &profile
		}
		// Act.
		_, err := NewClient(fixture).Execute(context.Background(), execution)
		// Assert: no Translate or provider call, regardless of the knowledge source.
		require.ErrorIs(t, err, ErrUnsupportedCapability)
		require.Zero(t, fixture.translations)
		require.Zero(t, fixture.calls)
	}
}
