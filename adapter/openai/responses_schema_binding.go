package openai

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/skosovsky/prompty"
)

// Bind source interpretation, not just bytes. The diagnostic SourceDigest keeps
// its exact-byte meaning; remote handles use this separate scoped-contract hash.
func responseSchemaBindingDigest(schema *prompty.CompiledProviderSchema) string {
	report := schema.Report()
	value := "prompty.responses.schema-binding\x00" + report.Dialect + "\x00" +
		string(report.Role) + "\x00" + report.Name + "\x00" + report.SourceDigest
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
