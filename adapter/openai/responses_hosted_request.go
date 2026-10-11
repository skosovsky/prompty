package openai

import (
	"bytes"
	"encoding/json"
	"math/big"
	"regexp"
	"slices"
	"strings"

	"github.com/openai/openai-go/v3/packages/param"
	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const (
	HostedWebSearch      = "web_search"
	HostedFileSearch     = "file_search"
	hostedFilterMaxDepth = 32
	hostedNumberMaxBytes = 1024
	hostedDecimalBase    = 10
	hostedBitsPerDigit   = 4
	hostedPrecisionGuard = 64
)

func cloneHostedRequests(input []prompty.HostedToolRequest) ([]prompty.HostedToolRequest, []string, error) {
	cloned := make([]prompty.HostedToolRequest, 0, len(input))
	caps := make([]string, 0, len(input))
	seen := make(map[string]bool)
	for _, tool := range input {
		if seen[tool.Kind] {
			return nil, nil, prompty.ErrRemoteResponse
		}
		capability, err := hostedCapability(tool.Kind)
		if err != nil {
			return nil, nil, err
		}
		seen[tool.Kind] = true
		tool.Settings = prompty.CloneJSONDocument(tool.Settings)
		cloned = append(cloned, tool)
		caps = append(caps, capability)
	}
	return cloned, caps, nil
}

func hostedCapability(kind string) (string, error) {
	switch kind {
	case HostedWebSearch:
		return prompty.CapabilityHostedWebSearch, nil
	case HostedFileSearch:
		return prompty.CapabilityHostedFileSearch, nil
	default:
		return "", adapter.ErrUnsupportedCapability
	}
}

func hostedIncludes(kinds []string) ([]responses.ResponseIncludable, []string, error) {
	var include []responses.ResponseIncludable
	var caps []string
	for _, kind := range kinds {
		capability, err := hostedCapability(kind)
		if err != nil {
			return nil, nil, err
		}
		caps = append(caps, capability)
		if kind == HostedWebSearch {
			include = append(include, responses.ResponseIncludableWebSearchCallActionSources)
		} else {
			include = append(include, responses.ResponseIncludableFileSearchCallResults)
		}
	}
	return include, caps, nil
}

func prepareHostedTools(tools []prompty.HostedToolRequest, params *responses.ResponseNewParams) error {
	var kinds []string
	for _, tool := range tools {
		var fields map[string]json.RawMessage
		var err error
		if tool.Kind == HostedWebSearch {
			fields, err = validateHostedWebSettings(tool.Settings)
		} else {
			fields, err = validateHostedFileSettings(tool.Settings)
		}
		if err != nil {
			return err
		}
		encodedKind, _ := json.Marshal(tool.Kind)
		fields[wireType] = encodedKind
		encoded, err := json.Marshal(fields)
		if err != nil {
			return prompty.ErrRemoteResponse
		}
		params.Tools = append(params.Tools, param.Override[responses.ToolUnionParam](json.RawMessage(encoded)))
		kinds = append(kinds, tool.Kind)
	}
	include, _, err := hostedIncludes(kinds)
	params.Include = include
	if len(tools) == 0 {
		params.SetExtraFields(map[string]any{"tools": []responses.ToolUnionParam{}})
	}
	return err
}

func hostedObject(document json.RawMessage, allowed ...string) (map[string]json.RawMessage, error) {
	if len(document) == 0 {
		document = json.RawMessage(`{}`)
	}
	if !uniqueResponseJSON(document) || bytes.Equal(bytes.TrimSpace(document), []byte("null")) {
		return nil, prompty.ErrRemoteResponse
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(document, &fields) != nil || fields == nil {
		return nil, prompty.ErrRemoteResponse
	}
	for key := range fields {
		valid := false
		for _, name := range allowed {
			valid = valid || key == name
		}
		if !valid {
			return nil, prompty.ErrRemoteResponse
		}
	}
	return fields, nil
}

func validateHostedWebSettings(document json.RawMessage) (map[string]json.RawMessage, error) {
	fields, err := hostedObject(document, "search_context_size", "filters", "user_location", "external_web_access")
	if err != nil {
		return nil, err
	}
	if value, present := fields["search_context_size"]; present && !hostedEnum(value, "low", "medium", "high") {
		return nil, prompty.ErrRemoteResponse
	}
	if value, present := fields["external_web_access"]; present && string(value) != "true" && string(value) != "false" {
		return nil, prompty.ErrRemoteResponse
	}
	if value, present := fields["filters"]; present {
		filter, filterErr := hostedObject(value, "allowed_domains")
		if filterErr != nil || !hostedStrings(filter["allowed_domains"], false) {
			return nil, prompty.ErrRemoteResponse
		}
	}
	if value, present := fields["user_location"]; present {
		location, locationErr := hostedObject(value, wireType, "city", "country", "region", "timezone")
		if locationErr != nil || !hostedEnum(location[wireType], "approximate") {
			return nil, prompty.ErrRemoteResponse
		}
		for key, field := range location {
			if key != wireType && !hostedString(field) {
				return nil, prompty.ErrRemoteResponse
			}
		}
	}
	return fields, nil
}

var vectorStoreID = regexp.MustCompile(`^vs_[A-Za-z0-9_-]+$`)

func validateHostedFileSettings(document json.RawMessage) (map[string]json.RawMessage, error) {
	fields, err := hostedObject(document, "vector_store_ids", "max_num_results", "filters", "ranking_options")
	if err != nil || !hostedStrings(fields["vector_store_ids"], true) {
		return nil, prompty.ErrRemoteResponse
	}
	var ids []string
	_ = json.Unmarshal(fields["vector_store_ids"], &ids)
	for _, id := range ids {
		if !vectorStoreID.MatchString(id) {
			return nil, prompty.ErrRemoteResponse
		}
	}
	if value, present := fields["max_num_results"]; present {
		var maximum int
		if json.Unmarshal(value, &maximum) != nil || maximum < 1 || maximum > 50 {
			return nil, prompty.ErrRemoteResponse
		}
	}
	if value, present := fields["filters"]; present && !validHostedFilter(value, 0) {
		return nil, prompty.ErrRemoteResponse
	}
	if value, present := fields["ranking_options"]; present && !validHostedRanking(value) {
		return nil, prompty.ErrRemoteResponse
	}
	return fields, nil
}

func validHostedRanking(document json.RawMessage) bool {
	fields, err := hostedObject(document, "ranker", "score_threshold", "hybrid_search")
	if err != nil {
		return false
	}
	if value, present := fields["ranker"]; present && !hostedEnum(value, "auto", "default-2024-11-15") {
		return false
	}
	if value, present := fields["score_threshold"]; present && !hostedNumberRange(value, true) {
		return false
	}
	if value, present := fields["hybrid_search"]; present {
		hybrid, hybridErr := hostedObject(value, "embedding_weight", "text_weight")
		if hybridErr != nil || !hostedNumberRange(hybrid["embedding_weight"], false) ||
			!hostedNumberRange(hybrid["text_weight"], false) {
			return false
		}
	}
	return true
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func validHostedFilter(document json.RawMessage, depth int) bool {
	if depth > hostedFilterMaxDepth {
		return false
	}
	fields, err := hostedObject(document, wireType, "key", "value", "filters")
	if err != nil {
		return false
	}
	if hostedEnum(fields[wireType], "and", "or") {
		if len(fields) != 2 {
			return false
		}
		var children []json.RawMessage
		if json.Unmarshal(fields["filters"], &children) != nil || len(children) == 0 {
			return false
		}
		for _, child := range children {
			if !validHostedFilter(child, depth+1) {
				return false
			}
		}
		return true
	}
	if len(fields) != 3 || !hostedString(fields["key"]) {
		return false
	}
	if hostedEnum(fields[wireType], "in", "nin") {
		var values []json.RawMessage
		if json.Unmarshal(fields["value"], &values) != nil || len(values) == 0 {
			return false
		}
		for _, value := range values {
			if !hostedScalar(value) {
				return false
			}
		}
		return true
	}
	return hostedEnum(fields[wireType], "eq", "ne", "gt", "gte", "lt", "lte") && hostedScalar(fields["value"])
}

func hostedNumberRange(document json.RawMessage, upperBound bool) bool {
	if len(document) == 0 || len(document) > hostedNumberMaxBytes || !json.Valid(document) {
		return false
	}
	precision := uint(len(document)*hostedBitsPerDigit + hostedPrecisionGuard)
	value, _, err := big.ParseFloat(string(document), hostedDecimalBase, precision, big.ToNearestEven)
	return err == nil && value.Sign() >= 0 && (!upperBound || value.Cmp(big.NewFloat(1)) <= 0)
}

func hostedScalar(document json.RawMessage) bool {
	decoder := json.NewDecoder(bytes.NewReader(document))
	decoder.UseNumber()
	var value any
	if decoder.Decode(&value) != nil {
		return false
	}
	switch value.(type) {
	case string, bool, json.Number:
		return true
	default:
		return false
	}
}

func hostedEnum(document json.RawMessage, allowed ...string) bool {
	var value string
	if json.Unmarshal(document, &value) != nil {
		return false
	}
	return slices.Contains(allowed, value)
}

func hostedString(document json.RawMessage) bool {
	var value string
	return json.Unmarshal(document, &value) == nil && strings.TrimSpace(value) != ""
}

func hostedStrings(document json.RawMessage, required bool) bool {
	var values []string
	if json.Unmarshal(document, &values) != nil || values == nil || (required && len(values) == 0) {
		return false
	}
	seen := make(map[string]bool)
	for _, value := range values {
		if strings.TrimSpace(value) == "" || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}
