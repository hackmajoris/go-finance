.PHONY: help test lint fmt generate summary release

.DEFAULT_GOAL := help

help: ## Show available commands
	@grep -E '^[a-zA-Z_-]+:.*## ' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-10s\033[0m %s\n", $$1, $$2}'

test: ## Run tests with race detector and coverage
	go test -race -cover ./...

GOLANGCI_LINT_VERSION := v2.11.4
lint: ## Run golangci-lint (installs pinned version if missing)
	@golangci-lint --version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)" || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	golangci-lint run --fix ./...

fmt: ## Format code with gofmt and goimports
	@which goimports > /dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	gofmt -w .
	goimports -w .

generate: ## Run go generate
	go generate ./...

# Usage: make summary TICKER=VWCE.DE
TICKER ?= NVDA
summary: ## Print summary table for TICKER (default NVDA), e.g. make summary TICKER=VWCE.DE
	go run ./examples/summary $(TICKER)

# Usage: make release VERSION=v0.1.4
release: test ## Tag and push a release, e.g. make release VERSION=v0.1.4
	@[ -n "$(VERSION)" ] || { echo "Usage: make release VERSION=vX.Y.Z"; exit 1; }
	@echo "Releasing $(VERSION)..."
	git tag $(VERSION)
	git push origin $(VERSION)
	@echo "Done. Module available at: github.com/hackmajoris/go-finance@$(VERSION)"
