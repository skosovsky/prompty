package git

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/skosovsky/prompty"
	"github.com/skosovsky/prompty/remoteregistry"

	"github.com/stretchr/testify/require"
	"go.uber.org/goleak"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// initRepo creates a git repo in dir with one commit containing the given files (relative path -> content).
func initRepo(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))     // #nosec G301 -- test helper: dir is t.TempDir()
		require.NoError(t, os.WriteFile(full, []byte(content), 0644)) // #nosec G306 -- test helper: manifest content
	}
	for _, c := range []string{"git init", "git branch -M main", "git add .", "git commit -m init"} {
		cmd := exec.Command("sh", "-c", c) // #nosec G204 -- test helper: c is from fixed list
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "run %q: %s", c, out)
	}
}

// initRepoWithBranches creates a repo with main and an optional second branch; branchFiles is content for the second branch.
func initRepoWithBranches(t *testing.T, dir string, mainFiles map[string]string, branchName string, branchFiles map[string]string) {
	t.Helper()
	initRepo(t, dir, mainFiles)
	if branchName == "" || len(branchFiles) == 0 {
		return
	}
	for path, content := range branchFiles {
		full := filepath.Join(dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0755))     // #nosec G301 -- test helper
		require.NoError(t, os.WriteFile(full, []byte(content), 0644)) // #nosec G306 -- test helper
	}
	for _, c := range []string{"git checkout -b " + branchName, "git add .", "git commit -m branch"} {
		cmd := exec.Command("sh", "-c", c) // #nosec G204 -- test helper: c is from fixed list
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=test", "GIT_AUTHOR_EMAIL=test@test", "GIT_COMMITTER_NAME=test", "GIT_COMMITTER_EMAIL=test@test")
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "run %q: %s", c, out)
	}
}

func TestFetcher_Fetch_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"support_agent.yaml": `
id: support_agent
version: "1"
messages:
  - role: system
    content: "Hello {{ .user_name }}"
`,
	})
	fileURL := "file://" + dir
	g, err := NewFetcher(fileURL)
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	data, err := g.Fetch(ctx, "support_agent", "")
	require.NoError(t, err)
	require.Contains(t, string(data), "support_agent")
	require.Contains(t, string(data), "Hello")
}

func TestFetcher_Fetch_EnvSpecific(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"p.yaml": `
id: p
version: "1"
messages:
  - role: system
    content: "Base"
`,
		"p.production.yaml": `
id: p
version: "1"
messages:
  - role: system
    content: "Production"
`,
	})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	data, err := g.Fetch(ctx, "p", "production")
	require.NoError(t, err)
	require.Contains(t, string(data), "Production")
}

func TestFetcher_Fetch_WithDir(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"prompts/agent.yaml": `
id: agent
version: "1"
messages:
  - role: system
    content: "From subdir"
`,
	})
	g, err := NewFetcher("file://"+dir, WithDir("prompts"))
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	data, err := g.Fetch(ctx, "agent", "")
	require.NoError(t, err)
	require.Contains(t, string(data), "From subdir")
}

func TestFetcher_IntegrationWithRegistry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{
		"integ.yaml": `
id: integ
version: "1"
messages:
  - role: system
    content: "Integrated"
`,
	})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	reg := remoteregistry.New(g)
	ctx := context.Background()
	tpl, err := reg.GetTemplate(ctx, "integ", "")
	require.NoError(t, err)
	require.NotNil(t, tpl)
	require.Equal(t, "integ", tpl.Metadata.ID)
}

func TestFetcher_Fetch_WithBranch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepoWithBranches(t, dir,
		map[string]string{"main_only.yaml": "id: main_only\nversion: \"1\"\nmessages:\n  - role: system\n    content: FromMain\n"},
		"dev",
		map[string]string{"dev_only.yaml": "id: dev_only\nversion: \"1\"\nmessages:\n  - role: system\n    content: FromDev\n"},
	)
	g, err := NewFetcher("file://"+dir, WithBranch("dev"))
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	data, err := g.Fetch(ctx, "dev_only", "")
	require.NoError(t, err)
	require.Contains(t, string(data), "FromDev")
}

func TestFetcher_FetchAfterClose(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.yaml": "id: a\nversion: \"1\"\nmessages:\n  - role: system\n    content: x\n"})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	data, err := g.Fetch(context.Background(), "a", "")
	require.NoError(t, err)
	require.Contains(t, string(data), "id: a")
	require.NoError(t, g.Close())
	// After Close, Fetch re-clones and should still succeed.
	data2, err := g.Fetch(context.Background(), "a", "")
	require.NoError(t, err)
	require.Contains(t, string(data2), "id: a")
}

func TestFetcher_Concurrent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"c.yaml": "id: c\nversion: \"1\"\nmessages:\n  - role: system\n    content: concurrent\n"})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	type result struct {
		data []byte
		err  error
	}
	results := make(chan result, 20)
	for range 20 {
		go func() {
			data, err := g.Fetch(ctx, "c", "")
			results <- result{data: data, err: err}
		}()
	}
	for range 20 {
		r := <-results
		require.NoError(t, r.err)
		require.Contains(t, string(r.data), "concurrent")
	}
}

func TestFetcher_Close_Idempotent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.yaml": "id: a\nversion: \"1\"\nmessages:\n  - role: system\n    content: x\n"})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	_, _ = g.Fetch(context.Background(), "a", "")
	require.NoError(t, g.Close())
	require.NoError(t, g.Close())
}

func TestNewFetcher_EmptyURL(t *testing.T) {
	t.Parallel()
	_, err := NewFetcher("")
	require.Error(t, err)
	_, err = NewFetcher("   ")
	require.Error(t, err)
}

func TestFetcher_WithDepth(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"d.yaml": "id: d\nversion: \"1\"\nmessages:\n  - role: system\n    content: depth\n"})
	g, err := NewFetcher("file://"+dir, WithDepth(1))
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	data, err := g.Fetch(context.Background(), "d", "")
	require.NoError(t, err)
	require.Contains(t, string(data), "depth")
}

func TestFetcher_WithAuth_OptionApplied(t *testing.T) {
	t.Parallel()
	// WithAuth does not affect file:// URLs; just ensure Fetcher is created and Fetch works.
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"auth_opt.yaml": "id: auth_opt\nversion: \"1\"\nmessages:\n  - role: system\n    content: ok\n"})
	g, err := NewFetcher("file://"+dir, WithAuth("token"))
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	data, err := g.Fetch(context.Background(), "auth_opt", "")
	require.NoError(t, err)
	require.Contains(t, string(data), "auth_opt")
}

func TestFetcher_Fetch_InvalidNameRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	initRepo(t, dir, map[string]string{"a.yaml": "id: a\nversion: \"1\"\nmessages:\n  - role: system\n    content: x\n"})
	g, err := NewFetcher("file://" + dir)
	require.NoError(t, err)
	defer func() { _ = g.Close() }()
	ctx := context.Background()
	_, err = g.Fetch(ctx, "../../etc/passwd", "")
	require.Error(t, err)
	require.ErrorIs(t, err, prompty.ErrInvalidName)
}
