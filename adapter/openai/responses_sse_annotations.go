package openai

import (
	"bytes"
	"encoding/json"

	"github.com/openai/openai-go/v3/responses"

	"github.com/skosovsky/prompty"
)

func (s *responseSSEMachine) annotation(event responses.ResponseStreamEventUnion) error {
	_, part, err := s.part(event)
	if err != nil || part.contentDone || part.kind != "output_text" || !event.JSON.Annotation.Valid() ||
		!event.JSON.AnnotationIndex.Valid() || event.AnnotationIndex != int64(len(part.annotations)) {
		return prompty.ErrStreamProtocol
	}
	var source map[string]json.RawMessage
	if json.Unmarshal([]byte(event.RawJSON()), &source) != nil {
		return prompty.ErrStreamProtocol
	}
	annotation := source["annotation"]
	if len(annotation) == 0 || annotation[0] != '{' {
		return prompty.ErrStreamProtocol
	}
	part.annotations = append(part.annotations, bytes.Clone(annotation))
	return nil
}

// Known annotation events must survive item finalization unchanged. Additional
// annotations first supplied by the completed item remain representable.
func checkSSEAnnotations(observed []prompty.JSONDocument, index int, complete []prompty.PartAnnotation) error {
	var annotations []prompty.JSONDocument
	for _, annotation := range complete {
		if annotation.PartIndex == index {
			annotations = append(annotations, annotation.Value)
		}
	}
	if len(observed) > len(annotations) {
		return prompty.ErrStreamProtocol
	}
	for i, annotation := range observed {
		if !equalSSEJSON(string(annotation), string(annotations[i])) {
			return prompty.ErrStreamProtocol
		}
	}
	return nil
}
