#!/usr/bin/env python3
"""Isolated v0 release transaction. No user checkout or tag mutation."""
import argparse
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import zipfile


class ReleaseError(RuntimeError):
    pass


def command(cwd, *args, env=None):
    result = subprocess.run(args, cwd=cwd, env=env, text=True, capture_output=True, check=False)
    if result.returncode:
        raise ReleaseError(f"{args[0]} {' '.join(args[1:3])}: {result.stderr.strip()}")
    return result.stdout.strip()


def save_record(path, record):
    temporary = path.with_suffix('.tmp')
    temporary.write_text(json.dumps(record, sort_keys=True, indent=2) + '\n')
    temporary.replace(path)


def remote_refs(root, target):
    output = command(root, 'git', 'ls-remote', '--tags', target)
    return dict((ref, digest) for digest, ref in (line.split() for line in output.splitlines()))


def reconcile(root, record):
    refs = remote_refs(root, record['target'])
    observed = {ref: refs.get(ref) for ref in record['refs']}
    if record.get('commit') and all(value == record['commit'] for value in observed.values()):
        return 'published'
    if all(value is None for value in observed.values()):
        return 'unpublished'
    return 'conflicting'


def module_json(stage, directory):
    return json.loads(command(stage / directory, 'go', 'mod', 'edit', '-json'))


def module_inventory(stage, selected):
    tracked = set(command(stage, 'git', 'ls-files').splitlines())
    declared = {str(Path(name).parent) for name in tracked if Path(name).name == 'go.mod'}
    directories = {str(Path(name)) for name in selected.split()}
    if directories != declared or '.' not in directories:
        raise ReleaseError(f'module inventory mismatch: selected={sorted(directories)}, tracked={sorted(declared)}')
    if (stage / 'go.work').exists():
        work = json.loads(command(stage, 'go', 'work', 'edit', '-json'))
        workspace = {str(Path(item['DiskPath'])) for item in work['Use']}
        if workspace != directories:
            raise ReleaseError('go.work and release module inventory differ')
    modules = {name: module_json(stage, name)['Module']['Path'] for name in sorted(directories)}
    root_path = modules['.']
    for directory, path in modules.items():
        expected = root_path if directory == '.' else root_path + '/' + directory
        if path != expected:
            raise ReleaseError(f'module path {path} does not match tag directory {directory}')
    return modules


def escape_module(path):
    return ''.join('!' + char.lower() if char.isupper() else char for char in path)


def build_proxy(stage, modules, version, proxy):
    tracked = command(stage, 'git', 'ls-files').splitlines()
    for directory, module in modules.items():
        destination = proxy / escape_module(module) / '@v'
        destination.mkdir(parents=True, exist_ok=True)
        (destination / (version + '.mod')).write_bytes((stage / directory / 'go.mod').read_bytes())
        (destination / (version + '.info')).write_text(json.dumps({'Version': version, 'Time': '2000-01-01T00:00:00Z'}))
        (destination / 'list').write_text(version + '\n')
        prefix = '' if directory == '.' else directory + '/'
        included = []
        for name in tracked:
            if not name.startswith(prefix):
                continue
            relative = name[len(prefix):]
            if any(other != directory and other != '.' and name.startswith(other + '/') for other in modules):
                continue
            path = stage / name
            if path.is_symlink() or not path.is_file() or '/vendor/' in '/' + relative:
                continue
            included.append((relative, path))
        if directory != '.' and not any(name == 'LICENSE' for name, _ in included) and (stage / 'LICENSE').exists():
            included.append(('LICENSE', stage / 'LICENSE'))
        with zipfile.ZipFile(destination / (version + '.zip'), 'w', zipfile.ZIP_DEFLATED) as archive:
            for relative, path in included:
                archive.write(path, module + '@' + version + '/' + relative)


def decode_json_stream(source):
    decoder = json.JSONDecoder()
    values = []
    remaining = source.lstrip()
    while remaining:
        value, end = decoder.raw_decode(remaining)
        values.append(value)
        remaining = remaining[end:].lstrip()
    return values


def validate_graph(stage, modules, version, state):
    proxy = state / 'proxy'
    shutil.rmtree(proxy, ignore_errors=True)
    build_proxy(stage, modules, version, proxy)
    validation = state / 'validation'
    shutil.rmtree(validation, ignore_errors=True)
    shutil.copytree(stage, validation, ignore=shutil.ignore_patterns('.git'))
    env = dict(os.environ, GOWORK='off', GONOPROXY='none', GONOSUMDB=modules['.'] + ',' + modules['.'] + '/*',
               GOPROXY=proxy.as_uri() + ',' + os.environ.get('GOPROXY', 'https://proxy.golang.org,direct'),
               GOMODCACHE=str(state / 'modcache'), GOPATH=str(state / 'gopath'))
    for directory in modules:
        path = validation / directory
        original = (path / 'go.mod').read_bytes()
        graph = decode_json_stream(command(path, 'go', 'list', '-mod=mod', '-m', '-json', 'all', env=env))
        for dependency in graph:
            if dependency['Path'] in modules.values() and not dependency.get('Main'):
                if dependency.get('Version') != version or dependency.get('Replace'):
                    raise ReleaseError('published graph has a foreign internal dependency')
        command(path, 'go', 'build', '-mod=mod', './...', env=env)
        if (path / 'go.mod').read_bytes() != original:
            raise ReleaseError(f'published module {directory} requires go.mod changes during standalone validation')


def prepare(root, state, record, selected):
    stage = state / 'stage'
    shutil.rmtree(stage, ignore_errors=True)
    command(root, 'git', 'clone', '--no-hardlinks', '--no-checkout', str(root), str(stage))
    command(stage, 'git', 'checkout', '--detach', record['source'])
    modules = module_inventory(stage, selected)
    record['modules'] = modules
    record['refs'] = ['refs/tags/' + ('' if directory == '.' else directory + '/') + record['version'] for directory in modules]
    save_record(state / 'transaction.json', record)
    internal = set(modules.values())
    for directory in modules:
        metadata = module_json(stage, directory)
        edits = []
        for dependency in metadata.get('Require') or []:
            if dependency['Path'] in internal:
                edits.append('-require=' + dependency['Path'] + '@' + record['version'])
            elif dependency['Path'].startswith(modules['.'] + '/'):
                raise ReleaseError('internal dependency outside declared inventory')
        for replacement in metadata.get('Replace') or []:
            if replacement['Old']['Path'] in internal:
                old = replacement['Old']
                edits.append('-dropreplace=' + old['Path'] + ('@' + old['Version'] if old.get('Version') else ''))
            elif replacement['New'].get('Version', '') == '':
                raise ReleaseError('publishable module cannot retain local filesystem replacement')
        command(stage / directory, 'go', 'mod', 'edit', *edits, '-fmt')
    changes = set(command(stage, 'git', 'diff', '--name-only').splitlines())
    allowed = {str(Path(directory) / 'go.mod') for directory in modules}
    if not changes <= allowed:
        raise ReleaseError('unexpected release diff outside inventoried manifests')
    validate_graph(stage, modules, record['version'], state)
    command(stage, 'git', 'add', '--', *sorted(allowed))
    if command(stage, 'git', 'diff', '--cached', '--name-only'):
        command(stage, 'git', '-c', 'commit.gpgsign=false', '-c', 'user.name=prompty release', '-c', 'user.email=release@localhost',
                'commit', '-m', 'chore: release ' + record['version'])
    record['commit'] = command(stage, 'git', 'rev-parse', 'HEAD')
    record['status'] = 'prepared'
    save_record(state / 'transaction.json', record)


def publish(root, state, record):
    status = reconcile(root, record)
    if status == 'published':
        record['status'] = status
        save_record(state / 'transaction.json', record)
        print('Release ' + record['version'] + ' already published; refs verified')
        return
    if status != 'unpublished':
        raise ReleaseError('partial/conflicting remote refs: explicit reconciliation required; no new release')
    if not record.get('commit'):
        raise ReleaseError('preparation incomplete; use abort after remote reconciliation')
    record['status'] = 'publishing'
    save_record(state / 'transaction.json', record)
    refspecs = [record['commit'] + ':' + ref for ref in record['refs']]
    push_error = None
    try:
        command(state / 'stage', 'git', 'push', '--atomic', '--no-follow-tags', record['target'], *refspecs)
    except ReleaseError as error:
        push_error = error
    try:
        status = reconcile(root, record)
    except ReleaseError:
        record['status'] = 'unknown'
        save_record(state / 'transaction.json', record)
        raise ReleaseError('publication outcome unknown; use reconcile/resume at the same version') from push_error
    record['status'] = status
    save_record(state / 'transaction.json', record)
    if status != 'published':
        raise ReleaseError(f'publication {status}; use resume at version {record["version"]}: {push_error}')
    print('Release ' + record['version'] + ' published; exact refs verified')


def run(action, selected, yes=False):
    root = Path(command(Path.cwd(), 'git', 'rev-parse', '--show-toplevel')).resolve()
    if Path.cwd().resolve() != root:
        raise ReleaseError('run release from repository root')
    state = Path(command(root, 'git', 'rev-parse', '--absolute-git-dir')) / 'prompty-release'
    state.mkdir(exist_ok=True)
    lock = state / 'lock'
    try:
        lock.mkdir()
    except FileExistsError as error:
        raise ReleaseError('release already running; inspect owner before removing stale lock') from error
    try:
        record_path = state / 'transaction.json'
        record = json.loads(record_path.read_text()) if record_path.exists() else None
        if action in ('resume', 'reconcile', 'abort'):
            if not record:
                raise ReleaseError('no release transaction')
            status = reconcile(root, record)
            if action == 'reconcile':
                record['status'] = status
                save_record(record_path, record)
                print(json.dumps(record, indent=2))
                return
            if action == 'abort':
                if status != 'unpublished':
                    raise ReleaseError('cannot abort potentially published refs')
                record['status'] = 'aborted'
                save_record(record_path, record)
                return
            publish(root, state, record)
            return
        if record and record['status'] not in ('published', 'aborted'):
            raise ReleaseError('pending release transaction; use reconcile/resume/abort, not a fresh version')
        command(root, 'git', 'diff', '--quiet')
        command(root, 'git', 'diff', '--cached', '--quiet')
        target = command(root, 'git', 'remote', 'get-url', '--push', 'origin')
        colon = target.find(':')
        slash = target.find('/')
        # Git's scp-like syntax requires the colon before any path separator.
        if colon < 0 or (slash >= 0 and slash < colon):
            target_path = Path(target).expanduser()
            target = str((root / target_path).resolve())
        refs = remote_refs(root, target)
        versions = [tuple(map(int, match.groups())) for ref in refs
                    if (match := re.fullmatch(r'refs/tags/v(\d+)\.(\d+)\.(\d+)', ref))]
        major, minor, patch = max(versions, default=(0, 0, 0))
        if major != 0:
            raise ReleaseError('v1+ requires explicit semantic import-path migration; only v0 supported')
        version = f'v0.{minor + 1}.0' if action == 'break' else f'v0.{minor}.{patch + 1}'
        if not yes and input(f'Publish {version}? [y/N] ').lower() != 'y':
            raise ReleaseError('aborted by user')
        if record:
            save_record(state / (record['version'] + '.json'), record)
        record = {'source': command(root, 'git', 'rev-parse', 'HEAD'), 'target': target,
                  'version': version, 'refs': ['refs/tags/' + version], 'status': 'preparing', 'commit': None}
        save_record(record_path, record)
        prepare(root, state, record, selected)
        publish(root, state, record)
    finally:
        lock.rmdir()


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['patch', 'break', 'resume', 'reconcile', 'abort'])
    parser.add_argument('modules', nargs='?', default='')
    parser.add_argument('--yes', action='store_true')
    args = parser.parse_args()
    try:
        run(args.action, args.modules, args.yes)
    except (ReleaseError, OSError, ValueError) as error:
        print('release: ' + str(error), file=sys.stderr)
        sys.exit(1)


if __name__ == '__main__':
    main()
