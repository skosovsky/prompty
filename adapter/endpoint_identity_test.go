package adapter

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestUnsafeDescriptorEndpointNeverLeaksOrDispatches(t *testing.T) {
	t.Parallel()
	for _, endpoint := range []string{"https://private:secret@fixture.invalid", "https://fixture.invalid?private=secret", "https://fixture.invalid#private-secret"} {
		for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
			// Arrange: descriptor endpoint is invalid even without a selected profile.
			descriptor := fixtureDescriptor()
			descriptor.Endpoint = endpoint
			fixture := &checkedFixture{descriptor: descriptor}
			execution := prompty.SimplePrompt("private prompt")
			// Act.
			diagnostics, preflightErr := Preflight(descriptor, execution, mode, false)
			client := NewClient(fixture)
			var err error
			if mode == "" {
				_, err = client.Execute(context.Background(), execution)
			} else {
				for _, streamErr := range client.ExecuteStream(context.Background(), execution, mode).Events() {
					err = streamErr
				}
			}
			// Assert: refusal before diagnostics assignment, translation or transport.
			require.ErrorIs(t, preflightErr, ErrInvalidProfile)
			require.ErrorIs(t, err, prompty.ErrCapabilityProfile)
			encoded, encodeErr := json.Marshal(diagnostics)
			require.NoError(t, encodeErr)
			require.NotContains(t, string(encoded), "private")
			require.NotContains(t, err.Error(), "secret")
			require.Zero(t, fixture.translations)
			require.Zero(t, fixture.calls)
		}
	}
}

func TestRuntimeDescriptorDoesNotExposeCredentialEndpoints(t *testing.T) {
	t.Parallel()
	for _, nested := range []bool{false, true} {
		// Arrange: every returned identity must be safe, including unrelated metadata.
		descriptor := fixtureDescriptor()
		const unsafe = "https://private:secret@fixture.invalid"
		if nested {
			descriptor.Profiles[0].Identity.Endpoint = unsafe
		} else {
			descriptor.Endpoint = unsafe
		}
		fixture := &checkedFixture{descriptor: descriptor}
		// Act.
		result, err := NewRuntimeClient(fixture).RuntimeDescriptor()
		// Assert.
		require.ErrorIs(t, err, ErrInvalidProfile)
		require.Empty(t, result.Name)
		require.Empty(t, result.Endpoint)
		require.Empty(t, result.Profiles)
		require.NotContains(t, err.Error(), "private")
		require.Zero(t, fixture.calls)
	}
}
