package prompty

func mustJSONDocument(m map[string]any) JSONDocument {
	doc, err := MapToJSONDocument(m)
	if err != nil {
		panic(err)
	}
	return doc
}

func mustJSONDocumentMap(doc JSONDocument) map[string]any {
	out, err := JSONDocumentAsMap(doc)
	if err != nil {
		panic(err)
	}
	return out
}
