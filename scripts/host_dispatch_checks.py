#!/usr/bin/env python3
"""Run the same optional semantic suite against local or published dependencies."""
import argparse
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile

ROOT = Path(__file__).resolve().parent.parent
DEPENDENCIES = {name: f'github.com/skosovsky/{name}' for name in ('toolsy', 'contexty', 'routery')}


def command(cwd, args, env, evidence):
    print('+ ' + ' '.join(map(str, args)), flush=True)
    result = subprocess.run(args, cwd=cwd, env=env, text=True, stdout=subprocess.PIPE,
                            stderr=subprocess.STDOUT, check=False)
    print(result.stdout, end='', flush=True)
    evidence['commands'].append({'args': list(map(str, args)), 'exit_code': result.returncode,
                                 'output': result.stdout})
    if result.returncode:
        raise RuntimeError(f'{args[0]} failed ({result.returncode})')
    return result.stdout


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('mode', choices=('local', 'published'))
    parser.add_argument('--source', action='append', default=[], metavar='NAME=PATH')
    parser.add_argument('--ref', help='published prompty root/adapter ref; default latest')
    parser.add_argument('--report', type=Path)
    parser.add_argument('--run', action='store_true', help='also run the offline demo')
    args = parser.parse_args()
    evidence = {'mode': args.mode, 'commands': [], 'sources': {}, 'success': False}
    env = dict(os.environ, GOWORK='off')
    try:
        with tempfile.TemporaryDirectory(prefix='prompty-host-dispatch-') as directory:
            stage = Path(directory).resolve()
            example = stage / 'examples/host_dispatch'
            shutil.copytree(ROOT / 'examples/host_dispatch', example)
            if args.mode == 'local':
                if args.ref:
                    raise ValueError('--ref is only valid in published mode')
                sources = {}
                for entry in args.source:
                    name, path = entry.split('=', 1)
                    if name not in DEPENDENCIES or name in sources:
                        raise ValueError(f'unknown or duplicate source {name}')
                    sources[name] = Path(path).resolve()
                if set(sources) != set(DEPENDENCIES):
                    raise ValueError('local mode requires toolsy, contexty and routery --source paths')
                modules = [ROOT, ROOT / 'adapter/openai', example]
                for name, path in sources.items():
                    module = json.loads(command(path, ['go', 'mod', 'edit', '-json'], env, evidence))
                    if module['Module']['Path'] != DEPENDENCIES[name]:
                        raise ValueError(f'source {name} has wrong module path')
                    revision = command(path, ['git', 'rev-parse', 'HEAD'], env, evidence).strip()
                    dirty = command(path, ['git', 'status', '--porcelain'], env, evidence).strip()
                    evidence['sources'][name] = {'path': str(path), 'commit': revision, 'dirty': dirty}
                    modules.append(path)
                workspace = stage / 'go.work'
                workspace.write_text('go 1.27.1\nuse (\n' + ''.join(
                    '\t' + json.dumps(str(path)) + '\n' for path in modules) + ')\n')
                env['GOWORK'] = str(workspace)
            else:
                if args.source:
                    raise ValueError('published mode forbids source overrides')
                ref = args.ref or 'latest'
                for path in ('github.com/skosovsky/prompty', 'github.com/skosovsky/prompty/adapter/openai'):
                    module = json.loads(command(example, ['go', 'list', '-m', '-json', path + '@' + ref], env, evidence))
                    command(example, ['go', 'mod', 'edit', '-require=' + path + '@' + module['Version']], env, evidence)
                command(example, ['go', 'mod', 'tidy'], env, evidence)
                manifest = json.loads(command(example, ['go', 'mod', 'edit', '-json'], env, evidence))
                if manifest.get('Replace'):
                    raise ValueError('published manifest contains replacements')
            # All dependency refs (including actual local replacements) are recorded.
            evidence['modules'] = command(example, ['go', 'list', '-m', '-json', 'all'], env, evidence)
            command(example, ['go', 'test', '-race', '-count=1', './...'], env, evidence)
            command(example, ['golangci-lint', 'run', '--allow-serial-runners',
                              '--config', str(ROOT / '.golangci.yml'), './...'], env, evidence)
            if args.run:
                command(example, ['go', 'run', './cmd/demo'], env, evidence)
            evidence['success'] = True
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as error:
        evidence['error'] = str(error)
        print(f'host dispatch checks: {error}', flush=True)
    finally:
        if args.report:
            args.report.parent.mkdir(parents=True, exist_ok=True)
            args.report.write_text(json.dumps(evidence, indent=2) + '\n')
    return 0 if evidence['success'] else 1


if __name__ == '__main__':
    raise SystemExit(main())
