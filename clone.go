package prompty

import (
	"bytes"
	"reflect"
)

// Clone returns a deep copy of the execution and all mutable nested structures.
func (e *PromptExecution) Clone() *PromptExecution {
	return clonePromptExecution(e)
}

func clonePromptExecution(exec *PromptExecution) *PromptExecution {
	if exec == nil {
		return nil
	}
	return &PromptExecution{
		Messages:       cloneMessages(exec.Messages),
		Tools:          cloneToolDefinitions(exec.Tools),
		ModelConfig:    cloneMapAny(exec.ModelConfig),
		Metadata:       clonePromptMetadata(exec.Metadata),
		ResponseFormat: cloneSchemaDefinition(exec.ResponseFormat),
	}
}

func clonePromptMetadata(meta PromptMetadata) PromptMetadata {
	return PromptMetadata{
		ID:          meta.ID,
		Version:     meta.Version,
		Description: meta.Description,
		Tags:        cloneStringSlice(meta.Tags),
		Environment: meta.Environment,
		Extras:      cloneMapAny(meta.Extras),
	}
}

func cloneSchemaDefinition(schema *SchemaDefinition) *SchemaDefinition {
	if schema == nil {
		return nil
	}
	return &SchemaDefinition{
		Name:        schema.Name,
		Description: schema.Description,
		Schema:      cloneMapAny(schema.Schema),
	}
}

func cloneToolDefinitions(tools []ToolDefinition) []ToolDefinition {
	if tools == nil {
		return nil
	}
	out := make([]ToolDefinition, len(tools))
	for i, tool := range tools {
		out[i] = ToolDefinition{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  cloneMapAny(tool.Parameters),
		}
	}
	return out
}

func cloneMessages(messages []ChatMessage) []ChatMessage {
	if messages == nil {
		return nil
	}
	out := make([]ChatMessage, len(messages))
	for i, msg := range messages {
		out[i] = ChatMessage{
			Role:       msg.Role,
			Content:    cloneContentParts(msg.Content),
			CachePoint: msg.CachePoint,
			Metadata:   cloneMapAny(msg.Metadata),
		}
	}
	return out
}

func cloneContentParts(parts []ContentPart) []ContentPart {
	if parts == nil {
		return nil
	}
	out := make([]ContentPart, len(parts))
	for i, part := range parts {
		out[i] = cloneContentPart(part)
	}
	return out
}

func cloneContentPart(part ContentPart) ContentPart {
	switch x := part.(type) {
	case TextPart:
		return x
	case *TextPart:
		if x == nil {
			return x
		}
		cp := *x
		return &cp
	case MediaPart:
		x.Data = bytes.Clone(x.Data)
		return x
	case *MediaPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.Data = bytes.Clone(cp.Data)
		return &cp
	case ReasoningPart:
		return x
	case *ReasoningPart:
		if x == nil {
			return x
		}
		cp := *x
		return &cp
	case ToolCallPart:
		return x
	case *ToolCallPart:
		if x == nil {
			return x
		}
		cp := *x
		return &cp
	case ToolResultPart:
		x.Content = cloneContentParts(x.Content)
		return x
	case *ToolResultPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.Content = cloneContentParts(cp.Content)
		return &cp
	default:
		return part
	}
}

func cloneStringSlice(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneMapAny(in map[string]any) map[string]any {
	if in == nil {
		return nil
	}
	out := make(map[string]any, len(in))
	for key, value := range in {
		out[key] = cloneAny(value)
	}
	return out
}

func cloneAny(value any) any {
	if value == nil {
		return nil
	}
	cloned := cloneReflectValue(reflect.ValueOf(value))
	if !cloned.IsValid() {
		return nil
	}
	return cloned.Interface()
}

func cloneReflectValue(value reflect.Value) reflect.Value {
	if !value.IsValid() {
		return reflect.Value{}
	}
	switch value.Kind() {
	case reflect.Interface:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		return cloneReflectValue(value.Elem())
	case reflect.Pointer:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.New(value.Type().Elem())
		out.Elem().Set(cloneReflectValue(value.Elem()))
		return out
	case reflect.Map:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		out := reflect.MakeMapWithSize(value.Type(), value.Len())
		iter := value.MapRange()
		for iter.Next() {
			out.SetMapIndex(iter.Key(), cloneReflectValue(iter.Value()))
		}
		return out
	case reflect.Slice:
		if value.IsNil() {
			return reflect.Zero(value.Type())
		}
		if value.Type().Elem().Kind() == reflect.Uint8 {
			return reflect.ValueOf(bytes.Clone(value.Bytes())).Convert(value.Type())
		}
		out := reflect.MakeSlice(value.Type(), value.Len(), value.Len())
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := 0; i < value.Len(); i++ {
			out.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return out
	default:
		return value
	}
}
