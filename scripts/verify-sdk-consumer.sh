#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
sdk_toolchain="$(cd "$root" && go env GOVERSION)"
export GOTOOLCHAIN="$sdk_toolchain"
consumer_dir="$(mktemp -d "${TMPDIR:-/tmp}/prompty-sdk-consumer.XXXXXX")"
trap 'rm -rf "$consumer_dir"' EXIT
cp "$root"/scripts/testdata/sdk_consumer/*.go "$consumer_dir/"
cd "$consumer_dir"
export GOWORK=off
export GOPROXY=off
go mod init example.com/prompty-sdk-consumer
for path in . adapter/ollama adapter/gemini adapter/openai adapter/anthropic; do
 module=github.com/skosovsky/prompty
 if [[ "$path" != . ]]; then module="$module/$path"; fi
 go mod edit "-require=$module@v0.0.0" "-replace=$module=$root/$path"
done
cat "$root/go.sum" "$root"/adapter/{ollama,gemini,openai,anthropic}/go.sum | sort -u > go.sum
go list -mod=mod -m github.com/ollama/ollama google.golang.org/genai github.com/openai/openai-go/v3 github.com/anthropics/anthropic-sdk-go
go test -mod=mod -race -count=1 -v .
go run -mod=mod .
