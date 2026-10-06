package compiled

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"

	"github.com/skosovsky/prompty"
)

const formatVersion = 3

type promptWire struct {
	FormatVersion  int                         `json:"format_version"`
	ManifestID     string                      `json:"manifest_id"`
	ManifestDigest string                      `json:"manifest_digest"`
	DigestSource   DigestSource                `json:"digest_source"`
	Execution      prompty.PromptExecutionWire `json:"execution"`
}

func marshalJSON(c *Prompt) ([]byte, error) {
	execution, err := prompty.MarshalExecution(&c.execution, prompty.WirePolicy{State: prompty.StatePreserve})
	if err != nil {
		return nil, err
	}
	return json.Marshal(
		promptWire{
			FormatVersion:  formatVersion,
			ManifestID:     c.manifestID,
			ManifestDigest: c.manifestDigest,
			DigestSource:   c.digestSource,
			Execution:      execution,
		},
	)
}

func unmarshalJSON(data []byte) (*Prompt, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	dec.DisallowUnknownFields()
	var wire promptWire
	if err := dec.Decode(&wire); err != nil {
		return nil, err
	}
	var trailing any
	if err := dec.Decode(&trailing); !errors.Is(err, io.EOF) {
		return nil, errors.New("compiled prompt: trailing JSON after document")
	}
	if wire.FormatVersion != formatVersion {
		return nil, fmt.Errorf("compiled prompt: unsupported format_version %d", wire.FormatVersion)
	}
	if wire.ManifestDigest == "" {
		return nil, errors.New("compiled prompt: manifest_digest is required")
	}
	switch wire.DigestSource {
	case DigestSourceCapturedSources:
		descriptor := prompty.ManifestDescriptor{ID: wire.ManifestID, Digest: wire.ManifestDigest}
		if err := prompty.ValidateManifestDescriptor(descriptor); err != nil {
			return nil, err
		}
	default:
		return nil, fmt.Errorf("compiled prompt: invalid digest_source %q", wire.DigestSource)
	}
	execution, err := prompty.UnmarshalExecution(wire.Execution)
	if err != nil {
		return nil, err
	}
	return &Prompt{
		execution:      *execution,
		manifestID:     wire.ManifestID,
		manifestDigest: wire.ManifestDigest,
		digestSource:   wire.DigestSource,
	}, nil
}
