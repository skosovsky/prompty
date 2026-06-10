package prompty

import (
	"crypto/sha256"
	"encoding/hex"
)

// ManifestDescriptor is the lightweight checkpoint token for application state (manifest id + digest).
type ManifestDescriptor struct {
	ID     string `json:"id"`
	Digest string `json:"digest"` // SHA256 single-file bytes or compose closure digest (see Registry.RecommendManifestDescriptor)
}

// ManifestDigestSHA256 returns hex-encoded SHA-256 of raw manifest bytes.
func ManifestDigestSHA256(manifestBytes []byte) string {
	if len(manifestBytes) == 0 {
		return ""
	}
	sum := sha256.Sum256(manifestBytes)
	return hex.EncodeToString(sum[:])
}
