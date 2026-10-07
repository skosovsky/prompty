#!/usr/bin/env python3
"""Read the declared go.work inventory and verify it against tracked modules."""
import json
from pathlib import Path
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent


def modules():
    work = json.loads(subprocess.check_output(
        ['go', 'work', 'edit', '-json'], cwd=ROOT, text=True))
    declared = [str(Path(item['DiskPath'])) for item in work['Use']]
    tracked = subprocess.check_output(['git', 'ls-files'], cwd=ROOT, text=True).splitlines()
    actual = {str(Path(name).parent) for name in tracked if Path(name).name == 'go.mod'}
    if len(declared) != len(set(declared)) or set(declared) != actual or '.' not in actual:
        raise ValueError(f'go.work inventory differs from tracked modules: declared={declared}, tracked={sorted(actual)}')
    for directory in declared:
        path = Path(directory)
        if path.is_absolute() or '..' in path.parts or not (ROOT / path / 'go.mod').is_file():
            raise ValueError(f'invalid workspace module {directory}')
    return sorted(declared)


if __name__ == '__main__':
    try:
        result = modules()
        print(json.dumps(result) if '--json' in sys.argv[1:] else '\n'.join(result))
    except (ValueError, subprocess.CalledProcessError, KeyError) as error:
        sys.exit(f'workspace inventory: {error}')
