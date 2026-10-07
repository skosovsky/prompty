"""Exercise inventory disagreement in disposable repositories, without publication."""
from pathlib import Path
import subprocess
import tempfile
import unittest

import workspace


class WorkspaceInventoryTests(unittest.TestCase):
    def fixture(self, use, tracked):
        directory = tempfile.TemporaryDirectory(prefix='prompty-workspace-')
        self.addCleanup(directory.cleanup)
        root = Path(directory.name)
        (root / 'go.work').write_text('go 1.27.1\nuse (\n' + '\n'.join(use) + '\n)\n')
        for module in tracked:
            target = root / module
            target.mkdir(parents=True, exist_ok=True)
            (target / 'go.mod').write_text('module example.invalid/fixture\ngo 1.27.1\n')
        subprocess.run(['git', 'init', '-q', str(root)], check=True)
        subprocess.run(['git', 'add', '.'], cwd=root, check=True)
        previous = workspace.ROOT
        workspace.ROOT = root
        self.addCleanup(setattr, workspace, 'ROOT', previous)

    def test_declared_and_tracked_match(self):
        # Arrange.
        self.fixture(['.', './adapter'], ['.', 'adapter'])
        # Act.
        result = workspace.modules()
        # Assert.
        self.assertEqual(['.', 'adapter'], result)

    def test_missing_declared_module_rejects(self):
        # Arrange.
        self.fixture(['.'], ['.', 'adapter'])
        # Act / Assert.
        with self.assertRaisesRegex(ValueError, 'inventory differs'):
            workspace.modules()

    def test_untracked_workspace_member_rejects(self):
        # Arrange.
        self.fixture(['.', './missing'], ['.'])
        # Act / Assert.
        with self.assertRaisesRegex(ValueError, 'inventory differs'):
            workspace.modules()


if __name__ == '__main__':
    unittest.main()
