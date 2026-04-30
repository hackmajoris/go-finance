# go-finance

Fetches real-time and historical stock, crypto, and currency prices from Yahoo Finance and returns JSON output.

```json
{"symbol":"AAPL","price":270.17,"currency":"USD"}
```

## Prerequisites

- Go 1.22+
- [golangci-lint](https://golangci-lint.run/)

## CLI usage

```bash
make build

# Current price
make run ARGS="AAPL"
make run ARGS="BTC-USD"
make run ARGS="USD-EUR"

# Monthly price (OHLC + average)
make run ARGS="AAPL --year 2024 --month 3"

# Yearly price (aggregated from quarterly data)
make run ARGS="AAPL --year 2024"
```

### Output examples

**Current price**
```json
{"symbol":"AAPL","price":270.17,"currency":"USD"}
```

**Monthly (`--year 2024 --month 3`)**
```json
{"symbol":"AAPL","year":2024,"month":3,"open":179.55,"high":180.53,"low":168.49,"close":171.48,"avg":175.01}
```

**Yearly (`--year 2024`)**
```json
{"symbol":"AAPL","year":2024,"open":187.15,"high":237.49,"low":164.08,"close":225.91,"avg":203.66}
```

### Flags

| Flag | Description |
|------|-------------|
| `--year YEAR` | Return historical data for the given year. Omit `--month` for yearly aggregate. |
| `--month MONTH` | Month (1–12). Requires `--year`. |

Flags may appear before or after the ticker: `go-finance AAPL --year 2024` and `go-finance --year 2024 AAPL` both work.

## Using the package in another Go app

```bash
go get github.com/hackmajoris/go-finance@v0.1.0
```

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
    client, err := yahoo.New()
    if err != nil {
        log.Fatal(err)
    }

    // Current price
    quote, err := client.GetQuote(context.Background(), "USD-EUR")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(quote.Symbol, quote.Price, quote.Currency)

    // Monthly OHLC
    bar, err := client.GetMonthlyBar(context.Background(), "AAPL", 2024, 3)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(bar.Open, bar.High, bar.Low, bar.Close, bar.Avg)

    // Yearly OHLC (aggregated from 4 quarters)
    yearly, err := client.GetYearlyBar(context.Background(), "AAPL", 2024)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(yearly.Open, yearly.High, yearly.Low, yearly.Close, yearly.Avg)
}
```

`GetQuote` accepts any Yahoo Finance symbol — stocks (`AAPL`), crypto (`BTC-USD`), and currency pairs (`USD-EUR`, `RON-USD`).

`GetMonthlyBar` and `GetYearlyBar` currently support stock and crypto symbols. Forex pairs are not supported by the Yahoo Finance chart endpoint.

## Development

```bash
make build                          # compile binary to .bin/go-finance
make run ARGS="AAPL"                # current price
make run ARGS="AAPL --year 2024"    # yearly
make run ARGS="AAPL --year 2024 --month 3"  # monthly
make test                           # run tests with race detector
make lint                           # run linter
```

## License

MIT
