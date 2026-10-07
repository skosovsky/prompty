package prompty

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
)

// StreamPartFinalization closes a previously observed logical part. Content must
// exactly match its accumulated text/reasoning or assembled tool arguments.
// ProviderState binds the complete Content; annotations refer to local index 0.
// It grants no continuation across transport failure and executes no handler.
type StreamPartFinalization struct {
	PartID        string
	Content       ContentPart
	ProviderState *ProviderEnvelope
	Annotations   []PartAnnotation
}

func cloneStreamFinalizations(source []StreamPartFinalization) []StreamPartFinalization {
	if source == nil {
		return nil
	}
	result := make([]StreamPartFinalization, len(source))
	for i, finalization := range source {
		result[i] = finalization
		result[i].Content = cloneContentPart(finalization.Content)
		result[i].Annotations = clonePartAnnotations(finalization.Annotations)
		if finalization.ProviderState != nil {
			state := clonePartStates([]PartState{{PartIndex: 0, Envelope: *finalization.ProviderState}})
			result[i].ProviderState = &state[0].Envelope
		}
	}
	return result
}

// Work on the accumulate candidate only; the caller commits it atomically.
func finalizeStreamParts(
	result *Response,
	identity *StreamIdentity,
	updates []StreamPartFinalization,
	sealed map[string]bool,
) error {
	if len(updates) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(updates))
	for _, update := range updates {
		if strings.TrimSpace(update.PartID) == "" || sealed[update.PartID] || seen[update.PartID] {
			return fmt.Errorf("%w: repeated or empty part finalization", ErrStreamProtocol)
		}
		seen[update.PartID] = true
		if err := finalizeStreamPart(result, identity, update); err != nil {
			return err
		}
	}
	return nil
}

func finalizeStreamPart(result *Response, identity *StreamIdentity, update StreamPartFinalization) error {
	var fragments []ContentPart
	first := -1
	for i, id := range identity.PartIDs {
		if id == update.PartID {
			if first == -1 {
				first = i
			}
			fragments = append(fragments, result.Content[i])
		}
	}
	if first == -1 {
		return fmt.Errorf("%w: finalization of unknown logical part", ErrStreamProtocol)
	}
	if err := validateFinalContent(fragments, update.Content, update.PartID); err != nil {
		return err
	}
	var finalState []PartState
	if update.ProviderState != nil {
		finalState = []PartState{{PartIndex: 0, Envelope: *update.ProviderState}}
	}
	if err := ValidatePartAssociations([]ContentPart{update.Content}, finalState, update.Annotations); err != nil {
		return err
	}

	parts, ids, association, target := foldFinalizedPart(
		result.Content,
		identity.PartIDs,
		update,
		first,
		len(fragments),
	)
	states, err := finalizedPartStates(result, identity.PartIDs, update, association, target)
	if err != nil {
		return err
	}
	annotations := clonePartAnnotations(result.Annotations)
	for i := range annotations {
		annotations[i].PartIndex = association[annotations[i].PartIndex]
	}
	for _, annotation := range clonePartAnnotations(update.Annotations) {
		annotation.PartIndex = target
		annotations = append(annotations, annotation)
	}
	if err := ValidatePartAssociations(parts, states, annotations); err != nil {
		return err
	}
	result.Content, result.ProviderState, result.Annotations = parts, states, annotations
	identity.PartIDs = ids
	return nil
}

func foldFinalizedPart(
	content []ContentPart,
	sourceIDs []string,
	update StreamPartFinalization,
	first, count int,
) ([]ContentPart, []string, []int, int) {
	parts := make([]ContentPart, 0, len(content)-count+1)
	ids := make([]string, 0, cap(parts))
	association := make([]int, len(content))
	target := -1
	for i, id := range sourceIDs {
		switch {
		case id != update.PartID:
			association[i] = len(parts)
			parts = append(parts, content[i])
			ids = append(ids, id)
		case i == first:
			target = len(parts)
			association[i] = target
			parts = append(parts, cloneContentPart(update.Content))
			ids = append(ids, id)
		default:
			association[i] = target
		}
	}
	return parts, ids, association, target
}

func finalizedPartStates(
	result *Response,
	ids []string,
	update StreamPartFinalization,
	association []int,
	target int,
) ([]PartState, error) {
	states := make([]PartState, 0, len(result.ProviderState)+1)
	for _, state := range result.ProviderState {
		if ids[state.PartIndex] == update.PartID {
			if !sameStateBoundContent(result.Content[state.PartIndex], update.Content) {
				return nil, fmt.Errorf("%w: finalization alters state-bound content", ErrProviderState)
			}
			if update.ProviderState != nil {
				if !sameStateBinding(state.Envelope, *update.ProviderState) {
					return nil, fmt.Errorf("%w: finalization changes state binding", ErrProviderState)
				}
				continue
			}
		}
		state.PartIndex = association[state.PartIndex]
		states = append(states, state)
	}
	if update.ProviderState != nil {
		finalState := clonePartStates([]PartState{{PartIndex: target, Envelope: *update.ProviderState}})
		states = append(states, finalState...)
	}
	return states, nil
}

func validateFinalContent(fragments []ContentPart, final ContentPart, partID string) error {
	binding, err := streamBinding(final)
	if err != nil {
		return err
	}
	for _, fragment := range fragments {
		observed, err := streamBinding(fragment)
		if err != nil || observed != binding {
			return fmt.Errorf("%w: finalization changes part type or ID", ErrStreamProtocol)
		}
	}
	if _, tool := toolCallPartValue(final); tool {
		references := slices.Repeat([]string{partID}, len(fragments))
		assembled, _, err := assembleToolCallPartsWithReferences(fragments, references)
		if err != nil || len(assembled) != 1 || !sameStateBoundContent(assembled[0], final) {
			return fmt.Errorf("%w: finalization changes tool arguments", ErrStreamProtocol)
		}
		return nil
	}
	text, policy, textPart := finalizedTextValue(final)
	if !textPart {
		return fmt.Errorf("%w: finalization requires text, reasoning or tool part", ErrStreamProtocol)
	}
	var observed strings.Builder
	for _, fragment := range fragments {
		value, fragmentPolicy, ok := finalizedTextValue(fragment)
		if !ok || !reflect.DeepEqual(fragmentPolicy, policy) {
			return fmt.Errorf("%w: finalization changes content policy", ErrStreamProtocol)
		}
		observed.WriteString(value)
	}
	if observed.String() != text {
		return fmt.Errorf("%w: finalization changes observed text", ErrStreamProtocol)
	}
	return nil
}

func finalizedTextValue(part ContentPart) (string, *CachePolicy, bool) {
	switch value := part.(type) {
	case TextPart:
		return value.Text, value.CachePolicy, true
	case *TextPart:
		if value != nil {
			return value.Text, value.CachePolicy, true
		}
	case ReasoningPart:
		return value.Text, value.CachePolicy, true
	case *ReasoningPart:
		if value != nil {
			return value.Text, value.CachePolicy, true
		}
	}
	return "", nil, false
}
