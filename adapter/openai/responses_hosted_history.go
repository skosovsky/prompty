package openai

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/adapter"
)

const hostedHistoryAnnotation = "openai.responses.output_order"

type responseHistoryEntry struct {
	Source prompty.JSONDocument `json:"source,omitempty"`
	Part   *int                 `json:"part,omitempty"`
}

type responseHistoryManifest struct {
	Scope prompty.ProfileIdentity `json:"scope"`
	Items []responseHistoryEntry  `json:"items"`
}

func preserveHostedHistory(items []responses.ResponseOutputItemUnion, response *prompty.Response,
	profile prompty.ProfileIdentity,
) error {
	hasHosted := false
	for _, item := range items {
		hasHosted = hasHosted || item.Type == "web_search_call" || item.Type == "file_search_call"
	}
	if !hasHosted {
		return nil
	}
	manifest := responseHistoryManifest{Scope: profile}
	part := 0
	for _, item := range items {
		entry := responseHistoryEntry{}
		if item.Type == "reasoning" || item.Type == "function_call" {
			entry.Part = new(part)
			part++
		} else {
			entry.Source = []byte(item.RawJSON())
			if item.Type == "message" {
				part += len(item.Content)
			}
		}
		manifest.Items = append(manifest.Items, entry)
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return prompty.ErrRemoteResponse
	}
	response.MessageAnnotations = append(response.MessageAnnotations, prompty.MessageAnnotation{
		Kind: hostedHistoryAnnotation, Value: encoded, Scope: new(profile), Required: true,
	})
	return nil
}

func decodeHostedHistory(message prompty.ChatMessage) (responseHistoryManifest, error) {
	var manifest responseHistoryManifest
	found := false
	for _, annotation := range message.MessageAnnotations {
		if !annotation.Required || annotation.Scope == nil {
			return manifest, prompty.ErrProviderState
		}
		if annotation.Kind != hostedHistoryAnnotation {
			if annotation.Kind != hostedItemAnnotation {
				return manifest, adapter.ErrUnsupportedCapability
			}
			continue
		}
		if found || !uniqueResponseJSON(annotation.Value) {
			return manifest, prompty.ErrProviderState
		}
		found = true
		decoder := json.NewDecoder(bytes.NewReader(annotation.Value))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&manifest) != nil || len(manifest.Items) == 0 {
			return manifest, prompty.ErrProviderState
		}
	}
	if !found {
		return manifest, prompty.ErrProviderState
	}
	for _, annotation := range message.MessageAnnotations {
		if *annotation.Scope != manifest.Scope {
			return manifest, prompty.ErrProviderState
		}
	}
	return manifest, nil
}

func responseHostedHistoryInput(message prompty.ChatMessage, profile prompty.ProfileIdentity,
) ([]responses.ResponseInputItemUnionParam, error) {
	manifest, err := decodeHostedHistory(message)
	if err != nil {
		return nil, err
	}
	if manifest.Scope != profile || message.Role != prompty.RoleAssistant || message.CachePolicy != nil ||
		len(message.Metadata) != 0 {
		return nil, prompty.ErrProviderState
	}
	expected := prompty.NewResponse(nil)
	states := make(map[int]prompty.PartState)
	for _, state := range message.ProviderState {
		states[state.PartIndex] = state
	}
	var out []responses.ResponseInputItemUnionParam
	seen := make(map[string]bool)
	for index, entry := range manifest.Items {
		if (entry.Part == nil) == (len(entry.Source) == 0) {
			return nil, prompty.ErrProviderState
		}
		if entry.Part != nil {
			item, itemErr := responseHistoryPrivateItem(message, states, *entry.Part, expected, profile)
			if itemErr != nil {
				return nil, itemErr
			}
			out = append(out, item)
			continue
		}
		item, itemErr := responseHistoryPublicItem(entry.Source, index, expected, profile)
		if itemErr != nil {
			return nil, itemErr
		}
		id := responseHistoryItemID(entry.Source)
		if id == "" || seen[id] {
			return nil, prompty.ErrProviderState
		}
		seen[id] = true
		out = append(out, item)
	}
	if len(states) != 0 || !equalHostedHistoryProjection(message, expected) {
		return nil, prompty.ErrProviderState
	}
	return out, nil
}

func responseHistoryPrivateItem(message prompty.ChatMessage, states map[int]prompty.PartState,
	index int, expected *prompty.Response, profile prompty.ProfileIdentity,
) (responses.ResponseInputItemUnionParam, error) {
	var zero responses.ResponseInputItemUnionParam
	state, present := states[index]
	if !present || index != len(expected.Content) || index >= len(message.Content) {
		return zero, prompty.ErrProviderState
	}
	state.PartIndex = 0
	items, err := responseMessageInput(prompty.ChatMessage{
		Role: prompty.RoleAssistant, Content: []prompty.ContentPart{message.Content[index]},
		ProviderState: []prompty.PartState{state},
	}, profile)
	if err != nil || len(items) != 1 {
		return zero, prompty.ErrProviderState
	}
	delete(states, index)
	expected.Content = append(expected.Content, message.Content[index])
	return items[0], nil
}

func responseHistoryPublicItem(source prompty.JSONDocument, index int, expected *prompty.Response,
	profile prompty.ProfileIdentity,
) (responses.ResponseInputItemUnionParam, error) {
	var zero responses.ResponseInputItemUnionParam
	var item responses.ResponseOutputItemUnion
	if !uniqueResponseJSON(source) || json.Unmarshal(source, &item) != nil {
		return zero, prompty.ErrProviderState
	}
	switch item.Type {
	case "message":
		if item.Status != "completed" || parseRemoteItems([]responses.ResponseOutputItemUnion{item}, expected,
			nil, profile, responseToolContract{}) != nil {
			return zero, prompty.ErrProviderState
		}
	case "web_search_call", "file_search_call":
		if item.Status != "completed" && item.Status != "failed" {
			return zero, prompty.ErrProviderState
		}
		if preserveHostedItem(
			index,
			string(source),
			[]string{HostedWebSearch, HostedFileSearch},
			expected,
			profile,
		) != nil {
			return zero, prompty.ErrProviderState
		}
	default:
		return zero, prompty.ErrProviderState
	}
	var native responses.ResponseInputItemUnion
	if json.Unmarshal(source, &native) != nil {
		return zero, prompty.ErrProviderState
	}
	return native.ToParam(), nil
}

func responseHistoryItemID(source prompty.JSONDocument) string {
	var fields struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(source, &fields) != nil {
		return ""
	}
	return strings.TrimSpace(fields.ID)
}

func equalHostedHistoryProjection(message prompty.ChatMessage, expected *prompty.Response) bool {
	annotations := make([]prompty.MessageAnnotation, 0, len(message.MessageAnnotations))
	for _, annotation := range message.MessageAnnotations {
		if annotation.Kind != hostedHistoryAnnotation {
			annotations = append(annotations, annotation)
		}
	}
	type projection struct {
		Content     []prompty.ContentPart       `json:"content"`
		Annotations []prompty.PartAnnotation    `json:"annotations"`
		Message     []prompty.MessageAnnotation `json:"message"`
	}
	want, err := json.Marshal(projection{message.Content, message.Annotations, annotations})
	got, gotErr := json.Marshal(projection{expected.Content, expected.Annotations, expected.MessageAnnotations})
	return err == nil && gotErr == nil && bytes.Equal(want, got)
}
