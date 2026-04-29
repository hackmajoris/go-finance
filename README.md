# go-finance

Fetches real-time stock, crypto, and currency prices from Yahoo Finance and returns JSON output.

```json
{"symbol":"BTC-USD","price":94234.56,"currency":"USD"}
```

## Prerequisites

- Go 1.22+
- [golangci-lint](https://golangci-lint.run/)

## CLI usage

```bash
make build
make run BTC-USD
make run USD-EUR
make run AAPL
```

## Using the package in another Go app

```bash
go get github.com/hackmajoris/go-finance@v0.1.0
```

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"

    "github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
    client, err := yahoo.New()
    if err != nil {
        log.Fatal(err)
    }

    quote, err := client.GetQuote(context.Background(), "USD-EUR")
    if err != nil {
        log.Fatal(err)
    }

    json.NewEncoder(fmt.Stdout).Encode(quote)
    // {"symbol":"USD-EUR","price":0.85,"currency":"EUR"}
}
```

`GetQuote` accepts any Yahoo Finance symbol — stocks (`AAPL`), crypto (`BTC-USD`), and currency pairs (`USD-EUR`, `RON-USD`).

## Development

```bash
make build      # compile binary to .bin/go-finance
make run AAPL   # build and run with a ticker
make test       # run tests with race detector
make lint       # run linter
```

## License

MIT