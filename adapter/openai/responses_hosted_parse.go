package openai

import (
	"bytes"
	"encoding/json"
	"slices"
	"strings"

	"github.com/skosovsky/prompty"
)

const hostedItemAnnotation = "openai.responses.hosted_item"

// HostedSearchCoverage describes visibility, not exhaustive retrieval coverage.
type HostedSearchCoverage string

const (
	HostedCoverageUnknown     HostedSearchCoverage = "unknown"
	HostedCoverageUnavailable HostedSearchCoverage = "unavailable"
	HostedCoveragePartial     HostedSearchCoverage = "partial"
	HostedCoverageAvailable   HostedSearchCoverage = "available"
)

type HostedSearchSource struct {
	Type       string               `json:"type"`
	URL        string               `json:"url,omitempty"`
	SourceJSON prompty.JSONDocument `json:"-"`
}

type HostedFileSearchResult struct {
	FileID     string               `json:"file_id"`
	Filename   string               `json:"filename"`
	Text       string               `json:"text"`
	Score      *json.Number         `json:"score,omitempty"`
	Attributes prompty.JSONDocument `json:"attributes,omitempty"`
	SourceJSON prompty.JSONDocument `json:"-"`
}

// HostedSearchItem retains the original provider output index and JSON. Hosted
// calls are provider operations, never executable local ToolCallParts.
type HostedSearchItem struct {
	OutputIndex int
	ID          string
	Kind        string
	Status      string
	Coverage    HostedSearchCoverage
	Action      prompty.JSONDocument
	Queries     []string
	Sources     []HostedSearchSource
	Results     []HostedFileSearchResult
	SourceJSON  prompty.JSONDocument
}

type hostedItemProjection struct {
	OutputIndex int                  `json:"output_index"`
	Item        prompty.JSONDocument `json:"item"`
}

func preserveHostedItem(
	index int,
	raw string,
	allowed []string,
	response *prompty.Response,
	scope prompty.ProfileIdentity,
) error {
	projection := hostedItemProjection{OutputIndex: index, Item: []byte(raw)}
	item, err := parseHostedItem(projection)
	if err != nil {
		return err
	}
	if !slices.Contains(allowed, item.Kind) {
		return prompty.ErrRemoteResponse
	}
	encoded, err := json.Marshal(projection)
	if err != nil {
		return prompty.ErrRemoteResponse
	}
	response.MessageAnnotations = append(response.MessageAnnotations, prompty.MessageAnnotation{
		Kind: hostedItemAnnotation, Value: encoded, Scope: new(scope), Required: true,
	})
	return nil
}

// HostedSearchItems decodes isolated projections from response, wire-restored
// response or history metadata. Missing/null results stay distinct from empty.
func HostedSearchItems(response *prompty.Response) ([]HostedSearchItem, error) {
	if response == nil {
		return nil, prompty.ErrRemoteResponse
	}
	var items []HostedSearchItem
	seen := make(map[string]bool)
	last := -1
	for _, annotation := range response.MessageAnnotations {
		if annotation.Kind != hostedItemAnnotation {
			continue
		}
		if !annotation.Required || annotation.Scope == nil {
			return nil, prompty.ErrRemoteResponse
		}
		fields, err := hostedObject(annotation.Value, "output_index", "item")
		if err != nil || len(fields) != 2 || bytes.Equal(fields["output_index"], []byte("null")) {
			return nil, prompty.ErrRemoteResponse
		}
		var projection hostedItemProjection
		if json.Unmarshal(annotation.Value, &projection) != nil {
			return nil, prompty.ErrRemoteResponse
		}
		item, err := parseHostedItem(projection)
		if err != nil || seen[item.ID] || item.OutputIndex <= last {
			return nil, prompty.ErrRemoteResponse
		}
		seen[item.ID], last = true, item.OutputIndex
		items = append(items, item)
	}
	return items, nil
}

func parseHostedItem(projection hostedItemProjection) (HostedSearchItem, error) {
	var item HostedSearchItem
	if projection.OutputIndex < 0 || !uniqueResponseJSON(projection.Item) {
		return item, prompty.ErrRemoteResponse
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(projection.Item, &fields) != nil || fields == nil {
		return item, prompty.ErrRemoteResponse
	}
	if !hostedString(fields["id"]) || !hostedString(fields["status"]) {
		return item, prompty.ErrRemoteResponse
	}
	_ = json.Unmarshal(fields["id"], &item.ID)
	_ = json.Unmarshal(fields["status"], &item.Status)
	item.OutputIndex, item.SourceJSON = projection.OutputIndex, prompty.CloneJSONDocument(projection.Item)
	switch {
	case hostedEnum(fields["type"], "web_search_call"):
		item.Kind = HostedWebSearch
		if !hostedEnum(fields["status"], "in_progress", "searching", "completed", "failed") {
			return item, prompty.ErrRemoteResponse
		}
		if len(fields["action"]) == 0 && item.Status != "completed" {
			item.Coverage = HostedCoverageUnknown
			return item, nil
		}
		var action map[string]json.RawMessage
		if json.Unmarshal(fields["action"], &action) != nil || action == nil ||
			!hostedEnum(action["type"], "search", "open_page", "find") {
			return item, prompty.ErrRemoteResponse
		}
		item.Action = prompty.CloneJSONDocument(fields["action"])
		collection, coverage, err := hostedCollection(action["sources"], item.Status)
		if err != nil {
			return item, err
		}
		item.Coverage = coverage
		for _, raw := range collection {
			source, err := parseHostedSource(raw)
			if err != nil {
				return item, err
			}
			item.Sources = append(item.Sources, source)
		}
	case hostedEnum(fields["type"], "file_search_call"):
		item.Kind = HostedFileSearch
		if !hostedEnum(fields["status"], "in_progress", "searching", "completed", "incomplete", "failed") {
			return item, prompty.ErrRemoteResponse
		}
		if len(fields["queries"]) != 0 &&
			(json.Unmarshal(fields["queries"], &item.Queries) != nil || item.Queries == nil) {
			return item, prompty.ErrRemoteResponse
		}
		for _, query := range item.Queries {
			if strings.TrimSpace(query) == "" {
				return item, prompty.ErrRemoteResponse
			}
		}
		collection, coverage, err := hostedCollection(fields["results"], item.Status)
		if err != nil {
			return item, err
		}
		item.Coverage = coverage
		for _, raw := range collection {
			result, err := parseHostedFileResult(raw)
			if err != nil {
				return item, err
			}
			item.Results = append(item.Results, result)
		}
	default:
		return item, prompty.ErrRemoteResponse
	}
	return item, nil
}

func parseHostedSource(raw json.RawMessage) (HostedSearchSource, error) {
	var source HostedSearchSource
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || !hostedString(fields["type"]) {
		return source, prompty.ErrRemoteResponse
	}
	_ = json.Unmarshal(fields["type"], &source.Type)
	if url, present := fields["url"]; present {
		if json.Unmarshal(url, &source.URL) != nil {
			return source, prompty.ErrRemoteResponse
		}
	}
	if source.Type == "url" && strings.TrimSpace(source.URL) == "" {
		return source, prompty.ErrRemoteResponse
	}
	source.SourceJSON = prompty.CloneJSONDocument(raw)
	return source, nil
}

func parseHostedFileResult(raw json.RawMessage) (HostedFileSearchResult, error) {
	var result HostedFileSearchResult
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil || !hostedString(fields["file_id"]) {
		return result, prompty.ErrRemoteResponse
	}
	_ = json.Unmarshal(fields["file_id"], &result.FileID)
	for key, destination := range map[string]*string{"filename": &result.Filename, "text": &result.Text} {
		if value, present := fields[key]; present && json.Unmarshal(value, destination) != nil {
			return result, prompty.ErrRemoteResponse
		}
	}
	if score, present := fields["score"]; present && !bytes.Equal(bytes.TrimSpace(score), []byte("null")) {
		if !hostedNumberRange(score, true) {
			return result, prompty.ErrRemoteResponse
		}
		value := json.Number(string(score))
		result.Score = &value
	}
	if attributes, present := fields["attributes"]; present {
		var object map[string]json.RawMessage
		if json.Unmarshal(attributes, &object) != nil {
			return result, prompty.ErrRemoteResponse
		}
		result.Attributes = prompty.CloneJSONDocument(attributes)
	}
	result.SourceJSON = prompty.CloneJSONDocument(raw)
	return result, nil
}

func hostedCollection(raw json.RawMessage, status string) ([]json.RawMessage, HostedSearchCoverage, error) {
	if len(raw) == 0 {
		return nil, HostedCoverageUnknown, nil
	}
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil, HostedCoverageUnavailable, nil
	}
	var values []json.RawMessage
	if json.Unmarshal(raw, &values) != nil || values == nil {
		return nil, HostedCoverageUnknown, prompty.ErrRemoteResponse
	}
	if status == "completed" {
		return values, HostedCoverageAvailable, nil
	}
	return values, HostedCoveragePartial, nil
}
