package openai

// SupportsForcedTool reports actual native named-tool request mapping, not model knowledge.
func (*Adapter) SupportsForcedTool() bool { return true }
