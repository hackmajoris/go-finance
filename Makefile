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

lint:
	@which golangci-lint > /dev/null 2>&1 || go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	golangci-lint run --fix ./...

fmt:
	gofmt -w .
	goimports -w .

clean:
	rm -rf .bin/

generate:
	go generate ./...
