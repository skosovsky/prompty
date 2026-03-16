package prompty

// SimpleChat builds a typical execution with one system instruction and one user message.
func SimpleChat(system, user string) *PromptExecution {
	return NewExecution([]ChatMessage{
		NewSystemMessage(system),
		NewUserMessage(user),
	})
}

// SimplePrompt builds an execution with only one user message.
func SimplePrompt(user string) *PromptExecution {
	return NewExecution([]ChatMessage{
		NewUserMessage(user),
	})
}
