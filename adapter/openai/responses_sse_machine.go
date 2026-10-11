package openai

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
)

// responseSSEMachine owns one invocation's provider correlations. It emits
// logical deltas, never a second copy of completed output. Common Stream owns
// exact finalization checks, assembly, cancellation and terminal delivery.
type responseSSEMachine struct {
	creation *preparedResponseCreate
	handle   *responseStreamHandle
	sequence int64
	started  bool
	finished bool
	items    []*responseSSEItem
	ids      map[string]bool
}

type responseSSEItem struct {
	initial         responses.ResponseOutputItemUnion
	done            bool
	parts           []*responseSSEPart
	summaryIndex    int64
	summaries       []string
	summaryTextDone []bool
	summaryPartDone []bool
	finalized       bool
	doneSource      string
}

type responseSSEPart struct {
	id          string
	kind        string
	text        string
	annotations []prompty.JSONDocument
	textDone    bool
	contentDone bool
}

func newResponseSSEMachine(creation *preparedResponseCreate, handle *responseStreamHandle) *responseSSEMachine {
	return &responseSSEMachine{
		started:  false,
		finished: false,
		items:    nil,
		creation: creation,
		handle:   handle,
		sequence: -1,
		ids:      make(map[string]bool),
	}
}

func (s *responseSSEMachine) accept(event responses.ResponseStreamEventUnion) ([]*prompty.ResponseChunk, error) {
	if !uniqueResponseJSON([]byte(event.RawJSON())) || !validResponseSSESource([]byte(event.RawJSON())) ||
		!event.JSON.Type.Valid() ||
		!event.JSON.SequenceNumber.Valid() || event.SequenceNumber <= s.sequence || s.finished {
		return nil, prompty.ErrStreamProtocol
	}
	s.sequence = event.SequenceNumber
	if event.Type == "response.created" {
		if s.started || !event.JSON.Response.Valid() {
			return nil, prompty.ErrStreamProtocol
		}
		if err := s.handle.observeResponse(&event.Response); err != nil {
			return nil, err
		}
		if event.Response.Status != responses.ResponseStatusInProgress {
			return nil, prompty.ErrStreamProtocol
		}
		s.started = true
		frames := []*prompty.ResponseChunk{
			{
				Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
				Content:                 nil,
				ProviderState:           nil,
				StateUpdates:            nil,
				Finalizations:           nil,
				Annotations:             nil,
				MessageAnnotations:      nil,
				ContinuationUnavailable: false,
				Usage: prompty.Usage{
					PromptTokens:              0,
					CompletionTokens:          0,
					TotalTokens:               0,
					PromptTokensCached:        0,
					PromptTokensCacheCreation: 0,
					CompletionTokensReasoning: 0,
					Known:                     nil,
					Mode:                      "",
				},
				Outcome:      "",
				FinishReason: "",
				Result:       nil,
				Kind:         prompty.StreamStart,
				Model:        event.Response.Model,
				Diagnostics:  prompty.CloneExecutionDiagnostics(s.creation.report),
			},
		}
		usage, usageErr := responseSSELifecycleUsage(&event.Response)
		return append(frames, usage...), usageErr
	}
	if !s.started {
		return nil, prompty.ErrStreamProtocol
	}
	switch event.Type {
	case "response.in_progress":
		if !event.JSON.Response.Valid() || event.Response.Status != responses.ResponseStatusInProgress {
			return nil, prompty.ErrStreamProtocol
		}
		if err := s.handle.observeResponse(&event.Response); err != nil {
			return nil, err
		}
		return responseSSELifecycleUsage(&event.Response)
	case "response.output_item.added":
		return s.addItem(event)
	case "response.content_part.added", "response.reasoning_summary_part.added":
		return s.addPart(event)
	case "response.output_text.delta", wireResponseRefusalDelta, "response.function_call_arguments.delta",
		"response.reasoning_summary_text.delta":
		return s.delta(event)
	case "response.output_text.done", "response.refusal.done", "response.function_call_arguments.done",
		"response.reasoning_summary_text.done", "response.content_part.done", wireResponseReasoningSummaryPartDone:
		return nil, s.checkDone(event)
	case "response.output_item.done":
		return s.doneItem(event)
	case "response.output_text.annotation.added":
		return nil, s.annotation(event)
	case "response.web_search_call.in_progress",
		"response.web_search_call.searching",
		"response.web_search_call.completed",
		"response.file_search_call.in_progress",
		"response.file_search_call.searching",
		"response.file_search_call.completed":
		item, err := s.item(event)
		if err != nil || !strings.HasPrefix(event.Type, "response."+item.initial.Type+".") {
			return nil, prompty.ErrStreamProtocol
		}
		return nil, nil
	case "response.completed", "response.failed", "response.incomplete":
		return s.terminal(event)
	case wireError:
		return nil, prompty.ErrRemoteResponseUnknown
	default:
		return nil, fmt.Errorf("%w: unsupported Responses event %q", prompty.ErrStreamProtocol, event.Type)
	}
}

// Validate consumed wire fields before using SDK values: the SDK accepts some
// numbers as strings and truncates fractional indices. Extra provider fields
// remain available in RawJSON and are not rejected by this check.
func validResponseSSESource(source []byte) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(source, &fields) != nil || fields == nil {
		return false
	}
	for _, key := range []string{wireType, "item_id", "delta", wireText, wireRefusal, "arguments", "name"} {
		if value, present := fields[key]; present && !responseSSEString(value) {
			return false
		}
	}
	for _, key := range []string{"sequence_number", "output_index", "content_index", "summary_index", "annotation_index"} {
		if value, present := fields[key]; present {
			if _, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64); err != nil {
				return false
			}
		}
	}
	for _, key := range []string{"item", "part", "response"} {
		if value, present := fields[key]; present && !validResponseSSERecord(value) {
			return false
		}
	}
	return true
}

func responseSSEString(value json.RawMessage) bool {
	if len(value) == 0 || bytes.Equal(bytes.TrimSpace(value), []byte("null")) {
		return false
	}
	var text string
	return json.Unmarshal(value, &text) == nil
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func validResponseSSERecord(source json.RawMessage) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal(source, &fields) != nil || fields == nil {
		return false
	}
	for _, key := range []string{wireType, "id", "model", "status", "role", "call_id", "name", "arguments", wireText, wireRefusal} {
		if value, present := fields[key]; present && !responseSSEString(value) {
			return false
		}
	}
	for _, key := range []string{wireOutput, "content", "summary"} {
		// Private reasoning content stays an opaque source field; its codec owns
		// validation. Only its public summary is consumed by this stream machine.
		if key == "content" && string(fields[wireType]) == `"reasoning"` {
			continue
		}
		if value, present := fields[key]; present {
			var records []json.RawMessage
			if json.Unmarshal(value, &records) != nil || records == nil {
				return false
			}
			for _, record := range records {
				if !validResponseSSERecord(record) {
					return false
				}
			}
		}
	}
	return true
}

func (s *responseSSEMachine) item(event responses.ResponseStreamEventUnion) (*responseSSEItem, error) {
	if !event.JSON.OutputIndex.Valid() || event.OutputIndex < 0 || event.OutputIndex >= int64(len(s.items)) {
		return nil, prompty.ErrStreamProtocol
	}
	item := s.items[event.OutputIndex]
	if item.done || !event.JSON.ItemID.Valid() || event.ItemID != item.initial.ID {
		return nil, prompty.ErrStreamProtocol
	}
	return item, nil
}

func (s *responseSSEMachine) part(
	event responses.ResponseStreamEventUnion,
) (*responseSSEItem, *responseSSEPart, error) {
	item, err := s.item(event)
	if err != nil {
		return nil, nil, err
	}
	index := event.ContentIndex
	if item.initial.Type == wireFunctionCall || item.initial.Type == wireReasoning {
		index = 0
	} else if !event.JSON.ContentIndex.Valid() {
		return nil, nil, prompty.ErrStreamProtocol
	}
	if index < 0 || index >= int64(len(item.parts)) {
		return nil, nil, prompty.ErrStreamProtocol
	}
	return item, item.parts[index], nil
}

//nolint:gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (s *responseSSEMachine) addItem(event responses.ResponseStreamEventUnion) ([]*prompty.ResponseChunk, error) {
	if !event.JSON.OutputIndex.Valid() || event.OutputIndex != int64(len(s.items)) ||
		!event.JSON.Item.Valid() || event.Item.ID == "" || s.ids[event.Item.ID] {
		return nil, prompty.ErrStreamProtocol
	}
	item := &responseSSEItem{
		done:            false,
		parts:           nil,
		summaries:       nil,
		summaryTextDone: nil,
		summaryPartDone: nil,
		finalized:       false,
		doneSource:      "",
		initial:         event.Item,
		summaryIndex:    -1,
	}
	var content prompty.ContentPart
	switch event.Item.Type {
	case wireMessage:
		if event.Item.Role != "assistant" || len(event.Item.Content) != 0 {
			return nil, prompty.ErrStreamProtocol
		}
	case wireReasoning:
		_, text, err := decodeResponseReasoning([]byte(event.Item.RawJSON()))
		if err != nil {
			return nil, err
		}
		if len(event.Item.Summary) != 0 {
			return nil, prompty.ErrStreamProtocol
		}
		content = prompty.ReasoningPart{CachePolicy: nil, Text: text}
		item.parts = []*responseSSEPart{
			{
				annotations: nil,
				textDone:    false,
				contentDone: false,
				id:          event.Item.ID + ":reasoning",
				kind:        wireReasoning,
				text:        text,
			},
		}
	case wireFunctionCall:
		function := event.Item.AsFunctionCall()
		if event.Item.CallID == "" || event.Item.Name == "" || !event.Item.JSON.Arguments.Valid() {
			return nil, prompty.ErrStreamProtocol
		}
		if s.creation.handle.ToolSchemaDigests[event.Item.Name] == "" {
			return nil, prompty.ErrStreamProtocol
		}
		content = prompty.ToolCallPart{Args: "", CachePolicy: nil,
			ID:        event.Item.CallID,
			Name:      event.Item.Name,
			ArgsChunk: function.Arguments,
		}
		item.parts = []*responseSSEPart{
			{
				annotations: nil,
				textDone:    false,
				contentDone: false,
				id:          event.Item.ID + ":function",
				kind:        wireFunctionCall,
				text:        function.Arguments,
			},
		}
	case wireWebSearchCall, wireFileSearchCall:
		kind := HostedWebSearch
		if event.Item.Type == wireFileSearchCall {
			kind = HostedFileSearch
		}
		allowed := false
		for _, configured := range s.creation.handle.HostedTools {
			allowed = allowed || configured == kind
		}
		if !allowed {
			return nil, prompty.ErrStreamProtocol
		}
	default:
		return nil, prompty.ErrStreamProtocol
	}
	s.items = append(s.items, item)
	s.ids[event.Item.ID] = true
	if content == nil {
		return nil, nil
	}
	return []*prompty.ResponseChunk{s.partDelta(item.parts[0], content)}, nil
}

func (s *responseSSEMachine) partDelta(part *responseSSEPart, content prompty.ContentPart) *prompty.ResponseChunk {
	return &prompty.ResponseChunk{
		Model:                   "",
		ProviderState:           nil,
		StateUpdates:            nil,
		Finalizations:           nil,
		Annotations:             nil,
		MessageAnnotations:      nil,
		ContinuationUnavailable: false,
		Usage: prompty.Usage{
			PromptTokens:              0,
			CompletionTokens:          0,
			TotalTokens:               0,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 0,
			CompletionTokensReasoning: 0,
			Known:                     nil,
			Mode:                      "",
		},
		Outcome: "",
		Diagnostics: prompty.ExecutionDiagnostics{
			Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
			Snapshot:   "",
			Provenance: "",
			Override:   false,
			Checks:     nil,
			Schemas:    nil,
			Capture:    nil,
			Cache:      nil,
		},
		FinishReason: "",
		Result:       nil,
		Kind:         prompty.StreamDelta,
		Identity: prompty.StreamIdentity{
			MessageID: "",
			PartIDs:   []string{part.id},
		},
		Content: []prompty.ContentPart{content},
	}
}

func (s *responseSSEMachine) addPart(event responses.ResponseStreamEventUnion) ([]*prompty.ResponseChunk, error) {
	item, err := s.item(event)
	if err != nil || !event.JSON.Part.Valid() {
		return nil, prompty.ErrStreamProtocol
	}
	if event.Type == "response.reasoning_summary_part.added" {
		if item.initial.Type != wireReasoning || !event.JSON.SummaryIndex.Valid() ||
			event.SummaryIndex != item.summaryIndex+1 || event.Part.Type != wireSummaryText || !event.Part.JSON.Text.Valid() {
			return nil, prompty.ErrStreamProtocol
		}
		prefix := ""
		if event.SummaryIndex > 0 {
			prefix = "\n\n"
		}
		item.summaryIndex = event.SummaryIndex
		item.summaries = append(item.summaries, event.Part.Text)
		item.summaryTextDone = append(item.summaryTextDone, false)
		item.summaryPartDone = append(item.summaryPartDone, false)
		text := prefix + event.Part.Text
		item.parts[0].text += text
		return []*prompty.ResponseChunk{
			s.partDelta(item.parts[0], prompty.ReasoningPart{CachePolicy: nil, Text: text}),
		}, nil
	}
	if item.initial.Type != wireMessage || !event.JSON.ContentIndex.Valid() ||
		event.ContentIndex != int64(len(item.parts)) {
		return nil, prompty.ErrStreamProtocol
	}
	text := event.Part.Text
	if event.Part.Type == wireRefusal {
		if !event.Part.JSON.Refusal.Valid() {
			return nil, prompty.ErrStreamProtocol
		}
		text = event.Part.Refusal
	} else if event.Part.Type != wireOutputText || !event.Part.JSON.Text.Valid() {
		return nil, prompty.ErrStreamProtocol
	}
	part := &responseSSEPart{annotations: nil, textDone: false, contentDone: false,
		id:   fmt.Sprintf("%s:content:%d", item.initial.ID, event.ContentIndex),
		kind: event.Part.Type,
		text: text,
	}
	for _, annotation := range event.Part.Annotations {
		part.annotations = append(part.annotations, []byte(annotation.RawJSON()))
	}
	item.parts = append(item.parts, part)
	chunk := s.partDelta(part, prompty.TextPart{CachePolicy: nil, Text: text})
	if part.kind == wireRefusal {
		chunk.Outcome = prompty.OutcomeRefusal
	}
	return []*prompty.ResponseChunk{chunk}, nil
}

// Summary events are valid only after their part has been declared. In
// particular, the initial sentinel (-1) is never a provider-visible index.
func (item *responseSSEItem) validSummaryIndex(event responses.ResponseStreamEventUnion) bool {
	index := event.SummaryIndex
	return event.JSON.SummaryIndex.Valid() && index >= 0 && index == item.summaryIndex &&
		index < int64(len(item.summaries)) && index < int64(len(item.summaryTextDone)) &&
		index < int64(len(item.summaryPartDone))
}

func (s *responseSSEMachine) delta(event responses.ResponseStreamEventUnion) ([]*prompty.ResponseChunk, error) {
	item, part, err := s.part(event)
	if err != nil || !event.JSON.Delta.Valid() {
		return nil, prompty.ErrStreamProtocol
	}
	if part.textDone || part.contentDone {
		return nil, prompty.ErrStreamProtocol
	}
	var content prompty.ContentPart
	switch event.Type {
	case "response.output_text.delta", wireResponseRefusalDelta:
		kind := wireOutputText
		if event.Type == wireResponseRefusalDelta {
			kind = wireRefusal
		}
		if part.kind != kind {
			return nil, prompty.ErrStreamProtocol
		}
		content = prompty.TextPart{CachePolicy: nil, Text: event.Delta}
	case "response.function_call_arguments.delta":
		if part.kind != wireFunctionCall {
			return nil, prompty.ErrStreamProtocol
		}
		content = prompty.ToolCallPart{
			Args:        "",
			CachePolicy: nil,
			ID:          item.initial.CallID,
			Name:        item.initial.Name,
			ArgsChunk:   event.Delta,
		}
	case "response.reasoning_summary_text.delta":
		if part.kind != wireReasoning || !item.validSummaryIndex(event) {
			return nil, prompty.ErrStreamProtocol
		}
		if item.summaryTextDone[event.SummaryIndex] || item.summaryPartDone[event.SummaryIndex] {
			return nil, prompty.ErrStreamProtocol
		}
		content = prompty.ReasoningPart{CachePolicy: nil, Text: event.Delta}
		item.summaries[event.SummaryIndex] += event.Delta
	}
	part.text += event.Delta
	return []*prompty.ResponseChunk{s.partDelta(part, content)}, nil
}

// Optional legacy name fields remain source-checked even when SDK unions omit them.
func responseSSENameMatches(source, expected string) bool {
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(source), &fields) != nil {
		return false
	}
	raw, present := fields["name"]
	if !present {
		return true
	}
	var name string
	return json.Unmarshal(raw, &name) == nil && name == expected
}

//nolint:cyclop,funlen,gocognit,gocyclo // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (s *responseSSEMachine) checkDone(event responses.ResponseStreamEventUnion) error {
	item, part, err := s.part(event)
	if err != nil {
		return err
	}
	text := event.Text
	switch event.Type {
	case "response.function_call_arguments.done":
		if part.kind != wireFunctionCall || !event.JSON.Arguments.Valid() ||
			!responseSSENameMatches(event.RawJSON(), item.initial.Name) {
			return prompty.ErrStreamProtocol
		}
		text = event.Arguments
	case "response.refusal.done":
		if part.kind != wireRefusal || !event.JSON.Refusal.Valid() {
			return prompty.ErrStreamProtocol
		}
		text = event.Refusal
	case "response.output_text.done":
		if part.kind != wireOutputText || !event.JSON.Text.Valid() {
			return prompty.ErrStreamProtocol
		}
	case "response.content_part.done":
		if part.contentDone || !event.JSON.Part.Valid() || event.Part.Type != part.kind {
			return prompty.ErrStreamProtocol
		}
		text = event.Part.Text
		if part.kind == wireRefusal {
			if !event.Part.JSON.Refusal.Valid() {
				return prompty.ErrStreamProtocol
			}
			text = event.Part.Refusal
		} else if !event.Part.JSON.Text.Valid() {
			return prompty.ErrStreamProtocol
		}
		var annotations []prompty.PartAnnotation
		for _, annotation := range event.Part.Annotations {
			annotations = append(
				annotations,
				prompty.PartAnnotation{
					PartIndex: 0,
					Kind:      "openai.responses.annotation",
					Value:     []byte(annotation.RawJSON()),
				},
			)
		}
		if err := checkSSEAnnotations(part.annotations, 0, annotations); err != nil {
			return err
		}
		if text != part.text {
			return prompty.ErrStreamProtocol
		}
		part.annotations = nil
		for _, annotation := range annotations {
			part.annotations = append(part.annotations, annotation.Value)
		}
		part.contentDone = true
		return nil
	case "response.reasoning_summary_text.done", wireResponseReasoningSummaryPartDone:
		if part.kind != wireReasoning || !item.validSummaryIndex(event) {
			return prompty.ErrStreamProtocol
		}
		if event.Type == wireResponseReasoningSummaryPartDone {
			if item.summaryPartDone[event.SummaryIndex] {
				return prompty.ErrStreamProtocol
			}
			if !event.JSON.Part.Valid() || event.Part.Type != wireSummaryText || !event.Part.JSON.Text.Valid() {
				return prompty.ErrStreamProtocol
			}
			text = event.Part.Text
		} else if !event.JSON.Text.Valid() {
			return prompty.ErrStreamProtocol
		}
		if text != item.summaries[event.SummaryIndex] {
			return prompty.ErrStreamProtocol
		}
		if event.Type == wireResponseReasoningSummaryPartDone {
			item.summaryPartDone[event.SummaryIndex] = true
		} else {
			if item.summaryTextDone[event.SummaryIndex] {
				return prompty.ErrStreamProtocol
			}
			item.summaryTextDone[event.SummaryIndex] = true
		}
		return nil
	}
	if part.textDone || part.contentDone || text != part.text {
		return prompty.ErrStreamProtocol
	}
	part.textDone = true
	return nil
}

//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (s *responseSSEMachine) doneItem(event responses.ResponseStreamEventUnion) ([]*prompty.ResponseChunk, error) {
	if !event.JSON.OutputIndex.Valid() || event.OutputIndex < 0 || event.OutputIndex >= int64(len(s.items)) ||
		!event.JSON.Item.Valid() {
		return nil, prompty.ErrStreamProtocol
	}
	item := s.items[event.OutputIndex]
	if item.done || event.Item.ID != item.initial.ID || event.Item.Type != item.initial.Type {
		return nil, prompty.ErrStreamProtocol
	}
	if event.Item.Type == wireFunctionCall &&
		(event.Item.CallID != item.initial.CallID || event.Item.Name != item.initial.Name) {
		return nil, prompty.ErrStreamProtocol
	}
	checked := prompty.NewResponse(nil)
	if event.Item.Type == wireWebSearchCall || event.Item.Type == wireFileSearchCall {
		hostedErr := preserveHostedItem(
			int(event.OutputIndex),
			event.Item.RawJSON(),
			s.creation.handle.HostedTools,
			checked,
			s.creation.report.Profile,
		)
		if hostedErr != nil {
			return nil, hostedErr
		}
		item.done = true
		item.doneSource = event.Item.RawJSON()
		item.finalized = true
		return []*prompty.ResponseChunk{
			{
				Model:                   "",
				Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
				Content:                 nil,
				ProviderState:           nil,
				StateUpdates:            nil,
				Finalizations:           nil,
				Annotations:             nil,
				ContinuationUnavailable: false,
				Usage: prompty.Usage{
					PromptTokens:              0,
					CompletionTokens:          0,
					TotalTokens:               0,
					PromptTokensCached:        0,
					PromptTokensCacheCreation: 0,
					CompletionTokensReasoning: 0,
					Known:                     nil,
					Mode:                      "",
				},
				Outcome: "",
				Diagnostics: prompty.ExecutionDiagnostics{
					Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
					Snapshot:   "",
					Provenance: "",
					Override:   false,
					Checks:     nil,
					Schemas:    nil,
					Capture:    nil,
					Cache:      nil,
				},
				FinishReason:       "",
				Result:             nil,
				Kind:               prompty.StreamDelta,
				MessageAnnotations: checked.MessageAnnotations,
			},
		}, nil
	}
	parseErr := parseRemoteItems([]responses.ResponseOutputItemUnion{event.Item}, checked,
		s.creation.handle.HostedTools, s.creation.report.Profile, s.creation.request.tools)
	if len(checked.Content) != len(item.parts) {
		if parseErr != nil {
			return nil, parseErr
		}
		return nil, prompty.ErrStreamProtocol
	}
	for index, part := range item.parts {
		text := ""
		switch value := checked.Content[index].(type) {
		case prompty.TextPart:
			text = value.Text
		case prompty.ReasoningPart:
			text = value.Text
		case prompty.ToolCallPart:
			text = value.Args
		}
		if text != part.text {
			return nil, prompty.ErrStreamProtocol
		}
		if err := checkSSEAnnotations(part.annotations, index, checked.Annotations); err != nil {
			return nil, err
		}
	}
	item.done = true
	// State may arrive only in the terminal response; missing provisional state
	// must not irreversibly mark an otherwise resumable response unavailable.
	if event.Item.Type == wireReasoning && len(checked.ProviderState) == 0 {
		return nil, parseErr
	}
	item.finalized = true
	item.doneSource = event.Item.RawJSON()
	chunk := &prompty.ResponseChunk{
		Model:                   "",
		Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
		Content:                 nil,
		ProviderState:           nil,
		StateUpdates:            nil,
		Finalizations:           nil,
		Annotations:             nil,
		MessageAnnotations:      nil,
		ContinuationUnavailable: false,
		Usage: prompty.Usage{
			PromptTokens:              0,
			CompletionTokens:          0,
			TotalTokens:               0,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 0,
			CompletionTokensReasoning: 0,
			Known:                     nil,
			Mode:                      "",
		},
		Outcome: "",
		Diagnostics: prompty.ExecutionDiagnostics{
			Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
			Snapshot:   "",
			Provenance: "",
			Override:   false,
			Checks:     nil,
			Schemas:    nil,
			Capture:    nil,
			Cache:      nil,
		},
		FinishReason: "",
		Result:       nil,
		Kind:         prompty.StreamDelta,
	}
	for index, part := range item.parts {
		chunk.Finalizations = append(chunk.Finalizations, responseSSEFinalization(part.id, index, checked))
	}
	return []*prompty.ResponseChunk{chunk}, parseErr
}

//nolint:funlen,gocognit // Ordered provider protocol checks preserve capture, partial-result and terminal-state semantics.
func (s *responseSSEMachine) terminal(event responses.ResponseStreamEventUnion) ([]*prompty.ResponseChunk, error) {
	if !event.JSON.Response.Valid() || "response."+string(event.Response.Status) != event.Type {
		return nil, prompty.ErrStreamProtocol
	}
	if err := s.handle.observeResponse(&event.Response); err != nil {
		return nil, err
	}
	result, parseErr := parseRemoteResponse(
		&event.Response,
		s.handle.observedHandle(),
		&prompty.RemoteResponseResult{Handle: nil, Response: nil,
			Diagnostics: s.creation.report,
		},
		s.creation.request.schema,
		s.creation.request.tools,
	)
	if result.Response == nil {
		return nil, parseErr
	}
	usageErr := validateResponseSSEUsage(event.Response.Usage.RawJSON(), &result.Response.Usage)
	parseErr = errors.Join(parseErr, usageErr)
	frames := []*prompty.ResponseChunk{
		{
			Model:                   "",
			Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
			Content:                 nil,
			ProviderState:           nil,
			StateUpdates:            nil,
			Finalizations:           nil,
			Annotations:             nil,
			MessageAnnotations:      nil,
			ContinuationUnavailable: false,
			Outcome:                 "",
			Diagnostics: prompty.ExecutionDiagnostics{
				Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
				Snapshot:   "",
				Provenance: "",
				Override:   false,
				Checks:     nil,
				Schemas:    nil,
				Capture:    nil,
				Cache:      nil,
			},
			FinishReason: "",
			Result:       nil,
			Kind:         prompty.StreamUsage,
			Usage:        result.Response.Usage,
		},
	}
	if len(event.Response.Output) != len(s.items) {
		return frames, prompty.ErrStreamProtocol
	}
	var ids []string
	var finalized []bool
	for index, raw := range event.Response.Output {
		item := s.items[index]
		if raw.ID != item.initial.ID || raw.Type != item.initial.Type || !item.done {
			return frames, prompty.ErrStreamProtocol
		}
		if item.finalized && !equalSSEJSON(item.doneSource, raw.RawJSON()) {
			return frames, prompty.ErrStreamProtocol
		}
		for _, part := range item.parts {
			ids = append(ids, part.id)
			finalized = append(finalized, item.finalized)
		}
	}
	if len(ids) != len(result.Response.Content) {
		return frames, prompty.ErrStreamProtocol
	}
	final := &prompty.ResponseChunk{
		Model:                   "",
		Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
		Content:                 nil,
		ProviderState:           nil,
		StateUpdates:            nil,
		Finalizations:           nil,
		Annotations:             nil,
		MessageAnnotations:      nil,
		ContinuationUnavailable: false,
		Usage: prompty.Usage{
			PromptTokens:              0,
			CompletionTokens:          0,
			TotalTokens:               0,
			PromptTokensCached:        0,
			PromptTokensCacheCreation: 0,
			CompletionTokensReasoning: 0,
			Known:                     nil,
			Mode:                      "",
		},
		Outcome: "",
		Diagnostics: prompty.ExecutionDiagnostics{
			Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
			Snapshot:   "",
			Provenance: "",
			Override:   false,
			Checks:     nil,
			Schemas:    nil,
			Capture:    nil,
			Cache:      nil,
		},
		FinishReason: "",
		Result:       nil,
		Kind:         prompty.StreamDelta,
	}
	for index := range result.Response.Content {
		if !finalized[index] {
			final.Finalizations = append(
				final.Finalizations,
				responseSSEFinalization(ids[index], index, result.Response),
			)
		}
	}
	frames = append(frames, final)
	// Hosted records were already emitted at item.done; only emit the complete order manifest here.
	for _, annotation := range result.Response.MessageAnnotations {
		if annotation.Kind == "openai.responses.output_order" {
			frames = append(
				frames,
				&prompty.ResponseChunk{
					Model:                   "",
					Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
					Content:                 nil,
					ProviderState:           nil,
					StateUpdates:            nil,
					Finalizations:           nil,
					Annotations:             nil,
					ContinuationUnavailable: false,
					Usage: prompty.Usage{
						PromptTokens:              0,
						CompletionTokens:          0,
						TotalTokens:               0,
						PromptTokensCached:        0,
						PromptTokensCacheCreation: 0,
						CompletionTokensReasoning: 0,
						Known:                     nil,
						Mode:                      "",
					},
					Outcome: "",
					Diagnostics: prompty.ExecutionDiagnostics{
						Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
						Snapshot:   "",
						Provenance: "",
						Override:   false,
						Checks:     nil,
						Schemas:    nil,
						Capture:    nil,
						Cache:      nil,
					},
					FinishReason:       "",
					Result:             nil,
					Kind:               prompty.StreamDelta,
					MessageAnnotations: []prompty.MessageAnnotation{annotation},
				},
			)
		}
	}
	frames = append(
		frames,
		&prompty.ResponseChunk{
			Model:              "",
			Identity:           prompty.StreamIdentity{MessageID: "", PartIDs: nil},
			Content:            nil,
			ProviderState:      nil,
			StateUpdates:       nil,
			Finalizations:      nil,
			Annotations:        nil,
			MessageAnnotations: nil,
			Usage: prompty.Usage{
				PromptTokens:              0,
				CompletionTokens:          0,
				TotalTokens:               0,
				PromptTokensCached:        0,
				PromptTokensCacheCreation: 0,
				CompletionTokensReasoning: 0,
				Known:                     nil,
				Mode:                      "",
			},
			Diagnostics: prompty.ExecutionDiagnostics{
				Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
				Snapshot:   "",
				Provenance: "",
				Override:   false,
				Checks:     nil,
				Schemas:    nil,
				Capture:    nil,
				Cache:      nil,
			},
			FinishReason:            "",
			Result:                  nil,
			Kind:                    prompty.StreamDelta,
			Outcome:                 result.Response.Outcome,
			ContinuationUnavailable: result.Response.ContinuationUnavailable,
		},
	)
	if parseErr != nil {
		return frames, parseErr
	}
	s.handle.mu.Lock()
	s.handle.handle = prompty.CloneRemoteResponseHandle(*result.Handle)
	s.handle.mu.Unlock()
	s.finished = true
	frames = append(
		frames,
		&prompty.ResponseChunk{
			Model:                   "",
			Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
			Content:                 nil,
			ProviderState:           nil,
			StateUpdates:            nil,
			Finalizations:           nil,
			Annotations:             nil,
			MessageAnnotations:      nil,
			ContinuationUnavailable: false,
			Usage: prompty.Usage{
				PromptTokens:              0,
				CompletionTokens:          0,
				TotalTokens:               0,
				PromptTokensCached:        0,
				PromptTokensCacheCreation: 0,
				CompletionTokensReasoning: 0,
				Known:                     nil,
				Mode:                      "",
			},
			Diagnostics: prompty.ExecutionDiagnostics{
				Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
				Snapshot:   "",
				Provenance: "",
				Override:   false,
				Checks:     nil,
				Schemas:    nil,
				Capture:    nil,
				Cache:      nil,
			},
			Result:       nil,
			Kind:         prompty.StreamFinish,
			Outcome:      result.Response.Outcome,
			FinishReason: result.Response.FinishReason,
		},
	)
	return frames, nil
}

func (h *responseStreamHandle) observedHandle() prompty.RemoteResponseHandle {
	h.mu.Lock()
	defer h.mu.Unlock()
	return prompty.CloneRemoteResponseHandle(h.handle)
}

func responseSSEFinalization(id string, index int, response *prompty.Response) prompty.StreamPartFinalization {
	update := prompty.StreamPartFinalization{
		ProviderState: nil,
		Annotations:   nil,
		PartID:        id,
		Content:       response.Content[index],
	}
	for _, state := range response.ProviderState {
		if state.PartIndex == index {
			owned := state.Envelope
			update.ProviderState = &owned
		}
	}
	for _, annotation := range response.Annotations {
		if annotation.PartIndex == index {
			annotation.PartIndex = 0
			update.Annotations = append(update.Annotations, annotation)
		}
	}
	return update
}

func equalSSEJSON(left, right string) bool {
	decode := func(source string) (any, error) {
		decoder := json.NewDecoder(bytes.NewBufferString(source))
		decoder.UseNumber()
		var value any
		err := decoder.Decode(&value)
		return value, err
	}
	a, errA := decode(left)
	b, errB := decode(right)
	return errA == nil && errB == nil && reflect.DeepEqual(a, b)
}

func responseSSELifecycleUsage(raw *responses.Response) ([]*prompty.ResponseChunk, error) {
	usage, err := responseUsage(raw.Usage)
	err = errors.Join(err, validateResponseSSEUsage(raw.Usage.RawJSON(), &usage))
	if len(usage.Known) == 0 && err == nil {
		return nil, nil
	}
	return []*prompty.ResponseChunk{
		{
			Model:                   "",
			Identity:                prompty.StreamIdentity{MessageID: "", PartIDs: nil},
			Content:                 nil,
			ProviderState:           nil,
			StateUpdates:            nil,
			Finalizations:           nil,
			Annotations:             nil,
			MessageAnnotations:      nil,
			ContinuationUnavailable: false,
			Outcome:                 "",
			Diagnostics: prompty.ExecutionDiagnostics{
				Profile:    prompty.ProfileIdentity{Provider: "", Endpoint: "", Model: "", ModelFamily: ""},
				Snapshot:   "",
				Provenance: "",
				Override:   false,
				Checks:     nil,
				Schemas:    nil,
				Capture:    nil,
				Cache:      nil,
			},
			FinishReason: "",
			Result:       nil,
			Kind:         prompty.StreamUsage,
			Usage:        usage,
		},
	}, err
}

// SDK presence flags omit malformed scalar values. Validate the original wire
// counters so absent usage and malformed observed usage remain distinguishable.
func validateResponseSSEUsage(source string, usage *prompty.Usage) error {
	if source == "" || source == "null" {
		return nil
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal([]byte(source), &fields) != nil || fields == nil {
		return prompty.ErrUsage
	}
	var failure error
	for _, field := range []struct {
		key, parent string
		counter     prompty.UsageCounter
		destination *int
	}{
		{"input_tokens", "", prompty.UsagePrompt, &usage.PromptTokens},
		{"output_tokens", "", prompty.UsageCompletion, &usage.CompletionTokens},
		{"total_tokens", "", prompty.UsageTotal, &usage.TotalTokens},
		{"cached_tokens", "input_tokens_details", prompty.UsageCacheRead, &usage.PromptTokensCached},
		{"reasoning_tokens", "output_tokens_details", prompty.UsageReasoning, &usage.CompletionTokensReasoning},
	} {
		sourceFields := fields
		malformed := false
		if field.parent != "" {
			parent, present := fields[field.parent]
			if !present {
				continue
			}
			sourceFields = nil
			malformed = json.Unmarshal(parent, &sourceFields) != nil || sourceFields == nil
		}
		if value, present := sourceFields[field.key]; present {
			_, err := strconv.ParseInt(strings.TrimSpace(string(value)), 10, 64)
			malformed = malformed || err != nil
		}
		if malformed {
			*field.destination = 0
			usage.Known = slices.DeleteFunc(
				usage.Known,
				func(counter prompty.UsageCounter) bool { return counter == field.counter },
			)
			failure = errors.Join(failure, prompty.ErrUsage)
		}
	}
	return failure
}
