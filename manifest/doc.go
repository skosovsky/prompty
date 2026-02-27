// Package manifest parses YAML/JSON prompt manifests into prompty.ChatPromptTemplate.
// Use ParseBytes, ParseFile, or ParseFS to load a manifest; the result is used with
// fileregistry or embedregistry, or passed to NewChatPromptTemplate callers.
package manifest
