package anthropic

import (
	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty"
)

const maxCacheMarkers = 4

// ValidateCacheControl checks this module's block marker/TTL request mapping.
// Model and endpoint support are separately checked by high-level preflight.
func (a *Adapter) ValidateCacheControl(policy *prompty.CachePolicy) error {
	if err := prompty.ValidateCachePolicy(policy); err != nil {
		return err
	}
	if policy == nil || policy.Mechanism != prompty.CacheBlockMarker ||
		policy.Provider != "" && policy.Provider != "anthropic" {
		return prompty.ErrCacheRequirement
	}
	_, err := anthropicCacheTTL(policy)
	return err
}

func anthropicCacheTTL(policy *prompty.CachePolicy) (anthropic.CacheControlEphemeralTTL, error) {
	controls, err := prompty.JSONDocumentAsMap(policy.Controls)
	if err != nil {
		return "", prompty.ErrCachePolicy
	}
	for key := range controls {
		if key != "ttl" {
			return "", prompty.ErrCacheRequirement
		}
	}
	value, exists := controls["ttl"]
	if !exists {
		return "", nil // Provider default; no TTL guarantee was requested.
	}
	switch value {
	case "5m":
		return anthropic.CacheControlEphemeralTTLTTL5m, nil
	case "1h":
		return anthropic.CacheControlEphemeralTTLTTL1h, nil
	default:
		return "", prompty.ErrCacheRequirement
	}
}

// Validate the actual SDK block order (system before messages, nested tool
// result content before its enclosing result). No control is silently dropped
// to meet the provider limit, even for advisory intent.
func validateCacheMarkers(request *anthropic.MessageNewParams) error {
	var controls []*anthropic.CacheControlEphemeralParam
	for i := range request.System {
		controls = append(controls, &request.System[i].CacheControl)
	}
	for _, message := range request.Messages {
		for _, block := range message.Content {
			if block.OfToolResult != nil {
				for _, content := range block.OfToolResult.Content {
					controls = append(controls, content.GetCacheControl())
				}
			}
			controls = append(controls, block.GetCacheControl())
		}
	}
	count := 0
	shortSeen := false
	for _, control := range controls {
		if control == nil || control.Type == "" {
			continue
		}
		count++
		if count > maxCacheMarkers || shortSeen && control.TTL == anthropic.CacheControlEphemeralTTLTTL1h {
			return prompty.ErrCacheRequirement
		}
		if control.TTL != anthropic.CacheControlEphemeralTTLTTL1h {
			shortSeen = true // Empty TTL selects the provider's 5m default.
		}
	}
	return nil
}
