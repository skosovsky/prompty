package prompty

import (
	"errors"
	"strings"
)

// HostedToolRequest is explicit provider execution intent, never a local
// handler binding. Kind/settings semantics belong to the selected adapter.
type HostedToolRequest struct {
	Kind     string       `json:"kind"`
	Settings JSONDocument `json:"settings,omitempty"`
}

var ErrHostedToolRequest = errors.New("prompty: invalid hosted tool declaration")

func CloneHostedToolRequests(tools []HostedToolRequest) []HostedToolRequest {
	if tools == nil {
		return nil
	}
	out := make([]HostedToolRequest, len(tools))
	for index, tool := range tools {
		out[index] = tool
		out[index].Settings = CloneJSONDocument(tool.Settings)
	}
	return out
}

// ValidateHostedToolRequests checks only portable shape. It grants no provider
// support and interprets no vendor options; adapters validate their own whitelist.
func ValidateHostedToolRequests(tools []HostedToolRequest) error {
	seen := make(map[string]bool)
	for _, tool := range tools {
		if strings.TrimSpace(tool.Kind) == "" || seen[tool.Kind] {
			return ErrHostedToolRequest
		}
		seen[tool.Kind] = true
		if _, err := JSONDocumentAsMap(tool.Settings); err != nil {
			return errors.Join(ErrHostedToolRequest, err)
		}
	}
	return nil
}
