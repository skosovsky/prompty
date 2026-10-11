package manifest

import (
	"context"
	"errors"
	"fmt"

	"github.com/skosovsky/prompty"
)

// CheckpointRecommend returns the identity of one captured source generation.
func CheckpointRecommend(
	ctx context.Context,
	id string,
	registry prompty.ManifestSnapshotRegistry,
) (prompty.ManifestDescriptor, error) {
	if registry == nil {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: registry is required")
	}
	snapshot, err := registry.CaptureManifest(ctx, id)
	if err != nil {
		return prompty.ManifestDescriptor{}, err
	}
	if snapshot == nil || snapshot.Descriptor().ID != id {
		return prompty.ManifestDescriptor{}, errors.New("manifest descriptor: nil snapshot")
	}
	return snapshot.Descriptor(), nil
}

// CheckpointVerify checks against the generation selected by the registry capture policy.
// For verified execution use VerifiedManifestPlan, not a separate Verify/Plan pair.
func CheckpointVerify(
	ctx context.Context,
	desc prompty.ManifestDescriptor,
	registry prompty.ManifestSnapshotRegistry,
) error {
	if err := prompty.ValidateManifestDescriptor(desc); err != nil {
		return err
	}
	if registry == nil {
		return errors.New("manifest descriptor: registry is required")
	}
	snapshot, err := registry.CaptureManifest(ctx, desc.ID)
	if err != nil {
		return err
	}
	if snapshot == nil || snapshot.Descriptor() != desc {
		return fmt.Errorf("%w: %q", prompty.ErrManifestDigestMismatch, desc.ID)
	}
	return nil
}
