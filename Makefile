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
	golangci-lint run ./...

fmt:
	gofmt -w .
	goimports -w .

clean:
	rm -rf .bin/

generate:
	go generate ./...
