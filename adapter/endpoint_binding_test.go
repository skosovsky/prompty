package adapter_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	anthropicsdk "github.com/anthropics/anthropic-sdk-go"
	ollamasdk "github.com/ollama/ollama/api"
	openaisdk "github.com/openai/openai-go/v3"
	"github.com/stretchr/testify/require"
	"google.golang.org/genai"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
	anthropicadapter "github.com/skosovsky/prompty/adapter/anthropic"
	geminiadapter "github.com/skosovsky/prompty/adapter/gemini"
	ollamaadapter "github.com/skosovsky/prompty/adapter/ollama"
	openaiadapter "github.com/skosovsky/prompty/adapter/openai"
)

var errBindingFixtureDispatch = errors.New("fixture dispatch observed")

type endpointBindingSpy[Req, Resp any] struct {
	adapter.ProviderAdapter[Req, Resp]

	describer    adapter.RuntimeDescriber
	translations int
	calls        int
}

func (s *endpointBindingSpy[Req, Resp]) RuntimeDescriptor() adapter.RuntimeDescriptor {
	return s.describer.RuntimeDescriptor()
}

func (s *endpointBindingSpy[Req, Resp]) Translate(execution *prompty.PromptExecution) (Req, error) {
	s.translations++
	return s.ProviderAdapter.Translate(execution)
}

func (s *endpointBindingSpy[Req, Resp]) Execute(context.Context, Req) (Resp, error) {
	s.calls++
	var zero Resp
	return zero, errBindingFixtureDispatch
}

func assertEndpointBinding[Req, Resp any](
	t *testing.T, provider adapter.ProviderAdapter[Req, Resp], bound bool,
) {
	t.Helper()
	// Arrange: actual provider descriptor/Translate, with dispatch intercepted before SDK use.
	spy := &endpointBindingSpy[Req, Resp]{ProviderAdapter: provider,
		describer: provider.(adapter.RuntimeDescriber), translations: 0, calls: 0}
	client := adapter.NewClient(spy)
	execution := prompty.SimplePrompt("fixture")
	// Act.
	_, err := client.Execute(context.Background(), execution)
	// Assert: endpoint knowledge is explicit; no capabilities are invented.
	if bound {
		require.ErrorIs(t, err, errBindingFixtureDispatch)
		require.Equal(t, 1, spy.translations)
		require.Equal(t, 1, spy.calls)
		require.Equal(t, "caller.endpoint", spy.RuntimeDescriptor().Endpoint)
		require.Empty(t, spy.RuntimeDescriptor().Profiles)
		return
	}
	require.ErrorIs(t, err, adapter.ErrNoRuntimeDescriptor)
	require.Empty(t, strings.TrimSpace(spy.RuntimeDescriptor().Endpoint))
	require.Zero(t, spy.translations)
	require.Zero(t, spy.calls)
	// An execution knowledge override must not manufacture the missing transport binding.
	var override prompty.CapabilityProfile
	override.Identity = prompty.ProfileIdentity{Provider: spy.RuntimeDescriptor().Name,
		Endpoint: "caller.endpoint", Model: spy.RuntimeDescriptor().DefaultModel}
	override.Provenance = "fixture knowledge"
	execution.Requirements.Override = &override
	_, err = client.Execute(context.Background(), execution)
	require.ErrorIs(t, err, adapter.ErrNoRuntimeDescriptor)
	require.Zero(t, spy.translations)
	require.Zero(t, spy.calls)
	for _, mode := range []prompty.StreamMode{prompty.StreamNative, prompty.StreamBuffered} {
		stream := client.ExecuteStream(context.Background(), execution, mode)
		for _, streamErr := range stream.Events() {
			require.ErrorIs(t, streamErr, adapter.ErrNoRuntimeDescriptor)
		}
		require.Equal(t, prompty.StreamFailed, stream.Status().State)
	}
	require.Zero(t, spy.translations)
	require.Zero(t, spy.calls)
}

func TestInjectedClientsRequireExplicitEndpointBinding(t *testing.T) {
	t.Parallel()
	t.Run("openai", func(t *testing.T) {
		t.Parallel()
		assertEndpointBinding(t, openaiadapter.New(openaiadapter.WithClient(new(openaisdk.Client))), false)
		assertEndpointBinding(t, openaiadapter.New(openaiadapter.WithClient(new(openaisdk.Client)),
			openaiadapter.WithProfiles(" ")), false)
		assertEndpointBinding(t, openaiadapter.New(openaiadapter.WithClient(new(openaisdk.Client)),
			openaiadapter.WithProfiles("caller.endpoint")), true)
		assertEndpointBinding(t, openaiadapter.New(openaiadapter.WithProfiles("caller.endpoint"),
			openaiadapter.WithClient(new(openaisdk.Client))), true)
	})
	t.Run("anthropic", func(t *testing.T) {
		t.Parallel()
		assertEndpointBinding(t, anthropicadapter.New(anthropicadapter.WithClient(new(anthropicsdk.Client))), false)
		assertEndpointBinding(t, anthropicadapter.New(anthropicadapter.WithClient(new(anthropicsdk.Client)),
			anthropicadapter.WithProfiles(" ")), false)
		assertEndpointBinding(t, anthropicadapter.New(anthropicadapter.WithClient(new(anthropicsdk.Client)),
			anthropicadapter.WithProfiles("caller.endpoint")), true)
		assertEndpointBinding(t, anthropicadapter.New(anthropicadapter.WithProfiles("caller.endpoint"),
			anthropicadapter.WithClient(new(anthropicsdk.Client))), true)
	})
	t.Run("gemini", func(t *testing.T) {
		t.Parallel()
		assertEndpointBinding(t, geminiadapter.New(geminiadapter.WithClient(new(genai.Client))), false)
		assertEndpointBinding(t, geminiadapter.New(geminiadapter.WithClient(new(genai.Client)),
			geminiadapter.WithProfiles(" ")), false)
		assertEndpointBinding(t, geminiadapter.New(geminiadapter.WithClient(new(genai.Client)),
			geminiadapter.WithProfiles("caller.endpoint")), true)
		assertEndpointBinding(t, geminiadapter.New(geminiadapter.WithProfiles("caller.endpoint"),
			geminiadapter.WithClient(new(genai.Client))), true)
	})
	t.Run("ollama", func(t *testing.T) {
		t.Parallel()
		assertEndpointBinding(t, ollamaadapter.New(ollamaadapter.WithClient(new(ollamasdk.Client))), false)
		assertEndpointBinding(t, ollamaadapter.New(ollamaadapter.WithClient(new(ollamasdk.Client)),
			ollamaadapter.WithProfiles(" ")), false)
		assertEndpointBinding(t, ollamaadapter.New(ollamaadapter.WithClient(new(ollamasdk.Client)),
			ollamaadapter.WithProfiles("caller.endpoint")), true)
		assertEndpointBinding(t, ollamaadapter.New(ollamaadapter.WithProfiles("caller.endpoint"),
			ollamaadapter.WithClient(new(ollamasdk.Client))), true)
	})
}
