package manifest

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/skosovsky/prompty"
)

// ComposeClosureDigestSHA256 returns a composite digest covering the main manifest and all
// transitive import manifest bytes (conservative closure without runtime compose values).
func ComposeClosureDigestSHA256(
	mainID string,
	readBytes func(id string) ([]byte, error),
	u Unmarshaler,
) (string, error) {
	if mainID == "" {
		return "", errors.New("compose digest: id is required")
	}
	if readBytes == nil {
		return "", errors.New("compose digest: readBytes is required")
	}
	if u == nil {
		return "", errors.New("compose digest: unmarshaller is required")
	}
	mainBytes, err := readBytes(mainID)
	if err != nil {
		return "", err
	}
	if len(mainBytes) == 0 {
		return "", errors.New("compose digest: manifest bytes are required")
	}
	usesCompose, peekErr := PeekComposeFieldsE(mainBytes, u)
	if peekErr != nil {
		return "", peekErr
	}
	if !usesCompose {
		return prompty.ManifestDigestSHA256(mainBytes), nil
	}
	var raw RawManifest
	if unmarshalErr := u.Unmarshal(mainBytes, &raw); unmarshalErr != nil {
		return "", fmt.Errorf("compose digest: unmarshal main: %w", unmarshalErr)
	}
	entries := []string{mainID + ":" + prompty.ManifestDigestSHA256(mainBytes)}
	if err := appendComposeDigestEntries(
		&raw, readBytes, u, &entries, map[string]bool{}, map[string]bool{},
	); err != nil {
		return "", err
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\n")))
	return hex.EncodeToString(sum[:]), nil
}

func appendComposeDigestEntries(
	raw *RawManifest,
	readBytes func(id string) ([]byte, error),
	u Unmarshaler,
	entries *[]string,
	done map[string]bool,
	stack map[string]bool,
) error {
	if raw == nil {
		return nil
	}
	for _, imp := range raw.Imports {
		if imp.ID == "" {
			return errors.New("compose digest: import id is required")
		}
		if stack[imp.ID] {
			return fmt.Errorf("compose digest: cyclic import %q", imp.ID)
		}
		if done[imp.ID] {
			continue
		}
		stack[imp.ID] = true
		childBytes, err := readBytes(imp.ID)
		if err != nil {
			return fmt.Errorf("compose digest: read import %q: %w", imp.ID, err)
		}
		*entries = append(*entries, imp.ID+":"+prompty.ManifestDigestSHA256(childBytes))
		var child RawManifest
		if unmarshalErr := u.Unmarshal(childBytes, &child); unmarshalErr != nil {
			return fmt.Errorf("compose digest: unmarshal import %q: %w", imp.ID, unmarshalErr)
		}
		if err := appendComposeDigestEntries(&child, readBytes, u, entries, done, stack); err != nil {
			return err
		}
		done[imp.ID] = true
		delete(stack, imp.ID)
	}
	return nil
}
