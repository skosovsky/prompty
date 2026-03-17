.PHONY: test test-all lint lint-all bench bench-all cover cover-all examples-build fix-all

SUBMODULES := remoteregistry/git adapter/openai adapter/anthropic adapter/gemini adapter/ollama \
	examples/basic_chat examples/funcmap_tools examples/git_prompts examples/secure_prompt

fix-all:
	@echo "=== root: go mod tidy ==="
	@go mod tidy
	@echo "=== root: go work sync ==="
	@go work sync
	@echo "=== root: go fix ./... ==="
	@go fix ./...
	@echo "=== root: golangci-lint run --fix ./... ==="
	@golangci-lint run --fix ./...
	@for dir in $(SUBMODULES); do echo "=== fix $$dir ===" && (cd $$dir && go mod tidy && go fix ./... && golangci-lint run --fix ./...) || exit 1; done

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
