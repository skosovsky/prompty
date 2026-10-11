package openai

import (
	"strings"

	sdk "github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
)

// A checked advisory omission is not an unsupported provider block marker.
// Validate the portable sentinel so malformed controls cannot bypass preflight.
func responseCacheOmitted(policy *prompty.CachePolicy) bool {
	return policy == nil || policy.Mechanism == prompty.CacheOmit && prompty.ValidateCachePolicy(policy) == nil
}

// ValidateRequestCacheControl checks actual SDK knobs, not model/account knowledge.
func (*Adapter) ValidateRequestCacheControl(policy *prompty.CachePolicy) error {
	_, err := openAIRequestCache(policy, wireOpenai)
	return err
}

// ValidateRequestCacheControl keeps Responses profile scope distinct from Chat.
func (*Responses) ValidateRequestCacheControl(policy *prompty.CachePolicy) error {
	_, err := openAIRequestCache(policy, "openai.responses")
	return err
}

func openAIRequestCache(policy *prompty.CachePolicy, provider string) (map[string]string, error) {
	if err := prompty.ValidateRequestCachePolicy(policy); err != nil {
		return nil, err
	}
	if policy == nil {
		return nil, nil //nolint:nilnil // No mapped value or emitted frame is a successful absence, not an operation failure.
	}
	if policy.Provider != "" && policy.Provider != provider ||
		(len(policy.Controls) != 0 && !uniqueResponseJSON(policy.Controls)) {
		return nil, prompty.ErrCacheRequirement
	}
	controls, err := prompty.JSONDocumentAsMap(policy.Controls)
	if err != nil {
		return nil, prompty.ErrCachePolicy
	}
	values := make(map[string]string)
	for key, raw := range controls {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			return nil, prompty.ErrCacheRequirement
		}
		switch key {
		case "prompt_cache_key":
			values[key] = value
		case "prompt_cache_retention":
			if value != "in-memory" && value != "24h" {
				return nil, prompty.ErrCacheRequirement
			}
			values[key] = value
		default:
			return nil, prompty.ErrCacheRequirement
		}
	}
	return values, nil
}

func applyChatRequestCache(params *sdk.ChatCompletionNewParams, policy *prompty.CachePolicy) error {
	values, err := openAIRequestCache(policy, wireOpenai)
	if err != nil {
		return err
	}
	if value, ok := values["prompt_cache_key"]; ok {
		params.PromptCacheKey = sdk.String(value)
	}
	params.PromptCacheRetention = sdk.ChatCompletionNewParamsPromptCacheRetention(values["prompt_cache_retention"])
	return nil
}

func applyResponsesRequestCache(params *responses.ResponseNewParams, policy *prompty.CachePolicy) error {
	values, err := openAIRequestCache(policy, "openai.responses")
	if err != nil {
		return err
	}
	if value, ok := values["prompt_cache_key"]; ok {
		params.PromptCacheKey = sdk.String(value)
	}
	params.PromptCacheRetention = responses.ResponseNewParamsPromptCacheRetention(values["prompt_cache_retention"])
	return nil
}
