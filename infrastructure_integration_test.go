//go:build integration

package prompty_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type releaseFixture struct {
	repo   string
	remote string
	source string
	base   string
}

func fixtureCommand(t *testing.T, dir, program string, args ...string) string {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), program, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GOWORK=off", "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "%s %v: %s", program, args, output)
	return strings.TrimSpace(string(output))
}

func fixtureWrite(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func newReleaseFixture(t *testing.T) releaseFixture {
	t.Helper()
	for _, tool := range []string{"git", "go", "make", "bash"} {
		_, err := exec.LookPath(tool)
		require.NoError(t, err, "required prerequisite: %s", tool)
	}
	dir := t.TempDir()
	f := releaseFixture{repo: filepath.Join(dir, "caller"), remote: filepath.Join(dir, "origin.git")}
	require.NoError(t, os.Mkdir(f.repo, 0o755))
	fixtureCommand(t, dir, "git", "init", "--bare", f.remote)
	fixtureCommand(t, f.repo, "git", "init", "-b", "main")
	fixtureCommand(t, f.repo, "git", "config", "user.name", "Release test")
	fixtureCommand(t, f.repo, "git", "config", "user.email", "release@example.invalid")
	fixtureCommand(t, f.repo, "git", "config", "commit.gpgsign", "false")
	script, err := os.ReadFile("scripts/release.sh")
	require.NoError(t, err)
	fixtureWrite(t, filepath.Join(f.repo, "scripts/release.sh"), string(script))
	makefile, err := os.ReadFile("Makefile")
	require.NoError(t, err)
	// Keep production discovery; lightweight source gates isolate the Git protocol.
	lines := strings.Split(string(makefile), "\n")
	fixtureWrite(t, filepath.Join(f.repo, "Makefile"), strings.Join(lines[:2], "\n")+`
modules:
	@printf '%s\n' $(MODULES)
lint test test-integration test-e2e:
	@test -z "$$PROMPTY_RELEASE_FAIL_GATE"
`)
	fixtureWrite(t, filepath.Join(f.repo, "go.mod"), "module example.invalid/library\n\ngo 1.27.2\n")
	fixtureWrite(
		t,
		filepath.Join(f.repo, "extensions/widget/go.mod"),
		"module example.invalid/library/extensions/widget\n\ngo 1.27.2\n\nrequire example.invalid/library v0.0.0\n\nreplace example.invalid/library => ../..\n",
	)
	fixtureWrite(t, filepath.Join(f.repo, ".hidden/go.mod"), "module ignored.invalid/hidden\n")
	fixtureWrite(t, filepath.Join(f.repo, "vendor/ignored/go.mod"), "module ignored.invalid/vendor\n")
	fixtureCommand(t, f.repo, "git", "add", ".")
	fixtureCommand(t, f.repo, "git", "commit", "-m", "base")
	f.base = fixtureCommand(t, f.repo, "git", "rev-parse", "HEAD")
	fixtureCommand(t, f.repo, "git", "remote", "add", "origin", f.remote)
	fixtureCommand(t, f.repo, "git", "push", "origin", "main")
	fixtureWrite(t, filepath.Join(f.repo, "source.txt"), "selected source\n")
	fixtureCommand(t, f.repo, "git", "add", "source.txt")
	fixtureCommand(t, f.repo, "git", "commit", "-m", "source")
	f.source = fixtureCommand(t, f.repo, "git", "rev-parse", "HEAD")
	return f
}

func (f releaseFixture) run(t *testing.T, operation, answer string, extraEnv ...string) (string, error) {
	t.Helper()
	// /bin/bash exercises macOS's supported system Bash 3.2 and Linux's Bash.
	cmd := exec.CommandContext(t.Context(), "/bin/bash", "scripts/release.sh", operation)
	cmd.Dir = f.repo
	cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null")
	cmd.Env = append(cmd.Env, extraEnv...)
	cmd.Stdin = strings.NewReader(answer)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

func (f releaseFixture) record(t *testing.T, field string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(f.repo, ".git/library-releases/active", field))
	require.NoError(t, err)
	return strings.TrimSpace(string(data))
}

func (f releaseFixture) verifyPublication(t *testing.T, candidate string) {
	t.Helper()
	assert.NotEqual(t, f.source, candidate)
	assert.Equal(t, f.source, fixtureCommand(t, f.remote, "git", "rev-parse", "refs/heads/main"))
	for _, tag := range []string{"v0.0.1", "extensions/widget/v0.0.1"} {
		assert.Equal(t, candidate, fixtureCommand(t, f.remote, "git", "rev-parse", "refs/tags/"+tag))
	}
	assert.Contains(
		t,
		fixtureCommand(t, f.remote, "git", "show", "main:extensions/widget/go.mod"),
		"replace example.invalid/library => ../..",
	)
	manifest := fixtureCommand(t, f.remote, "git", "show", "v0.0.1:extensions/widget/go.mod")
	assert.NotContains(t, manifest, "replace")
	assert.Contains(t, manifest, "example.invalid/library v0.0.1")
	assert.Equal(t, f.source, fixtureCommand(t, f.repo, "git", "rev-parse", "HEAD"))
	assert.Equal(t, "main", fixtureCommand(t, f.repo, "git", "branch", "--show-current"))
	assert.Empty(t, fixtureCommand(t, f.repo, "git", "status", "--porcelain", "--untracked-files=no"))
	assert.Empty(t, fixtureCommand(t, f.repo, "git", "tag", "--list"))
	assert.Equal(
		t,
		"extensions/widget/go.mod",
		fixtureCommand(t, f.remote, "git", "diff", "--name-only", f.source, candidate),
	)
}

func TestIntegrationReleaseSourceAndCandidate(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	fixtureWrite(t, filepath.Join(f.repo, "untracked.txt"), "caller notes\n")
	assert.Equal(t, ".\nextensions/widget", fixtureCommand(t, f.repo, "make", "--no-print-directory", "-s", "modules"))
	// Act.
	output, err := f.run(t, "patch", "y\n")
	// Assert.
	require.NoError(t, err, output)
	f.verifyPublication(t, f.record(t, "candidate"))
	assert.Equal(t, "?? untracked.txt", fixtureCommand(t, f.repo, "git", "status", "--porcelain"))
	output, err = f.run(t, "finish", "")
	require.NoError(t, err, output)
	_, err = os.Stat(filepath.Join(f.repo, ".git/library-releases/active"))
	assert.True(t, os.IsNotExist(err))
}

func TestIntegrationModulePaths(t *testing.T) {
	// Arrange.
	modules := strings.Split(fixtureCommand(t, ".", "make", "--no-print-directory", "-s", "modules"), "\n")
	rootPath := fixtureCommand(t, ".", "go", "list", "-m", "-f", "{{.Path}}")
	// Act and assert.
	for _, module := range modules {
		path := fixtureCommand(t, module, "go", "list", "-m", "-f", "{{.Path}}")
		expected := rootPath
		if module != "." {
			expected += "/" + module
		}
		assert.Equal(t, expected, path, "module directory: %s", module)
	}
}

func TestIntegrationReleaseFailedGate(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	// Act.
	output, err := f.run(t, "patch", "y\n", "PROMPTY_RELEASE_FAIL_GATE=1")
	// Assert.
	require.Error(t, err, output)
	assert.Equal(t, f.base, fixtureCommand(t, f.remote, "git", "rev-parse", "main"))
	assert.Empty(t, fixtureCommand(t, f.remote, "git", "tag", "--list"))
	assert.Equal(t, "preparing", f.record(t, "phase"))
	output, err = f.run(t, "resume", "y\n")
	require.NoError(t, err, output)
	f.verifyPublication(t, f.record(t, "candidate"))
}

func TestIntegrationReleaseAtomicRejectionAndRecovery(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	hook := filepath.Join(f.remote, "hooks/update")
	fixtureWrite(t, hook, "#!/bin/sh\ncase \"$1\" in refs/tags/extensions/widget/*) exit 1;; esac\nexit 0\n")
	require.NoError(t, os.Chmod(hook, 0o755))
	// Act.
	output, err := f.run(t, "patch", "y\n")
	// Assert.
	require.Error(t, err, output)
	candidate := f.record(t, "candidate")
	assert.Equal(t, f.base, fixtureCommand(t, f.remote, "git", "rev-parse", "main"))
	assert.Empty(t, fixtureCommand(t, f.remote, "git", "tag", "--list"))
	output, err = f.run(t, "finish", "")
	require.Error(t, err, output)
	require.NoError(t, os.Remove(hook))
	output, err = f.run(t, "inspect", "")
	require.NoError(t, err, output)
	assert.Contains(t, output, "status: none")
	output, err = f.run(t, "resume", "y\n")
	require.NoError(t, err, output)
	assert.Equal(t, candidate, f.record(t, "candidate"))
	assert.Equal(t, "v0.0.1", f.record(t, "version"))
	f.verifyPublication(t, candidate)
	// A lost response is recovered by observation without republishing.
	fixtureWrite(t, filepath.Join(f.repo, ".git/library-releases/active/status"), "unknown\n")
	output, err = f.run(t, "inspect", "")
	require.NoError(t, err, output)
	assert.Contains(t, output, "status: complete")
	output, err = f.run(t, "resume", "")
	require.NoError(t, err, output)
	assert.Equal(t, candidate, f.record(t, "candidate"))
}

func TestIntegrationReleaseTagCollision(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	output, err := f.run(t, "patch", "n\n")
	require.Error(t, err, output)
	candidate := f.record(t, "candidate")
	fixtureCommand(t, f.remote, "git", "update-ref", "refs/tags/v0.0.1", f.base)
	// Act.
	output, err = f.run(t, "inspect", "")
	// Assert.
	require.NoError(t, err, output)
	assert.Contains(t, output, "status: collision")
	output, err = f.run(t, "resume", "y\n")
	require.Error(t, err, output)
	assert.Equal(t, candidate, f.record(t, "candidate"))
	assert.Equal(t, f.base, fixtureCommand(t, f.remote, "git", "rev-parse", "main"))
	assert.Equal(t, f.base, fixtureCommand(t, f.remote, "git", "rev-parse", "v0.0.1"))
}

func TestIntegrationReleaseRejectsWrongModulePath(t *testing.T) {
	// Arrange.
	f := newReleaseFixture(t)
	fixtureWrite(t, filepath.Join(f.repo, "extensions/widget/go.mod"), "module example.invalid/wrong\n\ngo 1.27.2\n")
	fixtureCommand(t, f.repo, "git", "add", ".")
	fixtureCommand(t, f.repo, "git", "commit", "-m", "wrong module")
	// Act.
	output, err := f.run(t, "patch", "y\n")
	// Assert.
	require.Error(t, err)
	assert.Contains(t, output, "unexpected module path")
	assert.Equal(t, f.base, fixtureCommand(t, f.remote, "git", "rev-parse", "main"))
	assert.Empty(t, fixtureCommand(t, f.remote, "git", "tag", "--list"))
}
