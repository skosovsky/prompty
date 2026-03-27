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
		ModelOptions:   cloneModelOptions(exec.ModelOptions),
		Metadata:       clonePromptMetadata(exec.Metadata),
		ResponseFormat: cloneSchemaDefinition(exec.ResponseFormat),
	}
}

func cloneExecutionWithMessages(exec *PromptExecution, messages []ChatMessage) *PromptExecution {
	if exec == nil {
		return nil
	}
	return &PromptExecution{
		Messages:       messages,
		Tools:          cloneToolDefinitions(exec.Tools),
		ModelOptions:   cloneModelOptions(exec.ModelOptions),
		Metadata:       clonePromptMetadata(exec.Metadata),
		ResponseFormat: cloneSchemaDefinition(exec.ResponseFormat),
	}
}

func cloneModelOptions(opts *ModelOptions) *ModelOptions {
	if opts == nil {
		return nil
	}
	out := &ModelOptions{
		Model:            opts.Model,
		Stop:             cloneStringSlice(opts.Stop),
		ProviderSettings: cloneMapAny(opts.ProviderSettings),
	}
	if opts.Temperature != nil {
		v := *opts.Temperature
		out.Temperature = &v
	}
	if opts.MaxTokens != nil {
		v := *opts.MaxTokens
		out.MaxTokens = &v
	}
	if opts.TopP != nil {
		v := *opts.TopP
		out.TopP = &v
	}
	return out
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

func cloneCacheControl(cache *CacheControl) *CacheControl {
	if cache == nil {
		return nil
	}
	out := *cache
	return &out
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

func cloneMessageTemplates(messages []MessageTemplate) []MessageTemplate {
	if messages == nil {
		return nil
	}
	out := make([]MessageTemplate, len(messages))
	for i, msg := range messages {
		out[i] = MessageTemplate{
			Role:         msg.Role,
			Content:      slicesCloneTemplateParts(msg.Content),
			Optional:     msg.Optional,
			CacheControl: cloneCacheControl(msg.CacheControl),
			Metadata:     cloneMapAny(msg.Metadata),
		}
	}
	return out
}

func slicesCloneTemplateParts(parts []TemplatePart) []TemplatePart {
	if parts == nil {
		return nil
	}
	out := make([]TemplatePart, len(parts))
	for i := range parts {
		out[i] = parts[i]
		out[i].CacheControl = cloneCacheControl(parts[i].CacheControl)
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
			Role:         msg.Role,
			Content:      cloneContentParts(msg.Content),
			CacheControl: cloneCacheControl(msg.CacheControl),
			Metadata:     cloneMapAny(msg.Metadata),
		}
	}
	return out
}

func cloneChatMessage(msg ChatMessage) ChatMessage {
	return cloneMessages([]ChatMessage{msg})[0]
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
		x.CacheControl = cloneCacheControl(x.CacheControl)
		return x
	case *TextPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.CacheControl = cloneCacheControl(cp.CacheControl)
		return &cp
	case MediaPart:
		x.Data = bytes.Clone(x.Data)
		x.CacheControl = cloneCacheControl(x.CacheControl)
		return x
	case *MediaPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.Data = bytes.Clone(cp.Data)
		cp.CacheControl = cloneCacheControl(cp.CacheControl)
		return &cp
	case ReasoningPart:
		x.CacheControl = cloneCacheControl(x.CacheControl)
		return x
	case *ReasoningPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.CacheControl = cloneCacheControl(cp.CacheControl)
		return &cp
	case ToolCallPart:
		x.CacheControl = cloneCacheControl(x.CacheControl)
		return x
	case *ToolCallPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.CacheControl = cloneCacheControl(cp.CacheControl)
		return &cp
	case ToolResultPart:
		x.Content = cloneContentParts(x.Content)
		x.CacheControl = cloneCacheControl(x.CacheControl)
		return x
	case *ToolResultPart:
		if x == nil {
			return x
		}
		cp := *x
		cp.Content = cloneContentParts(cp.Content)
		cp.CacheControl = cloneCacheControl(cp.CacheControl)
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
		for i := range value.Len() {
			out.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return out
	case reflect.Array:
		out := reflect.New(value.Type()).Elem()
		for i := range value.Len() {
			out.Index(i).Set(cloneReflectValue(value.Index(i)))
		}
		return out
	default:
		return value
	}
}
