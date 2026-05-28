package prompty

// DescriptorFromTemplate builds a manifest descriptor from an already-loaded template.
func DescriptorFromTemplate(tpl *ChatPromptTemplate) TemplateDescriptor {
	if tpl == nil {
		return TemplateDescriptor{}
	}
	seen := make(map[string]bool)
	var layerIDs []string
	for _, msg := range tpl.Messages {
		if msg.LayerID == "" || seen[msg.LayerID] {
			continue
		}
		seen[msg.LayerID] = true
		layerIDs = append(layerIDs, msg.LayerID)
	}
	return TemplateDescriptor{
		Metadata:          tpl.Metadata,
		ModelOptions:      cloneModelOptions(tpl.ModelOptions),
		Tools:             cloneToolDefinitions(tpl.Tools),
		RequiredTools:     cloneStringSlice(tpl.RequiredTools),
		RequiredInputVars: cloneStringSlice(tpl.RequiredVars),
		InputSchema:       cloneSchemaDefinition(tpl.InputSchema),
		ResponseFormat:    cloneSchemaDefinition(tpl.ResponseFormat),
		LayerIDs:          layerIDs,
	}
}
