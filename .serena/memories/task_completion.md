# When Task is Completed

1. Run `go build ./...` — ensure compile
2. Run `go test ./...` or `make test` — ensure tests pass
3. Run `golangci-lint run ./...` or `make lint` — ensure lint passes
4. For prompty-gen changes: `go test ./cmd/prompty-gen/... -count=1`
5. If golden files changed: `go test ./cmd/prompty-gen/gen -run TestGenerate_Golden -args -golden=./cmd/prompty-gen/testdata`