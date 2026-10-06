package prompty

import (
	"fmt"
	"reflect"
)

// updatedPartStates is a pure plan under the stream mutex. Payloads are opaque;
// their codec validation belongs to the producer and continuation adapter.
func (s *Stream) updatedPartStates(chunk *ResponseChunk) ([]PartState, error) {
	declared := clonePartStates(chunk.ProviderState)
	declared = append(declared, clonePartStates(chunk.StateUpdates)...)
	if err := ValidatePartAssociations(chunk.Content, declared, nil); err != nil {
		return nil, err
	}
	updates := make(map[string]PartState, len(chunk.StateUpdates))
	for _, update := range chunk.StateUpdates {
		if _, tool := toolCallPartValue(chunk.Content[update.PartIndex]); !tool {
			return nil, fmt.Errorf("%w: state update requires a logical tool call", ErrProviderState)
		}
		updates[chunk.Identity.PartIDs[update.PartIndex]] = update
	}
	previous := s.status.Result.ProviderState
	var retained []PartState
	if count := len(previous) + len(declared); count != 0 {
		retained = make([]PartState, 0, count)
	}
	replaced := make(map[string]bool)
	for _, state := range previous {
		id := s.status.Identity.PartIDs[state.PartIndex]
		update, replace := updates[id]
		if !replace {
			retained = append(retained, state)
			continue
		}
		if replaced[id] || !sameStateBinding(state.Envelope, update.Envelope) {
			return nil, fmt.Errorf("%w: ambiguous or incompatible state update", ErrProviderState)
		}
		replaced[id] = true
	}
	offset := len(s.status.Result.Content)
	for _, state := range declared {
		state.PartIndex += offset
		retained = append(retained, state)
	}
	return retained, nil
}

func sameStateBinding(previous, next ProviderEnvelope) bool {
	sameExpiry := previous.ExpiresAt == nil && next.ExpiresAt == nil ||
		previous.ExpiresAt != nil && next.ExpiresAt != nil && previous.ExpiresAt.Equal(*next.ExpiresAt)
	return previous.Format == next.Format && previous.Scope == next.Scope && previous.Codec == next.Codec &&
		previous.Required == next.Required && sameExpiry
}

// Tool assembly normalizes pointer/value representations but must not change
// the source-bound fields. Non-tool parts retain their original representation.
func sameStateBoundContent(previous, next ContentPart) bool {
	oldCall, oldTool := toolCallPartValue(previous)
	newCall, newTool := toolCallPartValue(next)
	if oldTool && newTool {
		return reflect.DeepEqual(oldCall, newCall)
	}
	return reflect.DeepEqual(previous, next)
}
