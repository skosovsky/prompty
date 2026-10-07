"""AAA acceptance on disposable repositories and local bare remotes only."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest


class ReleaseAcceptance(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix='prompty-release-test-')
        self.addCleanup(self.temporary.cleanup)
        self.base = Path(self.temporary.name)
        self.repo = self.base / 'repo'
        self.remote = self.base / 'remote.git'
        self.env = dict(os.environ, GOCACHE=os.environ.get('GOCACHE', '/tmp/prompty-task45-release-go-cache'))
        self.git(self.base, 'init', '--bare', str(self.remote))
        self.git(self.base, 'init', '-b', 'main', str(self.repo))
        self.git(self.repo, 'config', 'user.name', 'fixture')
        self.git(self.repo, 'config', 'user.email', 'fixture@example.invalid')
        self.git(self.repo, 'remote', 'add', 'origin', str(self.remote))
        scripts = Path(__file__).resolve().parent
        self.write('scripts/release.sh', (scripts / 'release.sh').read_text())
        self.write('scripts/release.py', (scripts / 'release.py').read_text())
        self.write('go.mod', 'module example.test/prompty\n\ngo 1.26.3\n')
        self.write('core.go', 'package prompty\nconst Value = 1\n')
        self.write('sdk/go.mod', 'module example.test/prompty/sdk\n\ngo 1.26.3\nrequire example.test/prompty v0.0.0\nreplace example.test/prompty => ..\n')
        self.write('sdk/sdk.go', 'package sdk\nimport "example.test/prompty"\nconst Value = prompty.Value\n')
        self.write('go.work', 'go 1.26.3\nuse (\n .\n ./sdk\n)\n')
        self.git(self.repo, 'add', '.')
        self.git(self.repo, 'commit', '-m', 'fixture')
        self.source = self.git(self.repo, 'rev-parse', 'HEAD').stdout.strip()
        self.write('private-untracked.txt', 'do not publish')
        self.git(self.repo, 'tag', '-a', 'unrelated', '-m', 'unrelated')
        self.env.update(GIT_CONFIG_COUNT='1', GIT_CONFIG_KEY_0='push.followTags', GIT_CONFIG_VALUE_0='true')

    def git(self, cwd, *args):
        result = subprocess.run(['git', '-c', 'commit.gpgsign=false', '-c', 'tag.gpgsign=false', *args], cwd=cwd, env=self.env, text=True, capture_output=True, check=False)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def write(self, filename, text):
        path = self.repo / filename
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def release(self, action='patch', modules='. ./sdk'):
        return subprocess.run(['bash', 'scripts/release.sh', action, modules, '--yes'],
                              cwd=self.repo, env=self.env, text=True, capture_output=True, check=False)

    def record(self):
        return json.loads((self.repo / '.git/prompty-release/transaction.json').read_text())

    def assert_checkout_preserved(self, branch='main'):
        self.assertEqual(self.git(self.repo, 'rev-parse', 'HEAD').stdout.strip(), self.source)
        self.assertEqual(self.git(self.repo, 'branch', '--show-current').stdout.strip(), branch)
        self.assertEqual(self.git(self.repo, 'diff', '--name-only').stdout, '')
        self.assertEqual(self.git(self.repo, 'diff', '--cached', '--name-only').stdout, '')
        self.assertEqual((self.repo / 'private-untracked.txt').read_text(), 'do not publish')
        self.assertEqual(self.git(self.repo, 'tag', '--list').stdout.strip(), 'unrelated')

    def reject_publication(self):
        hook = self.remote / 'hooks/pre-receive'
        hook.write_text('#!/bin/sh\nexit 1\n')
        hook.chmod(0o755)
        return hook

    def test_isolated_success_explicit_refs_and_published_graph(self):
        # Arrange: untracked secret/unrelated tag plus dependent module.
        # Act: actual release entrypoint; no production remote exists in this fixture.
        result = self.release()
        # Assert: only declared refs and committed source files published.
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assert_checkout_preserved()
        record = self.record()
        self.assertEqual(record['status'], 'published')
        refs = self.git(self.repo, 'ls-remote', '--tags', str(self.remote)).stdout.splitlines()
        self.assertEqual({line.split()[1] for line in refs}, {'refs/tags/v0.0.1', 'refs/tags/sdk/v0.0.1'})
        self.assertTrue(all(line.split()[0] == record['commit'] for line in refs))
        files = self.git(self.remote, 'ls-tree', '-r', '--name-only', 'v0.0.1').stdout.splitlines()
        self.assertNotIn('private-untracked.txt', files)
        module = self.git(self.remote, 'show', 'v0.0.1:sdk/go.mod').stdout
        self.assertIn('example.test/prompty v0.0.1', module)
        self.assertNotIn('replace', module)
        self.assertTrue((self.repo / '.git/prompty-release/proxy/example.test/prompty/@v/v0.0.1.zip').exists())

    def test_failed_push_preserves_detached_checkout_and_resumes_same_version(self):
        # Arrange rejected remote and detached starting state.
        hook = self.reject_publication()
        self.git(self.repo, 'checkout', '--detach', self.source)
        # Act.
        result = self.release()
        # Assert unpublished record and no checkout/tag damage.
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.record()['status'], 'unpublished', result.stderr)
        self.assert_checkout_preserved(branch='')
        second = self.release()
        self.assertNotEqual(second.returncode, 0)
        self.assertIn('pending release', second.stderr)
        hook.unlink()
        resumed = self.release('resume')
        self.assertEqual(resumed.returncode, 0, resumed.stderr)
        self.assertEqual(self.record()['version'], 'v0.0.1')
        self.assertEqual(self.record()['status'], 'published')
        self.assert_checkout_preserved(branch='')

    def test_partial_conflict_blocks_recovery_and_fresh_bump(self):
        # Arrange a prepared transaction whose atomic publication was rejected.
        hook = self.reject_publication()
        self.assertNotEqual(self.release().returncode, 0)
        hook.unlink()
        record = self.record()
        stage = self.repo / '.git/prompty-release/stage'
        self.git(stage, 'push', str(self.remote), record['commit'] + ':refs/tags/v0.0.1')
        # Act: remote has a partial expected ref set.
        result = self.release('resume')
        # Assert no second mutation or version bump; published ref retained.
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('partial/conflicting', result.stderr)
        self.assertNotEqual(self.release().returncode, 0)
        self.assertNotEqual(self.release('abort').returncode, 0)
        refs = self.git(self.repo, 'ls-remote', '--tags', str(self.remote)).stdout
        self.assertNotIn('sdk/v0.0.1', refs)
        self.assert_checkout_preserved()

    def test_lost_push_ack_reconciles_published_refs(self):
        # Arrange git wrapper simulating lost acknowledgment after a successful push.
        real_git = shutil.which('git')
        wrappers = self.base / 'bin'
        wrappers.mkdir()
        wrapper = wrappers / 'git'
        wrapper.write_text('#!/bin/sh\n' + f'"{real_git}" "$@"\nstatus=$?\n' +
                           'if [ "$1" = push ] && [ "$status" = 0 ]; then echo lost-ack >&2; exit 1; fi\nexit "$status"\n')
        wrapper.chmod(0o755)
        self.env['PATH'] = str(wrappers) + os.pathsep + self.env['PATH']
        # Act.
        result = self.release()
        # Assert remote evidence overrides an uncertain client acknowledgment.
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.record()['status'], 'published')
        self.assert_checkout_preserved()

    def test_relative_remote_is_bound_to_source_directory(self):
        # Arrange a valid source-relative local remote.
        self.git(self.repo, 'remote', 'set-url', 'origin', '../remote.git')
        # Act: publication executes from the isolated stage.
        result = self.release()
        # Assert normalized binding remains valid across preparation/resume.
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.record()['target'], str(self.remote.resolve()))
        self.assert_checkout_preserved()

    def test_relative_remote_with_colon_is_a_local_path(self):
        # Arrange Git's local-path syntax: slash precedes the colon.
        destination = self.base / 'remote:repo.git'
        self.remote.rename(destination)
        self.remote = destination
        self.git(self.repo, 'remote', 'set-url', 'origin', '../remote:repo.git')
        # Act.
        result = self.release()
        # Assert cwd changes cannot reinterpret this as scp syntax.
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.record()['target'], str(destination.resolve()))
        self.assert_checkout_preserved()

    def test_remote_without_atomic_push_is_not_used_nonatomically(self):
        # Arrange a remote which cannot guarantee the declared ref transaction.
        self.git(self.remote, 'config', 'receive.advertiseAtomic', 'false')
        # Act.
        result = self.release()
        # Assert no fallback publication, preserved source and resumable version.
        self.assertNotEqual(result.returncode, 0)
        self.assertEqual(self.record()['status'], 'unpublished', result.stderr)
        self.assertEqual(self.git(self.repo, 'ls-remote', '--tags', str(self.remote)).stdout, '')
        self.assert_checkout_preserved()
        self.git(self.remote, 'config', 'receive.advertiseAtomic', 'true')
        resumed = self.release('resume')
        self.assertEqual(resumed.returncode, 0, resumed.stderr)
        self.assertEqual(self.record()['version'], 'v0.0.1')

    def test_inventory_and_major_boundary_fail_closed(self):
        # Arrange incomplete module inventory.
        # Act / assert no publication, source remains unchanged.
        result = self.release(modules='.')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('inventory mismatch', result.stderr)
        self.assert_checkout_preserved()
        self.assertEqual(self.release('abort').returncode, 0)
        self.git(self.repo, 'push', str(self.remote), self.source + ':refs/tags/v1.0.0')
        result = self.release('break')
        self.assertNotEqual(result.returncode, 0)
        self.assertIn('semantic import-path migration', result.stderr)
        self.assert_checkout_preserved()


if __name__ == '__main__':
    unittest.main()
