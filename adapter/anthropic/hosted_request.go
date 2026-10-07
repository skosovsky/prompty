package anthropic

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/anthropics/anthropic-sdk-go"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

// Versioned declarations deliberately exclude dynamic filtering/code execution.
const (
	HostedWebSearch = "anthropic.web_search_20250305"
	HostedWebFetch  = "anthropic.web_fetch_20250910"
)

var ErrHostedTool = errors.New("anthropic: invalid hosted tool contract")

func prepareHostedTools(exec *prompty.PromptExecution) ([]anthropic.ToolUnionParam, error) {
	var tools []anthropic.ToolUnionParam
	for _, declaration := range exec.HostedTools {
		var tool anthropic.ToolUnionParam
		switch declaration.Kind {
		case HostedWebSearch:
			search, err := hostedSearchParams(declaration.Settings)
			if err != nil {
				return nil, err
			}
			tool.OfWebSearchTool20250305 = search
		case HostedWebFetch:
			fetch, err := hostedFetchParams(declaration.Settings)
			if err != nil {
				return nil, err
			}
			tool.OfWebFetchTool20250910 = fetch
		default:
			return nil, adapter.ErrUnsupportedCapability
		}
		name := "web_search"
		if declaration.Kind == HostedWebFetch {
			name = "web_fetch"
		}
		for _, local := range exec.Tools {
			if local.Name == name {
				return nil, ErrHostedTool
			}
		}
		tools = append(tools, tool)
	}
	return tools, nil
}

func hostedObject(document json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
	if len(document) == 0 {
		document = json.RawMessage(`{}`)
	}
	if !uniqueSyntheticJSON(document) {
		return nil, ErrHostedTool
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(document, &fields) != nil || fields == nil {
		return nil, ErrHostedTool
	}
	for key := range fields {
		if !slices.Contains(allowed, key) {
			return nil, ErrHostedTool
		}
	}
	return fields, nil
}

type hostedCommon struct {
	maxUses          *int64
	allowed, blocked []string
}

func hostedCommonSettings(fields map[string]json.RawMessage) (hostedCommon, error) {
	var out hostedCommon
	var err error
	if out.maxUses, err = hostedPositiveInteger(fields, "max_uses"); err != nil {
		return out, err
	}
	_, allowed := fields["allowed_domains"]
	_, blocked := fields["blocked_domains"]
	if allowed && blocked {
		return out, ErrHostedTool
	}
	if allowed {
		out.allowed, err = hostedDomains(fields["allowed_domains"])
	} else if blocked {
		out.blocked, err = hostedDomains(fields["blocked_domains"])
	}
	return out, err
}

func hostedPositiveInteger(fields map[string]json.RawMessage, key string) (*int64, error) {
	document, present := fields[key]
	if !present {
		return nil, nil
	}
	var value int64
	if json.Unmarshal(document, &value) != nil || value <= 0 {
		return nil, ErrHostedTool
	}
	return &value, nil
}

func hostedDomains(document json.RawMessage) ([]string, error) {
	var domains []string
	if json.Unmarshal(document, &domains) != nil || len(domains) == 0 {
		return nil, ErrHostedTool
	}
	for _, domain := range domains {
		if strings.TrimSpace(domain) != domain || domain == "" || strings.HasPrefix(domain, "/") ||
			strings.ContainsAny(domain, ":@?# \t\r\n") {
			return nil, ErrHostedTool
		}
	}
	return domains, nil
}

func hostedSearchParams(document json.RawMessage) (*anthropic.WebSearchTool20250305Param, error) {
	fields, err := hostedObject(document, "max_uses", "allowed_domains", "blocked_domains", "user_location")
	if err != nil {
		return nil, err
	}
	common, err := hostedCommonSettings(fields)
	if err != nil {
		return nil, err
	}
	out := &anthropic.WebSearchTool20250305Param{AllowedDomains: common.allowed, BlockedDomains: common.blocked}
	if common.maxUses != nil {
		out.MaxUses = anthropic.Int(*common.maxUses)
	}
	if location, present := fields["user_location"]; present {
		values, locationErr := hostedObject(location, "type", "city", "country", "region", "timezone")
		if locationErr != nil || len(values) < 2 {
			return nil, ErrHostedTool
		}
		var kind string
		if json.Unmarshal(values["type"], &kind) != nil || kind != "approximate" {
			return nil, ErrHostedTool
		}
		for key, value := range values {
			if key == "type" {
				continue
			}
			var text string
			if json.Unmarshal(value, &text) != nil || strings.TrimSpace(text) == "" {
				return nil, ErrHostedTool
			}
			switch key {
			case "city":
				out.UserLocation.City = anthropic.String(text)
			case "country":
				out.UserLocation.Country = anthropic.String(text)
			case "region":
				out.UserLocation.Region = anthropic.String(text)
			case "timezone":
				out.UserLocation.Timezone = anthropic.String(text)
			}
		}
	}
	return out, nil
}

func hostedFetchParams(document json.RawMessage) (*anthropic.WebFetchTool20250910Param, error) {
	fields, err := hostedObject(
		document,
		"max_uses",
		"allowed_domains",
		"blocked_domains",
		"max_content_tokens",
		"citations",
	)
	if err != nil {
		return nil, err
	}
	common, err := hostedCommonSettings(fields)
	if err != nil {
		return nil, err
	}
	maxContent, err := hostedPositiveInteger(fields, "max_content_tokens")
	if err != nil {
		return nil, err
	}
	out := &anthropic.WebFetchTool20250910Param{AllowedDomains: common.allowed, BlockedDomains: common.blocked}
	if common.maxUses != nil {
		out.MaxUses = anthropic.Int(*common.maxUses)
	}
	if maxContent != nil {
		out.MaxContentTokens = anthropic.Int(*maxContent)
	}
	if citation, present := fields["citations"]; present {
		values, citationErr := hostedObject(citation, "enabled")
		if citationErr != nil || len(values) != 1 {
			return nil, ErrHostedTool
		}
		var enabled *bool
		if json.Unmarshal(values["enabled"], &enabled) != nil || enabled == nil {
			return nil, ErrHostedTool
		}
		out.Citations.Enabled = anthropic.Bool(*enabled)
	}
	return out, nil
}
