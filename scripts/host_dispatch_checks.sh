#!/bin/bash
# Run the offline consumer against explicit source trees or published modules.
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
fail() { echo "host dispatch checks: $*" >&2; exit 1; }
mode=${1:-}; [[ $# == 0 ]] || shift
[[ "$mode" == local || "$mode" == published ]] || fail 'usage: host_dispatch_checks.sh local|published [--source NAME=PATH] [--ref VERSION] [--run] [--report LOG]'
ref=latest; explicit_ref=false; demo=false; report=
toolsy=; contexty=; routery=
while (( $# )); do
  case "$1" in
    --source)
      (( $# >= 2 )) || fail 'missing source'
      name=${2%%=*}; path=${2#*=}
      [[ "$path" != "$2" && -n "$path" ]] || fail 'source must be NAME=PATH'
      case "$name" in toolsy|contexty|routery) ;; *) fail "unknown source: $name";; esac
      [[ -z ${!name} ]] || fail "duplicate source: $name"
      path=$(cd "$path" && pwd)
      printf -v "$name" '%s' "$path"; shift 2;;
    --ref) (( $# >= 2 )) || fail 'missing ref'; ref=$2; explicit_ref=true; shift 2;;
    --report) (( $# >= 2 )) || fail 'missing report'; report=$2; shift 2;;
    --run) demo=true; shift;;
    *) fail "unknown argument: $1";;
  esac
done
if [[ "$mode" == local ]]; then
  [[ "$explicit_ref" == false ]] || fail '--ref requires published mode'
  [[ -n "$toolsy" && -n "$contexty" && -n "$routery" ]] || fail 'local mode requires toolsy, contexty and routery sources'
else
  [[ -z "$toolsy$contexty$routery" ]] || fail 'published mode forbids source overrides'
fi
if [[ -n "$report" ]]; then
  mkdir -p "$(dirname "$report")"
  exec > >(tee "$report") 2>&1
fi
stage=$(mktemp -d "${TMPDIR:-/tmp}/prompty-host-dispatch.XXXXXX")
trap 'rm -rf "$stage"' EXIT
export GOWORK=off
# Keep any toolchain selection consistent when moving outside the repository.
toolchain=$(cd "$root" && go list -m -f '{{.GoVersion}}')
export GOTOOLCHAIN="go${toolchain}+auto"
mkdir -p "$stage/examples"
cp -R "$root/examples/host_dispatch" "$stage/examples/host_dispatch"
example="$stage/examples/host_dispatch"
run() { printf '+'; printf ' %q' "$@"; printf '\n'; "$@"; }
cd "$example"
for path in github.com/skosovsky/prompty github.com/skosovsky/prompty/adapter/openai; do
  run go mod edit "-dropreplace=$path"
done
if [[ "$mode" == local ]]; then
  modules=("$root" "$root/adapter/openai" "$example")
  for name in toolsy contexty routery; do
    path=${!name}
    actual=$(cd "$path" && go list -m -f '{{.Path}}')
    [[ "$actual" == "github.com/skosovsky/$name" ]] || fail "wrong module path for $name: $actual"
    printf '\nSource %s: %s\n' "$name" "$path"
    git -C "$path" rev-parse HEAD
    git -C "$path" status --porcelain
    modules+=("$path")
  done
  cd "$stage"
  run go work init "${modules[@]}"
  export GOWORK="$stage/go.work"
  cat "$GOWORK"
else
  cd "$example"
  for path in github.com/skosovsky/prompty github.com/skosovsky/prompty/adapter/openai; do
    version=$(go list -m -f '{{.Version}}' "$path@$ref")
    run go mod edit "-require=$path@$version"
  done
  run go mod tidy
fi
cd "$example"
run go list -m -json all
if [[ "$mode" == published ]]; then
  replacements=$(go list -m -f '{{if .Replace}}{{.Path}}{{end}}' all)
  [[ -z "$replacements" ]] || fail "published graph contains replacements: $replacements"
fi
run go test -race -count=1 ./...
run golangci-lint run --allow-serial-runners --config "$root/.golangci.yml" ./...
if [[ "$demo" == true ]]; then run go run ./cmd/demo; fi
printf 'host dispatch %s: PASS\n' "$mode"
