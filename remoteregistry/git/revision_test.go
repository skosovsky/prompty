package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/parser/yaml"
	"github.com/skosovsky/prompty/remoteregistry"
)

func TestRegistry_ImmutableRevisionIgnoresMovingCheckout(t *testing.T) {
	t.Parallel()
	// Arrange: one immutable commit and a persistent clone.
	repository := t.TempDir()
	old := `{"id":"agent","messages":[{"role":"user","content":[{"type":"text","text":"OLD"}]}]}`
	initRepo(t, repository, map[string]string{"prompts/agent.yaml": old})
	hashCommand := exec.Command("git", "rev-parse", "HEAD")
	hashCommand.Dir = repository
	hash, err := hashCommand.Output()
	require.NoError(t, err)
	clone := t.TempDir()
	fetcher, err := NewFetcher("file://"+repository, WithCloneDir(clone), WithDir("prompts"), WithDepth(0))
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, fetcher.Close()) })
	registry, err := remoteregistry.New(
		fetcher,
		remoteregistry.WithParser(yaml.New()),
		remoteregistry.WithRevision(strings.TrimSpace(string(hash))),
	)
	require.NoError(t, err)
	first, err := registry.CaptureManifest(context.Background(), "agent")
	require.NoError(t, err)
	// Act: checkout bytes change while the explicitly selected commit stays bound.
	require.NoError(
		t,
		os.WriteFile(
			filepath.Join(clone, "prompts", "agent.yaml"),
			[]byte(strings.ReplaceAll(old, "OLD", "NEW")),
			0600,
		),
	)
	second, err := registry.CaptureManifest(context.Background(), "agent")
	require.NoError(t, err)
	plan, err := second.Plan(context.Background(), prompty.RegistryPlanInput{})
	require.NoError(t, err)
	execution, err := plan.Execute(context.Background())
	// Assert: no checkout read can replace the committed source bundle.
	require.NoError(t, err)
	require.Equal(t, first.Descriptor(), second.Descriptor())
	require.Equal(t, "OLD", execution.Messages[0].Content[0].(prompty.TextPart).Text)
	_, err = fetcher.BindRevision(context.Background(), "main")
	require.ErrorIs(t, err, remoteregistry.ErrFetchFailed)
	_, err = fetcher.BindRevision(context.Background(), strings.Repeat("0", 40))
	require.ErrorIs(t, err, remoteregistry.ErrFetchFailed)
}
