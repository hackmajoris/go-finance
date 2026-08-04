.PHONY: test lint fmt generate release

test:
	go test -race -cover ./...

GOLANGCI_LINT_VERSION := v2.11.4
lint:
	@golangci-lint --version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)" || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	golangci-lint run --fix ./...

fmt:
	@which goimports > /dev/null 2>&1 || go install golang.org/x/tools/cmd/goimports@latest
	gofmt -w .
	goimports -w .

generate:
	go generate ./...

# Usage: make release VERSION=v0.1.4
release: test
	@[ -n "$(VERSION)" ] || { echo "Usage: make release VERSION=vX.Y.Z"; exit 1; }
	@echo "Releasing $(VERSION)..."
	git tag $(VERSION)
	git push origin $(VERSION)
	@echo "Done. Module available at: github.com/hackmajoris/go-finance@$(VERSION)"
