.DEFAULT_GOAL := help

BIN      := bin/logpretty
PKG      :=
FUZZ     :=
FUZZTIME ?= 30s

.PHONY: help build test lint fuzz bench vuln check snapshot demo

help: ## Show available targets
	@awk -F':.*## ' '/^[a-z]+:.*## /{printf "  %-9s %s\n", $$1, $$2}' $(MAKEFILE_LIST)

build: ## Build bin/logpretty
	go build -o $(BIN) ./cmd/logpretty

# Under the race detector sync.Pool drops items, so the allocation checks that go through json.Valid are skipped: hence the second run.
test: ## Run tests with the race detector and coverage, then without it
	go test -race -cover ./...
	go test ./...

lint: ## Run golangci-lint
	golangci-lint run

fuzz: ## Fuzz one target: make fuzz FUZZ=FuzzName PKG=./path [FUZZTIME=30s]
	@{ test -n "$(FUZZ)" && test -n "$(PKG)"; } || { echo "usage: make fuzz FUZZ=FuzzName PKG=./path [FUZZTIME=30s]" >&2; exit 2; }
	go test -run='^$$' -fuzz='^$(FUZZ)$$' -fuzztime=$(FUZZTIME) $(PKG)

bench: ## Run benchmarks
	go test -run='^$$' -bench=. -benchmem ./...

vuln: ## Check dependencies and stdlib for known vulnerabilities
	go tool govulncheck ./...

check: test lint ## Run tests and linter (the pre-commit checks)

snapshot: ## Build the release archives into dist/ with GoReleaser, publishing nothing
	goreleaser release --snapshot --clean

demo: build ## Record assets/demo.gif with vhs
	PATH="$(CURDIR)/bin:$$PATH" vhs assets/demo.tape
