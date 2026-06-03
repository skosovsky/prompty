package prompty

import (
	"errors"
	"fmt"
)

// ErrIncompleteToolCallArgs indicates ArgsChunk was not glued into Args before validation.
var ErrIncompleteToolCallArgs = errors.New("prompty: tool call args incomplete (ArgsChunk without Args)")

// GlueToolCallArgChunks aggregates streaming ArgsChunk sequences by tool_call_id into Args.
func GlueToolCallArgChunks(parts []ContentPart) ([]ContentPart, error) {
	if len(parts) == 0 {
		return parts, nil
	}
	out := make([]ContentPart, 0, len(parts))
	for i := 0; i < len(parts); {
		switch parts[i].(type) {
		case ToolCallPart, *ToolCallPart:
			merged, next, err := glueToolCallRun(parts, i)
			if err != nil {
				return nil, err
			}
			out = append(out, merged)
			i = next
		default:
			out = append(out, parts[i])
			i++
		}
	}
	return out, nil
}

func glueToolCallRun(parts []ContentPart, start int) (ContentPart, int, error) {
	first, ok := toolCallPartValue(parts[start])
	if !ok {
		return parts[start], start + 1, errors.New("not a tool call part")
	}
	merged := seedStreamingToolCallArgs(first)
	i := start + 1
	for i < len(parts) {
		next, ok := toolCallPartValue(parts[i])
		if !ok {
			break
		}
		if !sameToolCallStream(merged, next) {
			break
		}
		var err error
		merged, err = mergeStreamingToolCall(merged, next)
		if err != nil {
			return merged, i, err
		}
		i++
	}
	return finalizeGluedToolCall(merged), i, nil
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

func sameToolCallStream(a, b ToolCallPart) bool {
	if a.ID == "" || b.ID == "" {
		return false
	}
	return a.ID == b.ID
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
		return acc, fmt.Errorf("prompty: conflicting tool names %q and %q for call %q", acc.Name, next.Name, acc.ID)
	}
	if next.Name != "" {
		acc.Name = next.Name
	}
	if next.ID != "" {
		acc.ID = next.ID
	}
	if next.ArgsChunk != "" {
		acc.Args += next.ArgsChunk
	} else if next.Args != "" {
		if acc.Args != "" && acc.Args != next.Args {
			return acc, fmt.Errorf("prompty: conflicting Args for tool call %q", acc.ID)
		}
		acc.Args = next.Args
	}
	acc.ArgsChunk = ""
	return acc, nil
}

func finalizeGluedToolCall(tc ToolCallPart) ToolCallPart {
	if tc.Args == "" && tc.ArgsChunk != "" {
		tc.Args = tc.ArgsChunk
	}
	tc.ArgsChunk = ""
	return tc
}

// ToolCallArgsForTranslate resolves tool-call JSON for adapter translation (fail-closed on ArgsChunk without Args).
func ToolCallArgsForTranslate(tc ToolCallPart) (string, error) {
	return resolvedToolCallArgs(tc)
}

func resolvedToolCallArgs(tc ToolCallPart) (string, error) {
	if tc.Args != "" {
		if tc.ArgsChunk != "" && tc.ArgsChunk != tc.Args {
			return "", fmt.Errorf("prompty: conflicting Args and ArgsChunk for tool %q", tc.Name)
		}
		return tc.Args, nil
	}
	if tc.ArgsChunk != "" {
		return "", ErrIncompleteToolCallArgs
	}
	return "", fmt.Errorf("prompty: tool call %q has empty args", tc.Name)
}
