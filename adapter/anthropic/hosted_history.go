package anthropic

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/packages/param"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/internal/exactjson"
)

const hostedHistoryAnnotation = "anthropic.output_order"

func hostedAnnotation(name string) string { return "anthropic.hosted." + name }

// HostedItem is a provider record, never a local function-handler call.
type HostedItem struct {
	BlockIndex int                     `json:"block_index"`
	Type       string                  `json:"type"`
	ID         string                  `json:"id,omitempty"`
	Name       string                  `json:"name"`
	ToolUseID  string                  `json:"tool_use_id,omitempty"`
	Input      prompty.JSONDocument    `json:"input,omitempty"`
	Content    prompty.JSONDocument    `json:"content,omitempty"`
	SourceJSON prompty.JSONDocument    `json:"source_json"`
	Scope      prompty.ProfileIdentity `json:"scope"`
}

type hostedHistoryEntry struct {
	Source      prompty.JSONDocument `json:"source,omitempty"`
	Part        *int                 `json:"part,omitempty"`
	StateDigest string               `json:"state_digest,omitempty"`
}

func hostedStateDigest(payload []byte) string {
	digest := sha256.Sum256(payload)
	return hex.EncodeToString(digest[:])
}

type hostedHistoryManifest struct {
	Scope      prompty.ProfileIdentity `json:"scope"`
	StopReason string                  `json:"stop_reason"`
	Items      []hostedHistoryEntry    `json:"items"`
}

func hostedType(kind string) bool {
	return kind == "server_tool_use" || kind == "web_search_tool_result" || kind == "web_fetch_tool_result"
}

func hasHostedHistory(messages []prompty.ChatMessage) bool {
	for _, message := range messages {
		if len(message.MessageAnnotations) > 0 {
			return true
		}
	}
	return false
}

func preserveHostedHistory(msg *anthropic.Message, response *prompty.Response, profile prompty.ProfileIdentity) error {
	hasHosted := false
	for _, block := range msg.Content {
		hasHosted = hasHosted || hostedType(block.Type)
	}
	if !hasHosted {
		return nil
	}
	manifest := hostedHistoryManifest{Scope: profile, StopReason: string(msg.StopReason)}
	ids := make(map[string]string)
	completed := make(map[string]bool)
	part := 0
	for index, block := range msg.Content {
		entry := hostedHistoryEntry{}
		if block.Type == "thinking" || block.Type == "redacted_thinking" {
			entry.Part = new(part)
			for _, state := range response.ProviderState {
				if state.PartIndex == part {
					entry.StateDigest = hostedStateDigest(state.Envelope.Payload)
				}
			}
			if entry.StateDigest == "" {
				return ErrHostedTool
			}
			part++
		} else {
			entry.Source = prompty.JSONDocument(block.RawJSON())
			if len(entry.Source) == 0 {
				return ErrHostedTool
			}
			switch block.Type {
			case "text":
				if err := validateHostedText(entry.Source); err != nil {
					return err
				}
				part++
			case "tool_use":
				fields, err := hostedObject(entry.Source, "type", "id", "name", "input", "caller")
				id, validID := sourceString(fields, "id", true)
				_, validName := sourceString(fields, "name", true)
				var input map[string]json.RawMessage
				validInput := uniqueSyntheticJSON(fields["input"]) &&
					json.Unmarshal(fields["input"], &input) == nil && input != nil
				validIdentity := validID && validName && ids[id] == ""
				if err != nil || !sourceEnum(fields, "type", "tool_use") || !sourceCaller(fields) ||
					!validIdentity || !validInput {
					return ErrHostedTool
				}
				ids[id] = "local"
				part++
			default:
				kind, id, name, err := validateHostedSource(entry.Source)
				if err != nil {
					return err
				}
				if kind == "server_tool_use" {
					if ids[id] != "" {
						return ErrHostedTool
					}
					ids[id] = name
				} else {
					if ids[id] != name || completed[id] {
						return ErrHostedTool
					}
					completed[id] = true
				}
				if err := appendHostedItem(response, index, entry.Source, profile); err != nil {
					return err
				}
			}
		}
		manifest.Items = append(manifest.Items, entry)
	}
	for id, name := range ids {
		if name != "local" && !completed[id] && msg.StopReason != "pause_turn" && msg.StopReason != "tool_use" &&
			msg.StopReason != "max_tokens" {
			return ErrHostedTool
		}
	}
	if part != len(response.Content) {
		return ErrHostedTool
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return ErrHostedTool
	}
	response.MessageAnnotations = append(response.MessageAnnotations, prompty.MessageAnnotation{
		Kind: hostedHistoryAnnotation, Value: encoded, Scope: new(profile), Required: true,
	})
	return nil
}

func appendHostedItem(
	response *prompty.Response,
	index int,
	source prompty.JSONDocument,
	scope prompty.ProfileIdentity,
) error {
	kind, id, name, err := validateHostedSource(source)
	if err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(source, &fields) != nil {
		return ErrHostedTool
	}
	item := HostedItem{BlockIndex: index, Type: kind, Name: name, SourceJSON: source, Scope: scope}
	if kind == "server_tool_use" {
		item.ID, item.Input = id, fields["input"]
	} else {
		item.ToolUseID, item.Content = id, fields["content"]
	}
	encoded, err := json.Marshal(item)
	if err != nil {
		return ErrHostedTool
	}
	response.MessageAnnotations = append(response.MessageAnnotations, prompty.MessageAnnotation{
		Kind: hostedAnnotation(name), Value: encoded, Scope: new(scope), Required: true,
	})
	return nil
}

// HostedItems returns owned records, verifying each projection against its source.
func HostedItems(response *prompty.Response) ([]HostedItem, error) {
	if response == nil {
		return nil, ErrHostedTool
	}
	var items []HostedItem
	prior := -1
	for _, annotation := range response.MessageAnnotations {
		if annotation.Kind != hostedAnnotation("web_search") && annotation.Kind != hostedAnnotation("web_fetch") {
			continue
		}
		if !annotation.Required || annotation.Scope == nil {
			return nil, ErrHostedTool
		}
		var item HostedItem
		decoder := json.NewDecoder(bytes.NewReader(annotation.Value))
		decoder.DisallowUnknownFields()
		if !uniqueSyntheticJSON(annotation.Value) || decoder.Decode(&item) != nil || item.BlockIndex <= prior ||
			*annotation.Scope != item.Scope {
			return nil, ErrHostedTool
		}
		check := prompty.NewResponse(nil)
		if appendHostedItem(check, item.BlockIndex, item.SourceJSON, item.Scope) != nil ||
			check.MessageAnnotations[0].Kind != annotation.Kind || !exactjson.Equal(check.MessageAnnotations[0].Value, annotation.Value) {
			return nil, ErrHostedTool
		}
		prior = item.BlockIndex
		items = append(items, item)
	}
	return items, nil
}

func decodeHostedHistory(message prompty.ChatMessage) (hostedHistoryManifest, error) {
	var manifest hostedHistoryManifest
	found := false
	for _, annotation := range message.MessageAnnotations {
		if !annotation.Required || annotation.Scope == nil {
			return manifest, prompty.ErrProviderState
		}
		if annotation.Kind != hostedHistoryAnnotation {
			if annotation.Kind != hostedAnnotation("web_search") && annotation.Kind != hostedAnnotation("web_fetch") {
				return manifest, prompty.ErrProviderState
			}
			continue
		}
		decoder := json.NewDecoder(bytes.NewReader(annotation.Value))
		decoder.DisallowUnknownFields()
		if found || !uniqueSyntheticJSON(annotation.Value) || decoder.Decode(&manifest) != nil ||
			len(manifest.Items) == 0 {
			return manifest, prompty.ErrProviderState
		}
		found = true
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

func (a *Adapter) hostedHistoryMessage(
	message prompty.ChatMessage,
	profile prompty.ProfileIdentity,
) (anthropic.MessageParam, error) {
	manifest, err := decodeHostedHistory(message)
	if err != nil {
		return anthropic.MessageParam{}, err
	}
	if manifest.Scope != profile || message.Role != prompty.RoleAssistant || message.CachePolicy != nil ||
		len(message.Metadata) != 0 {
		return anthropic.MessageParam{}, prompty.ErrProviderState
	}
	states := make(map[int]prompty.PartState)
	for _, state := range message.ProviderState {
		states[state.PartIndex] = state
	}
	var blocks []anthropic.ContentBlockUnion
	var params []anthropic.ContentBlockParamUnion
	part := 0
	for _, entry := range manifest.Items {
		if (entry.Part == nil) == (len(entry.Source) == 0) {
			return anthropic.MessageParam{}, prompty.ErrProviderState
		}
		source := entry.Source
		if entry.Part != nil {
			state, exists := states[*entry.Part]
			if !exists || *entry.Part != part || state.Envelope.Codec != thinkingCodec {
				return anthropic.MessageParam{}, prompty.ErrProviderState
			}
			source = state.Envelope.Payload
			if entry.StateDigest != hostedStateDigest(source) {
				return anthropic.MessageParam{}, prompty.ErrProviderState
			}
			if _, thinkingErr := decodeThinking(source); thinkingErr != nil {
				return anthropic.MessageParam{}, thinkingErr
			}
			delete(states, part)
		}
		if entry.Part == nil && entry.StateDigest != "" {
			return anthropic.MessageParam{}, prompty.ErrProviderState
		}
		var block anthropic.ContentBlockUnion
		if !uniqueSyntheticJSON(source) || json.Unmarshal(source, &block) != nil {
			return anthropic.MessageParam{}, prompty.ErrProviderState
		}
		if entry.Part == nil && (block.Type == "thinking" || block.Type == "redacted_thinking") {
			return anthropic.MessageParam{}, prompty.ErrProviderState
		}
		if !hostedType(block.Type) {
			part++
		}
		blocks = append(blocks, block)
		params = append(params, param.Override[anthropic.ContentBlockParamUnion](source))
	}
	if len(states) != 0 {
		return anthropic.MessageParam{}, prompty.ErrProviderState
	}
	expected, err := a.ParseResponseForProfile(
		&anthropic.Message{Content: blocks, StopReason: anthropic.StopReason(manifest.StopReason)},
		profile,
	)
	if err != nil || !equalHostedProjection(message, expected) {
		return anthropic.MessageParam{}, prompty.ErrProviderState
	}
	return anthropic.NewAssistantMessage(params...), nil
}

func equalHostedProjection(message prompty.ChatMessage, expected *prompty.Response) bool {
	if expected == nil || len(expected.Content) != len(message.Content) {
		return false
	}
	for index, part := range expected.Content {
		if call, ok := part.(prompty.ToolCallPart); ok {
			var actual prompty.ToolCallPart
			switch value := message.Content[index].(type) {
			case prompty.ToolCallPart:
				actual = value
			case *prompty.ToolCallPart:
				if value == nil {
					return false
				}
				actual = *value
			default:
				return false
			}
			if actual.ArgsChunk != "" || !exactjson.Equal([]byte(call.Args), []byte(actual.Args)) {
				return false
			}
			call.Args = actual.Args
			expected.Content[index] = call
		}
	}
	type projection struct {
		Content     []prompty.ContentPart       `json:"content"`
		State       []prompty.PartState         `json:"state"`
		Annotations []prompty.PartAnnotation    `json:"annotations"`
		Message     []prompty.MessageAnnotation `json:"message"`
	}
	want, err := json.Marshal(
		projection{message.Content, message.ProviderState, message.Annotations, message.MessageAnnotations},
	)
	got, gotErr := json.Marshal(
		projection{expected.Content, expected.ProviderState, expected.Annotations, expected.MessageAnnotations},
	)
	return err == nil && gotErr == nil && exactjson.Equal(want, got)
}
