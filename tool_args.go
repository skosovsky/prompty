package prompty

import (
	"errors"
	"fmt"
)

// ErrIncompleteToolCallArgs indicates incomplete or uncorrelated streaming arguments.
var ErrIncompleteToolCallArgs = errors.New("prompty: tool call arguments incomplete")

// AssembleToolCallParts materializes complete calls by stable identity. Text and
// other parts retain their positions; each call occupies its first delta position.
// It must be called after the provider has completed the calls, never on a prefix.
func AssembleToolCallParts(parts []ContentPart) ([]ContentPart, error) {
	out, _, err := assembleToolCallParts(parts)
	return out, err
}

// Each source part maps to its materialized position, including interleaved
// deltas that collapse into their call's first position.
func assembleToolCallParts(parts []ContentPart) ([]ContentPart, []int, error) {
	return assembleToolCallPartsWithReferences(parts, nil)
}

// references are native-source logical identities checked by the owning handle,
// never guessed by adjacency/name or converted into fabricated provider IDs.
func assembleToolCallPartsWithReferences(parts []ContentPart, references []string) ([]ContentPart, []int, error) {
	if references != nil && len(references) != len(parts) {
		return nil, nil, ErrStreamProtocol
	}
	out := make([]ContentPart, 0, len(parts))
	association := make([]int, len(parts))
	positions := make(map[string]int)
	argumentModes := make(map[string]bool)
	for i, part := range parts {
		call, ok := toolCallPartValue(part)
		if !ok {
			association[i] = len(out)
			out = append(out, cloneContentPart(part))
			continue
		}
		identity, identityErr := prepareToolCallAssembly(call, references, i, argumentModes)
		if identityErr != nil {
			return nil, nil, identityErr
		}
		call.CachePolicy = cloneCachePolicy(call.CachePolicy)
		if identity == "" {
			if call.ArgsChunk != "" {
				return nil, nil, fmt.Errorf("%w: delta has no correlation identity", ErrIncompleteToolCallArgs)
			}
			association[i] = len(out)
			out = append(out, call)
			continue
		}
		position, exists := positions[identity]
		if !exists {
			association[i] = len(out)
			positions[identity] = len(out)
			out = append(out, seedStreamingToolCallArgs(call))
			continue
		}
		association[i] = position
		previous, ok := toolCallPartValue(out[position])
		if !ok {
			return nil, nil, ErrIncompleteToolCallArgs
		}
		merged, err := mergeStreamingToolCall(previous, call)
		if err != nil {
			return nil, nil, err
		}
		out[position] = merged
	}
	return out, association, validateAssembledCalls(out)
}

func prepareToolCallAssembly(call ToolCallPart, references []string, index int, modes map[string]bool) (string, error) {
	if err := validateToolCallSource(call); err != nil {
		return "", err
	}
	identity, err := toolAssemblyIdentity(call, references, index)
	if err != nil {
		return "", err
	}
	return identity, checkToolArgumentMode(modes, call, identity)
}

func toolAssemblyIdentity(call ToolCallPart, references []string, index int) (string, error) {
	if call.ID != "" {
		return "provider:" + call.ID, nil
	}
	if references == nil {
		return "", nil
	}
	if references[index] == "" {
		return "", ErrStreamProtocol
	}
	return "local:" + references[index], nil
}

// Preserve the source representation before seeding collapses deltas into Args.
// Empty identity-only frames establish no argument mode.
func checkToolArgumentMode(modes map[string]bool, call ToolCallPart, identity string) error {
	if identity == "" || (call.Args == "" && call.ArgsChunk == "") {
		return nil
	}
	snapshot := call.Args != ""
	if previous, exists := modes[identity]; exists && previous != snapshot {
		return fmt.Errorf("%w: mixed snapshot and delta frames", ErrIncompleteToolCallArgs)
	}
	modes[identity] = snapshot
	return nil
}

func validateToolCallSource(call ToolCallPart) error {
	if call.Args == "" {
		return nil
	}
	if call.ArgsChunk != "" {
		return fmt.Errorf("%w: ambiguous snapshot and delta arguments", ErrIncompleteToolCallArgs)
	}
	args, err := SchemaDocumentAsMap(JSONDocument(call.Args))
	if err != nil || args == nil {
		return fmt.Errorf("%w: snapshot must be a complete JSON object", ErrIncompleteToolCallArgs)
	}
	return nil
}

func validateAssembledCalls(parts []ContentPart) error {
	for _, part := range parts {
		call, ok := toolCallPartValue(part)
		if !ok {
			continue
		}
		if call.Name == "" || call.Args == "" {
			return fmt.Errorf("%w: missing name or arguments", ErrIncompleteToolCallArgs)
		}
		args, err := SchemaDocumentAsMap(JSONDocument(call.Args))
		if err != nil || args == nil {
			return fmt.Errorf("%w: arguments must be a complete JSON object", ErrIncompleteToolCallArgs)
		}
	}
	return nil
}

func toolCallPartValue(part ContentPart) (ToolCallPart, bool) {
	switch x := part.(type) {
	case ToolCallPart:
		return x, true
	case *ToolCallPart:
		if x == nil {
			return ToolCallPart{}, false
		}
		return *x, true
	default:
		return ToolCallPart{}, false
	}
}

func seedStreamingToolCallArgs(tc ToolCallPart) ToolCallPart {
	if tc.Args == "" && tc.ArgsChunk != "" {
		tc.Args = tc.ArgsChunk
		tc.ArgsChunk = ""
	}
	return tc
}

func mergeStreamingToolCall(acc, next ToolCallPart) (ToolCallPart, error) {
	if next.Name != "" && acc.Name != "" && next.Name != acc.Name {
		return acc, fmt.Errorf("%w: conflicting tool names", ErrIncompleteToolCallArgs)
	}
	if next.Name != "" {
		acc.Name = next.Name
	}
	if next.ArgsChunk != "" {
		if next.Args != "" {
			return acc, fmt.Errorf("%w: ambiguous snapshot and delta arguments", ErrIncompleteToolCallArgs)
		}
		acc.Args += next.ArgsChunk
	} else if next.Args != "" {
		if acc.Args != "" && acc.Args != next.Args {
			return acc, fmt.Errorf("%w: conflicting Args snapshots", ErrIncompleteToolCallArgs)
		}
		acc.Args = next.Args
	}
	acc.ArgsChunk = ""
	return acc, nil
}

// ToolCallArgsForTranslate resolves complete tool-call JSON, rejecting unassembled deltas.
func ToolCallArgsForTranslate(tc ToolCallPart) (string, error) {
	return resolvedToolCallArgs(tc)
}

func resolvedToolCallArgs(tc ToolCallPart) (string, error) {
	if tc.Args != "" {
		if tc.ArgsChunk != "" {
			return "", fmt.Errorf("%w: conflicting Args and ArgsChunk", ErrIncompleteToolCallArgs)
		}
		return tc.Args, nil
	}
	if tc.ArgsChunk != "" {
		return "", ErrIncompleteToolCallArgs
	}
	return "", fmt.Errorf("prompty: tool call %q has empty args", tc.Name)
}
