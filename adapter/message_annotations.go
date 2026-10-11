package adapter

import "github.com/skosovsky/prompty"

// ValidateMessageAnnotationMappings prevents direct Translate from discarding
// required history kinds that the adapter does not implement. Profile support
// is checked by Preflight, independently of this implementation declaration.
func ValidateMessageAnnotationMappings(exec *prompty.PromptExecution, desc RuntimeDescriptor) error {
	if exec == nil {
		return ErrNilExecution
	}
	for _, message := range exec.Messages {
		if err := prompty.ValidateMessageAnnotations(message.MessageAnnotations); err != nil {
			return err
		}
		for _, annotation := range message.MessageAnnotations {
			if annotation.Required {
				if _, implemented := desc.MessageAnnotationCapabilities[annotation.Kind]; !implemented {
					return ErrUnsupportedCapability
				}
			}
		}
	}
	return nil
}

func requiredMessageAnnotations(annotations []prompty.MessageAnnotation) bool {
	for _, annotation := range annotations {
		if annotation.Required {
			return true
		}
	}
	return false
}
