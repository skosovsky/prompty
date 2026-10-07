package prompty

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// StreamIdentity is local logical correlation, not a provider or physical-attempt ID.
// PartIDs aligns with frame Content, or with retained Result.Content in status/terminal.
// Repeated IDs across frames refer to the same logical part. Text/reasoning
// fragments stay distinct until the source explicitly finalizes their logical part.
type StreamIdentity struct {
	MessageID string   `json:"message_id"`
	PartIDs   []string `json:"part_ids,omitempty"`
}

type streamPartBinding struct {
	partType reflect.Type
	toolID   string
}

func cloneStreamIdentity(identity StreamIdentity) StreamIdentity {
	identity.PartIDs = slices.Clone(identity.PartIDs)
	return identity
}

func checkStreamIdentity(
	chunk *ResponseChunk, messageID string, previous map[string]streamPartBinding, tools map[string]string,
) (map[string]streamPartBinding, error) {
	if chunk.Identity.MessageID != "" && chunk.Identity.MessageID != messageID {
		return nil, fmt.Errorf("%w: foreign message identity", ErrStreamProtocol)
	}
	if len(chunk.Identity.PartIDs) != len(chunk.Content) {
		return nil, fmt.Errorf("%w: logical part references must align with content", ErrStreamProtocol)
	}
	updates := make(map[string]streamPartBinding, len(chunk.Content))
	newTools := make(map[string]string)
	for i, id := range chunk.Identity.PartIDs {
		if strings.TrimSpace(id) == "" {
			return nil, fmt.Errorf("%w: empty logical part reference", ErrStreamProtocol)
		}
		if _, duplicate := updates[id]; duplicate {
			return nil, fmt.Errorf("%w: duplicate logical part reference in frame", ErrStreamProtocol)
		}
		binding, err := streamBinding(chunk.Content[i])
		if err != nil {
			return nil, err
		}
		if old, exists := previous[id]; exists && old != binding {
			return nil, fmt.Errorf("%w: conflicting logical part binding", ErrStreamProtocol)
		}
		if err = bindStreamTool(id, binding.toolID, tools, newTools); err != nil {
			return nil, err
		}
		updates[id] = binding
	}
	return updates, nil
}

func bindStreamTool(partID, toolID string, previous, updates map[string]string) error {
	if toolID == "" {
		return nil
	}
	if (previous[toolID] != "" && previous[toolID] != partID) ||
		(updates[toolID] != "" && updates[toolID] != partID) {
		return fmt.Errorf("%w: tool identity bound to multiple logical parts", ErrStreamProtocol)
	}
	updates[toolID] = partID
	return nil
}

func streamBinding(part ContentPart) (streamPartBinding, error) {
	var binding streamPartBinding
	binding.partType = reflect.TypeOf(part)
	if binding.partType == nil {
		return binding, fmt.Errorf("%w: nil logical part", ErrStreamProtocol)
	}
	if binding.partType.Kind() == reflect.Pointer {
		if reflect.ValueOf(part).IsNil() {
			return binding, fmt.Errorf("%w: nil logical part", ErrStreamProtocol)
		}
		binding.partType = binding.partType.Elem()
	}
	if call, ok := toolCallPartValue(part); ok {
		binding.toolID = call.ID
	}
	return binding, nil
}

func materializedPartIDs(source []string, association []int, count int) []string {
	ids := make([]string, count)
	for i, id := range source {
		ids[association[i]] = id
	}
	return ids
}
