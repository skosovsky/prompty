package adapter

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestRuntimeDescriptorCollectionsAreIsolatedInBothDirections(t *testing.T) {
	t.Parallel()
	// Arrange: populate every mutable descriptor/profile collection.
	descriptor := fixtureDescriptor()
	descriptor.ReasoningSettings = []string{"reasoning_effort"}
	descriptor.Profiles[0].ModelOptionsSchema = prompty.JSONDocument(`{"type":"object"}`)
	fixture := &checkedFixture{descriptor: descriptor}
	client := NewRuntimeClient(fixture)
	original, err := json.Marshal(descriptor)
	require.NoError(t, err)
	// Act: mutate one returned view in all mutable locations.
	first, err := client.RuntimeDescriptor()
	require.NoError(t, err)
	mutateDescriptorCollections(&first)
	second, err := client.RuntimeDescriptor()
	require.NoError(t, err)
	after, err := json.Marshal(second)
	require.NoError(t, err)
	// Assert: returned views cannot alter source or a later view.
	require.JSONEq(t, string(original), string(after))
	source, err := json.Marshal(fixture.descriptor)
	require.NoError(t, err)
	require.JSONEq(t, string(original), string(source))
	// Act: source-side changes also cannot alter the already returned second view.
	mutateDescriptorCollections(&fixture.descriptor)
	retained, err := json.Marshal(second)
	require.NoError(t, err)
	// Assert.
	require.JSONEq(t, string(original), string(retained))
}

func mutateDescriptorCollections(descriptor *RuntimeDescriptor) {
	descriptor.ReasoningSettings[0] = "changed"
	profile := &descriptor.Profiles[0]
	profile.Identity.Model = "changed"
	profile.Capabilities[prompty.CapabilityTools] = prompty.CapabilityUnsupported
	profile.ForbiddenCombinations[0][0] = "changed"
	profile.ForbiddenCombinations = append(profile.ForbiddenCombinations, []string{"changed"})
	profile.InputMIME[0] = "changed"
	profile.OutputMIME[0] = "changed"
	profile.ModelOptionsSchema[0] = 'x'
}
