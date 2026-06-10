package manifest

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/skosovsky/prompty"
)

// CheckpointRecommend selects a single-file or compose-closure checkpoint descriptor.
// Called by registry packages only; applications use ManifestCheckpointRegistry.
func CheckpointRecommend(
	ctx context.Context,
	id string,
	reader prompty.ManifestBytesReader,
	u Unmarshaler,
) (prompty.ManifestDescriptor, error) {
	return recommendManifestDescriptor(ctx, id, reader, u)
}

// CheckpointVerify verifies a descriptor against single-file or compose-closure digests.
// Called by registry packages only; applications use ManifestCheckpointRegistry.
func CheckpointVerify(
	ctx context.Context,
	desc prompty.ManifestDescriptor,
	reader prompty.ManifestBytesReader,
	u Unmarshaler,
) error {
	return verifyManifestDescriptor(ctx, desc, reader, u)
}

func recommendManifestDescriptor(
	ctx context.Context,
	id string,
	reader prompty.ManifestBytesReader,
	u Unmarshaler,
) (prompty.ManifestDescriptor, error) {
	if id == "" {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: id is required")
	}
	if reader == nil {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: reader is required")
	}
	if u == nil {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: unmarshaller is required")
	}
	if ctx.Err() != nil {
		return prompty.ManifestDescriptor{}, ctx.Err()
	}
	raw, err := reader.ReadManifestBytes(ctx, id)
	if err != nil {
		return prompty.ManifestDescriptor{}, err
	}
	if len(raw) == 0 {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: manifest bytes are empty")
	}
	usesCompose, peekErr := PeekComposeFieldsE(raw, u)
	if peekErr != nil {
		return prompty.ManifestDescriptor{}, fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
	}
	if usesCompose {
		return newComposeManifestDescriptor(ctx, id, reader, u)
	}
	return prompty.ManifestDescriptor{ID: id, Digest: prompty.ManifestDigestSHA256(raw)}, nil
}

func newComposeManifestDescriptor(
	ctx context.Context,
	id string,
	reader prompty.ManifestBytesReader,
	u Unmarshaler,
) (prompty.ManifestDescriptor, error) {
	if id == "" {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: id is required")
	}
	if reader == nil {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: reader is required")
	}
	if u == nil {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: unmarshaller is required")
	}
	if ctx.Err() != nil {
		return prompty.ManifestDescriptor{}, ctx.Err()
	}
	read := func(importID string) ([]byte, error) {
		return reader.ReadManifestBytes(ctx, importID)
	}
	digest, err := ComposeClosureDigestSHA256(id, read, u)
	if err != nil {
		return prompty.ManifestDescriptor{}, wrapCheckpointComposeDigestErr(err)
	}
	return prompty.ManifestDescriptor{ID: id, Digest: digest}, nil
}

func verifyComposeManifestDescriptor(
	ctx context.Context,
	desc prompty.ManifestDescriptor,
	reader prompty.ManifestBytesReader,
	u Unmarshaler,
) error {
	if reader == nil {
		return errors.New("manifest descriptor: reader is required")
	}
	if u == nil {
		return errors.New("manifest descriptor: unmarshaller is required")
	}
	if desc.ID == "" {
		return errors.New("manifest descriptor: id is required")
	}
	if desc.Digest == "" {
		return errors.New("manifest descriptor: digest is required")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	read := func(importID string) ([]byte, error) {
		return reader.ReadManifestBytes(ctx, importID)
	}
	digest, err := ComposeClosureDigestSHA256(desc.ID, read, u)
	if err != nil {
		return wrapCheckpointComposeDigestErr(err)
	}
	if digest != desc.Digest {
		return fmt.Errorf("%w: %q", prompty.ErrManifestDigestMismatch, desc.ID)
	}
	return nil
}

// wrapCheckpointComposeDigestErr maps compose-closure digest failures to checkpoint errors.
// Missing deployable imports propagate as-is (typically ErrTemplateNotFound).
func wrapCheckpointComposeDigestErr(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, prompty.ErrTemplateNotFound) {
		return err
	}
	msg := err.Error()
	if strings.Contains(msg, "compose digest: read import") {
		return err
	}
	return fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, err)
}

func verifyManifestDescriptor(
	ctx context.Context,
	desc prompty.ManifestDescriptor,
	reader prompty.ManifestBytesReader,
	u Unmarshaler,
) error {
	if reader == nil {
		return errors.New("manifest descriptor: reader is required")
	}
	if u == nil {
		return errors.New("manifest descriptor: unmarshaller is required")
	}
	if desc.ID == "" {
		return errors.New("manifest descriptor: id is required")
	}
	if desc.Digest == "" {
		return errors.New("manifest descriptor: digest is required")
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	raw, err := reader.ReadManifestBytes(ctx, desc.ID)
	if err != nil {
		return err
	}
	usesCompose, peekErr := PeekComposeFieldsE(raw, u)
	if peekErr != nil {
		return fmt.Errorf("%w: %w", prompty.ErrInvalidManifest, peekErr)
	}
	if usesCompose {
		return verifyComposeManifestDescriptor(ctx, desc, reader, u)
	}
	if prompty.ManifestDigestSHA256(raw) != desc.Digest {
		return fmt.Errorf("%w: %q", prompty.ErrManifestDigestMismatch, desc.ID)
	}
	return nil
}
