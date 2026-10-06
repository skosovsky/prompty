package adapter

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

func TestPreparedSchemaCacheDiagnosticsStayProfileBoundAfterResultFailure(t *testing.T) {
	t.Parallel()
	for _, mode := range []prompty.StreamMode{"", prompty.StreamNative, prompty.StreamBuffered} {
		t.Run(string(mode), func(t *testing.T) {
			t.Parallel()
			// Arrange: selected implicit caching coexists with explicitly lowered output.
			desc := fixtureDescriptor()
			desc.Profiles[0].Capabilities[prompty.CapabilityCacheImplicit] = prompty.CapabilitySupported
			fixture := &nativeLoweringFixture{loweringFixture: &loweringFixture{
				checkedFixture: &checkedFixture{descriptor: desc},
				response:       prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "1"}}),
			}, value: "1"}
			exec := prompty.SimplePrompt("fixture")
			exec.ResponseFormat = &prompty.SchemaDefinition{
				Name: "result", Schema: prompty.JSONDocument(`{"type":"integer","minimum":10}`),
			}
			exec.Requirements.Schema = prompty.SchemaPolicy{
				Mode: prompty.SchemaLowering, AllowedTransforms: []string{"fixture.drop-minimum"},
			}
			exec.Messages[0].CachePolicy = &prompty.CachePolicy{
				Mechanism: prompty.CacheImplicitPrefix, Mode: prompty.CacheRequired, Provider: desc.Name,
			}
			before, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			client := NewClient(fixture)
			// Act: original-schema validation fails after exactly one provider invocation.
			var response *prompty.Response
			if mode == "" {
				response, err = client.Execute(context.Background(), exec)
			} else {
				stream := client.ExecuteStream(context.Background(), exec, mode)
				for range stream.Events() {
				}
				status := stream.Status()
				require.Equal(t, prompty.StreamFailed, status.State)
				response, err = status.Result, status.Err
			}
			// Assert: result failure retains the exact prepared decision and schema binding.
			require.ErrorIs(t, err, prompty.ErrSchemaMismatch)
			require.NotNil(t, response)
			diagnostics := response.Diagnostics
			require.Equal(t, desc.Profiles[0].Identity, diagnostics.Profile)
			require.Equal(t, desc.Profiles[0].Provenance, diagnostics.Provenance)
			require.NotEmpty(t, diagnostics.Snapshot)
			require.Len(t, diagnostics.Cache, 1)
			require.Equal(t, "/messages/0/cache_policy", diagnostics.Cache[0].Path)
			require.Equal(t, prompty.CacheAccepted, diagnostics.Cache[0].Disposition)
			require.Equal(t, prompty.CacheImplicitPrefix, diagnostics.Cache[0].Mechanism)
			require.Len(t, diagnostics.Schemas, 1)
			report := diagnostics.Schemas[0]
			require.Equal(t, diagnostics.Profile, report.Profile)
			require.Equal(t, "result", report.Name)
			require.Equal(t, prompty.SchemaOutput, report.Role)
			require.Equal(t, prompty.SchemaLowering, report.Guarantee)
			require.NotEqual(t, report.SourceDigest, report.TranslatedDigest)
			require.Equal(t, "/minimum", report.Transformations[0].Pointer)
			require.Equal(t, 1, fixture.translations)
			require.Equal(t, 1, fixture.calls)
			after, err := prompty.MarshalExecution(exec, prompty.WirePolicy{State: prompty.StatePreserve})
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
