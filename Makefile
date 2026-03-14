.PHONY: test test-all lint lint-all bench bench-all cover cover-all examples-build

SUBMODULES := remoteregistry/git adapter/openai adapter/anthropic adapter/gemini adapter/ollama

test:
	@go test -race -count=1 ./...

examples-build:
	@echo "=== build example examples/secure_prompt ==="
	@go build -o /tmp/prompty-secure_prompt ./examples/secure_prompt

test-all: test examples-build
	@for dir in $(SUBMODULES); do echo "=== test $$dir ===" && (cd $$dir && go test -race -count=1 ./...) || exit 1; done

lint:
	@golangci-lint run ./...

lint-all: lint
	@for dir in $(SUBMODULES); do echo "=== lint $$dir ===" && (cd $$dir && golangci-lint run ./...) || exit 1; done

bench:
	@go test -bench=. -benchmem ./...

bench-all: bench
	@for dir in $(SUBMODULES); do echo "=== bench $$dir ===" && (cd $$dir && go test -bench=. -benchmem ./...) || exit 1; done

cover:
	@go test -coverprofile=coverage.out -covermode=atomic ./...
	@go tool cover -func=coverage.out

cover-all: cover
	@for dir in $(SUBMODULES); do echo "=== cover $$dir ===" && (cd $$dir && go test -coverprofile=coverage.out -covermode=atomic ./... && go tool cover -func=coverage.out) || exit 1; done
