package prompty

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ManifestDescriptor is the lightweight checkpoint token for application state (manifest id + digest).
type ManifestDescriptor struct {
	ID     string `json:"id"`
	Digest string `json:"digest"` // Versioned source-bundle identity; raw single-file digests are rejected.
}

// ValidateManifestDescriptor rejects incomplete legacy identities.
func ValidateManifestDescriptor(desc ManifestDescriptor) error {
	if err := ValidateID(desc.ID); err != nil {
		return err
	}
	hash, ok := strings.CutPrefix(desc.Digest, sourceDigestPrefix)
	decoded, err := hex.DecodeString(hash)
	if !ok || err != nil || len(decoded) != sha256.Size || strings.ToLower(hash) != hash {
		return fmt.Errorf(
			"manifest descriptor: unsupported source identity: %w",
			errors.Join(ErrWireFormat, ErrManifestDigestMismatch),
		)
	}
	return nil
}

// UnmarshalJSON rejects legacy descriptors instead of silently assigning them a new meaning.
func (d *ManifestDescriptor) UnmarshalJSON(data []byte) error {
	type descriptor ManifestDescriptor
	var out descriptor
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&out); err != nil {
		return err
	}
	if err := ValidateManifestDescriptor(ManifestDescriptor(out)); err != nil {
		return err
	}
	*d = ManifestDescriptor(out)
	return nil
}

// ManifestDigestSHA256 returns hex-encoded SHA-256 of raw manifest bytes.
func ManifestDigestSHA256(manifestBytes []byte) string {
	if len(manifestBytes) == 0 {
		return ""
	}
	sum := sha256.Sum256(manifestBytes)
	return hex.EncodeToString(sum[:])
}
