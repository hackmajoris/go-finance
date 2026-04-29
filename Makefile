.PHONY: build run test lint fmt clean generate

APP := go-finance
BIN := .bin/$(APP)

build:
	go build -ldflags="-s -w" -o $(BIN) ./cmd/$(APP)

run: build
	$(BIN) $(filter-out $@,$(MAKECMDGOALS))

%:
	@:

test:
	go test -race -cover ./...

GOLANGCI_LINT_VERSION := v2.11.4
lint:
	@golangci-lint --version 2>/dev/null | grep -q "$(GOLANGCI_LINT_VERSION)" || \
		curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- -b $(shell go env GOPATH)/bin $(GOLANGCI_LINT_VERSION)
	golangci-lint run --fix ./...

fmt:
	gofmt -w .
	goimports -w .

clean:
	rm -rf .bin/

generate:
	go generate ./...
