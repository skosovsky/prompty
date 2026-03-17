# Suggested Commands (Darwin)

## Build & Sync
- `go work sync` — sync workspace modules
- `go build ./...` — build all

## Testing
- `go test ./...` — test root
- `go test -race -count=1 ./...` — test with race detector
- `make test` — root tests
- `make test-all` — root + all submodules
- `go test ./cmd/prompty-gen/... -count=1` — prompty-gen tests

## Linting & Formatting
- `golangci-lint run ./...` — lint
- `golangci-lint run --fix ./...` — lint with auto-fix
- `make lint` / `make lint-all`
- `go fix ./...` — apply go fix

## Fix All (tidy + fix + lint)
- `make fix-all`

## Benchmarks
- `go test -bench=. -benchmem ./...`
- `make bench` / `make bench-all`

## Examples
- `go run ./examples/basic_chat`
- `go run ./examples/secure_prompt`

## Utils
- `git`, `cd`, `grep`, `find` — standard Darwin/Unix