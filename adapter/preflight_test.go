package adapter

import (
	"context"
	"iter"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
)

type checkedFixture struct {
	descriptor   RuntimeDescriptor
	translations int
	calls        int
}

func TestMalformedSchemaPolicyFailsBeforeTranslationAndDispatch(t *testing.T) {
	t.Parallel()
	// Arrange.
	fixture := &checkedFixture{descriptor: fixtureDescriptor()}
	exec := prompty.SimplePrompt("fixture")
	exec.Requirements.Schema = prompty.SchemaPolicy{Mode: prompty.SchemaLowering, AllowedTransforms: []string{" "}}
	// Act.
	_, err := NewClient(fixture).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrSchemaUnsupported)
	require.Zero(t, fixture.translations)
	require.Zero(t, fixture.calls)
}

func (a *checkedFixture) RuntimeDescriptor() RuntimeDescriptor { return a.descriptor }
func (*checkedFixture) MapSchema(request prompty.SchemaMappingRequest) (prompty.SchemaMapping, error) {
	return prompty.SchemaMapping{Schema: prompty.CloneJSONDocument(request.Source)}, nil
}
func (a *checkedFixture) Translate(exec *prompty.PromptExecution) (*prompty.PromptExecution, error) {
	a.translations++
	return exec, nil
}
func (a *checkedFixture) Execute(context.Context, *prompty.PromptExecution) (*prompty.Response, error) {
	a.calls++
	return prompty.NewResponse([]prompty.ContentPart{prompty.TextPart{Text: "ok"}}), nil
}
func (*checkedFixture) ParseResponse(result *prompty.Response) (*prompty.Response, error) {
	return result, nil
}

type nativeCheckedFixture struct{ *checkedFixture }

func (a nativeCheckedFixture) StreamEvents(
	context.Context,
	*prompty.PromptExecution,
) iter.Seq2[*prompty.ResponseChunk, error] {
	return func(yield func(*prompty.ResponseChunk, error) bool) {
		a.calls++
		yield(&prompty.ResponseChunk{Kind: prompty.StreamFinish, Outcome: prompty.OutcomeCompleted}, nil)
	}
}

func fixtureDescriptor() RuntimeDescriptor {
	identity := prompty.ProfileIdentity{Provider: "fixture", Endpoint: "https://fixture.invalid", Model: "known"}
	return RuntimeDescriptor{Name: identity.Provider, Endpoint: identity.Endpoint, DefaultModel: identity.Model,
		Profiles: []prompty.CapabilityProfile{{Identity: identity, Provenance: "AAA fixture",
			Capabilities: map[string]prompty.CapabilitySupport{
				prompty.CapabilityTools:            prompty.CapabilitySupported,
				prompty.CapabilityStructuredOutput: prompty.CapabilitySupported,
				prompty.CapabilityNativeStream:     prompty.CapabilitySupported,
			}, InputMIME: []string{"image/png"}, OutputMIME: []string{"text/plain"},
			ForbiddenCombinations: [][]string{{prompty.CapabilityTools, prompty.CapabilityStructuredOutput}},
		}},
	}
}

func TestCheckedExecutionFailsBeforeTranslateOrDispatch(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		modify func(*prompty.PromptExecution)
		want   error
	}{
		{"combination", func(e *prompty.PromptExecution) {
			e.Tools = []prompty.ToolDefinition{{Name: "lookup"}}
			e.ResponseFormat = &prompty.SchemaDefinition{}
		}, ErrUnsupportedCombination},
		{"unknown model", func(e *prompty.PromptExecution) {
			e.ModelOptions = &prompty.ModelOptions{Model: "unknown"}
			e.Requirements.Capabilities = []string{prompty.CapabilityTools}
		}, ErrUnknownProfile},
		{"input MIME", func(e *prompty.PromptExecution) {
			e.Messages[0].Content = []prompty.ContentPart{prompty.MediaPart{MIMEType: "video/mp4"}}
		}, ErrUnsupportedCapability},
		{
			"output MIME",
			func(e *prompty.PromptExecution) { e.Requirements.OutputMIME = []string{"audio/wav"} },
			ErrUnsupportedCapability,
		},
		{"invalid override", func(e *prompty.PromptExecution) {
			p := fixtureDescriptor().Profiles[0]
			p.Identity.Endpoint = "https://other.invalid"
			e.Requirements.Override = &p
		}, ErrInvalidProfileOverride},
		{"native absent", func(e *prompty.PromptExecution) {
			e.Requirements.Capabilities = []string{prompty.CapabilityNativeStream}
		}, ErrUnsupportedCapability},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			// Arrange.
			a := &checkedFixture{descriptor: fixtureDescriptor()}
			exec := prompty.SimplePrompt("private input must not appear in diagnostics")
			tt.modify(exec)
			// Act.
			_, err := NewClient(a).Execute(context.Background(), exec)
			// Assert.
			require.ErrorIs(t, err, tt.want)
			require.ErrorIs(t, err, ErrCapabilityRequirement)
			require.NotContains(t, err.Error(), "private input")
			require.Zero(t, a.translations)
			require.Zero(t, a.calls)
		})
	}
}

func TestNativeOverrideChecksEffectiveModelAndImplementation(t *testing.T) {
	t.Parallel()
	// Arrange.
	a := &checkedFixture{descriptor: fixtureDescriptor()}
	exec := prompty.SimplePrompt("fixture")
	exec.ModelOptions = &prompty.ModelOptions{Model: "unknown"}
	p := fixtureDescriptor().Profiles[0]
	p.Identity.Model = "unknown"
	exec.Requirements.Override = &p
	// Act: an override supplies knowledge, never an implementation.
	_, absentErr := Prepare(a, exec, prompty.StreamNative)
	stream := NewClient(nativeCheckedFixture{a}).ExecuteStream(context.Background(), exec, prompty.StreamNative)
	p.Capabilities[prompty.CapabilityNativeStream] = prompty.CapabilityUnsupported // handle already cloned
	for _, err := range stream.Events() {
		require.NoError(t, err)
	}
	status := stream.Status()
	// Assert.
	require.ErrorIs(t, absentErr, ErrUnsupportedCapability)
	require.Equal(t, 1, a.calls)
	require.True(t, status.Result.Diagnostics.Override)
	require.Equal(t, "unknown", status.Result.Diagnostics.Profile.Model)
	require.Equal(t, "AAA fixture", status.Result.Diagnostics.Provenance)
	status.Result.Diagnostics.Checks[0].Status = prompty.CapabilityUnsupported
	require.Equal(t, prompty.CapabilitySupported, stream.Status().Result.Diagnostics.Checks[0].Status)
}

func TestNativeRequirementCannotBeSatisfiedByBufferedOrSync(t *testing.T) {
	t.Parallel()
	// Arrange.
	a := nativeCheckedFixture{&checkedFixture{descriptor: fixtureDescriptor()}}
	exec := prompty.SimplePrompt("fixture")
	exec.Requirements.Capabilities = []string{prompty.CapabilityNativeStream}
	client := NewClient(a)
	// Act.
	_, syncErr := client.Execute(context.Background(), exec)
	stream := client.ExecuteStream(context.Background(), exec, prompty.StreamBuffered)
	for _, err := range stream.Events() {
		require.ErrorIs(t, err, ErrUnsupportedCapability)
	}
	// Assert.
	require.ErrorIs(t, syncErr, ErrUnsupportedCapability)
	require.Zero(t, a.calls)
	require.Zero(t, a.translations)
}

func TestPreparedSnapshotIsDeterministicAndCloned(t *testing.T) {
	t.Parallel()
	// Arrange.
	a := &checkedFixture{descriptor: fixtureDescriptor()}
	exec := prompty.SimplePrompt("fixture")
	exec.Requirements.Capabilities = []string{prompty.CapabilityTools}
	// Act.
	first, err := Prepare(a, exec, "")
	require.NoError(t, err)
	second, err := Prepare(a, exec, "")
	require.NoError(t, err)
	report := first.Diagnostics()
	report.Checks[0].Status = prompty.CapabilityUnsupported
	a.descriptor.Profiles[0].Capabilities[prompty.CapabilityTools] = prompty.CapabilityUnsupported
	// Assert.
	require.Equal(t, first.Diagnostics().Snapshot, second.Diagnostics().Snapshot)
	require.Equal(t, prompty.CapabilitySupported, first.Diagnostics().Checks[0].Status)
}

func TestStateOverrideCannotImplementMissingTranslator(t *testing.T) {
	t.Parallel()
	// Arrange.
	desc := fixtureDescriptor()
	desc.Profiles[0].Capabilities[prompty.CapabilityProviderState] = prompty.CapabilitySupported
	a := &checkedFixture{descriptor: desc}
	exec := prompty.SimplePrompt("fixture")
	exec.Messages[0].ProviderState = []prompty.PartState{{PartIndex: 0, Envelope: prompty.ProviderEnvelope{
		Format:   prompty.ProviderStateFormat,
		Scope:    desc.Profiles[0].Identity,
		Codec:    "fixture",
		Payload:  []byte("opaque"),
		Required: true,
	}}}
	// Act.
	_, err := NewClient(a).Execute(context.Background(), exec)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrProviderState)
	require.Zero(t, a.translations)
	require.Zero(t, a.calls)
}
