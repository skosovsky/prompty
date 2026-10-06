package manifest_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/manifest"
	"github.com/skosovsky/prompty/remoteregistry"
)

const ownershipSource = `{"id":"agent","messages":[{"role":"user","content":[{"type":"text","text":"OLD"}]}]}`

type ownershipReader struct{}

func (ownershipReader) ReadManifestBytes(context.Context, string) ([]byte, error) {
	return []byte(ownershipSource), nil
}

type ownershipParser struct {
	saved  *manifest.RawManifest
	mutate bool
}

func (p *ownershipParser) Unmarshal(in []byte, out any) error {
	if err := manifest.NewJSONParser().Unmarshal(in, out); err != nil {
		return err
	}
	p.saved = out.(*manifest.RawManifest)
	if p.mutate {
		copy(in, bytes.ReplaceAll(in, []byte("OLD"), []byte("NEW")))
	}
	return nil
}
func TestCaptureSourcesRejectsParserInputMutation(t *testing.T) {
	t.Parallel()
	// Arrange.
	parser := &ownershipParser{saved: nil, mutate: true}
	// Act.
	snapshot, err := manifest.CaptureSources(context.Background(), "agent", ownershipReader{}, parser, nil, nil)
	// Assert: no identity may attest to mutated bytes and an earlier parsed value.
	require.ErrorIs(t, err, prompty.ErrInvalidManifest)
	require.Nil(t, snapshot)
}
func TestCaptureSourcesSealsParserOutput(t *testing.T) {
	t.Parallel()
	// Arrange.
	parser := &ownershipParser{saved: nil, mutate: false}
	snapshot, err := manifest.CaptureSources(context.Background(), "agent", ownershipReader{}, parser, nil, nil)
	require.NoError(t, err)
	descriptor := snapshot.Descriptor()
	// Act: a parser retaining its output cannot mutate the cached closure.
	parser.saved.Messages[0].Content[0].Text = "NEW"
	plan, err := snapshot.Plan(context.Background(), prompty.RegistryPlanInput{})
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	// Assert.
	require.NoError(t, err)
	require.Equal(t, descriptor, snapshot.Descriptor())
	require.True(t, bytes.Equal([]byte(ownershipSource), snapshot.Sources().Sources["manifest:agent"]))
	require.Equal(t, "OLD", execution.Messages[0].Content[0].(prompty.TextPart).Text)
}

type ownershipFetcher struct{ data []byte }

func (f *ownershipFetcher) Fetch(context.Context, string) ([]byte, error) { return f.data, nil }
func TestRemoteCaptureRejectsParserMutationBeforeSourceSealing(t *testing.T) {
	t.Parallel()
	// Arrange: Fetch returns a caller-owned buffer.
	source := &ownershipFetcher{data: []byte(ownershipSource)}
	parser := &ownershipParser{saved: nil, mutate: true}
	registry, err := remoteregistry.New(source, remoteregistry.WithParser(parser))
	require.NoError(t, err)
	// Act.
	snapshot, err := registry.CaptureManifest(context.Background(), "agent")
	// Assert: no preliminary parser call may modify fetched bytes before capture.
	require.ErrorIs(t, err, prompty.ErrInvalidManifest)
	require.Nil(t, snapshot)
	require.True(t, bytes.Equal([]byte(ownershipSource), source.data))
}
func TestPeekComposeRejectsInputMutation(t *testing.T) {
	t.Parallel()
	// Arrange.
	source := []byte(ownershipSource)
	parser := &ownershipParser{saved: nil, mutate: true}
	// Act.
	_, err := manifest.PeekComposeFieldsE(source, parser)
	// Assert.
	require.ErrorIs(t, err, prompty.ErrInvalidManifest)
	require.True(t, bytes.Equal([]byte(ownershipSource), source))
}
