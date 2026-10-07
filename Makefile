GO      := go
MODULES := $(shell python3 scripts/workspace.py)

.PHONY: inventory lint fix test bench bench-hotpath fuzz cover release-patch release-break release-check generate gen-check

lint: inventory
	@for dir in $(MODULES); do \
		echo "golangci-lint - $$dir"; \
		(cd "$$dir" && golangci-lint run --allow-serial-runners ./...) || exit 1; \
	done

	@echo "golangci-lint - ./cmd/prompty-gen/testdata/gen_out"
	@golangci-lint run --allow-serial-runners ./cmd/prompty-gen/testdata/gen_out

fix: inventory
	@if [ -f "go.work" ]; then $(GO) work sync; fi
	@for dir in $(MODULES); do \
		echo "fix & tidy - $$dir"; \
		(cd "$$dir" && $(GO) fix ./... && $(GO) mod tidy) || exit 1; \
		(cd "$$dir" && golangci-lint run --fix ./...) || exit 1; \
	done

test: inventory
	@for dir in $(MODULES); do \
		echo "test - $$dir"; \
		(cd "$$dir" && $(GO) test -v -race ./...) || exit 1; \
	done
	@echo "test - ./cmd/prompty-gen/testdata/gen_out"
	@$(GO) test -v -race ./cmd/prompty-gen/testdata/gen_out

bench: inventory
	@for dir in $(MODULES); do \
		echo "bench - $$dir"; \
		(cd "$$dir" && $(GO) test -bench=. -run=^$$ ./...) || exit 1; \
	done

fuzz: inventory
	@for dir in $(MODULES); do \
		echo "fuzz - $$dir"; \
		(cd "$$dir" && \
			for pkg in $$($(GO) list -tags=fuzz ./...); do \
				if $(GO) test -tags=fuzz -list . "$$pkg" 2>/dev/null | grep -q '^Fuzz'; then \
					$(GO) test -tags=fuzz -fuzz=. -fuzztime=30s "$$pkg" || exit 1; \
				fi; \
			done \
		) || exit 1; \
	done

cover: inventory
	@for dir in $(MODULES); do \
		echo "cover - $$dir"; \
		(cd "$$dir" && $(GO) test -coverprofile=coverage.out ./... && $(GO) tool cover -func=coverage.out) || exit 1; \
	done

release-check:
	@PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s scripts -p 'test_*.py'

release-patch: lint test release-check ## v0.5.0 -> v0.5.1
	@./scripts/release.sh patch "$(MODULES)"

release-break: lint test release-check ## v0.5.1 -> v0.6.0
	@./scripts/release.sh break "$(MODULES)"

inventory:
	@python3 scripts/workspace.py > /dev/null

generate: inventory
	@$(GO) run ./cmd/prompty-gen -config cmd/prompty-gen/testdata/prompty.yaml generate

gen-check: generate
	@git diff HEAD --exit-code -- cmd/prompty-gen/testdata/gen_out
	@test -z "$$(git ls-files --others --exclude-standard -- cmd/prompty-gen/testdata/gen_out)"
	@$(GO) test -race ./cmd/prompty-gen/testdata/gen_out
