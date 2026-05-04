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

# Yearly price for a currency pair
make run ARGS="USD-RON --year 2015"
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

**Yearly — currency pair (`USD-RON --year 2015`)**
```json
{"symbol":"USD-RON","year":2015,"open":3.7029,"high":4.1851,"low":3.7029,"close":4.0333,"avg":3.9061}
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

    ctx := context.Background()

    // Current price — single symbol
    quote, err := client.GetQuote(ctx, "USD-EUR")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(quote.Symbol, quote.Price, quote.Currency)

    // Current prices — multiple symbols in parallel (returns map[symbol]price)
    prices, err := client.FetchQuotes(ctx, []string{"AAPL", "BTC-USD", "BRK B"})
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(prices["AAPL"], prices["BTC-USD"], prices["BRK-B"])

    // FX rates relative to a base currency — fetched in parallel
    rates, err := client.FetchFXRates(ctx, []string{"EUR", "RON", "USD"}, "USD")
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(rates["EUR"], rates["RON"]) // rates["USD"] == 1.0

    // Normalize broker tickers to Yahoo Finance format ("BRK B" → "BRK-B")
    fmt.Println(yahoo.NormalizeTicker("BRK B"))

    // Monthly OHLC
    bar, err := client.GetMonthlyBar(ctx, "AAPL", 2024, 3)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(bar.Open, bar.High, bar.Low, bar.Close, bar.Avg)

    // Yearly OHLC (aggregated from 4 quarters)
    yearly, err := client.GetYearlyBar(ctx, "AAPL", 2024)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(yearly.Open, yearly.High, yearly.Low, yearly.Close, yearly.Avg)

    // Yearly OHLC for a currency pair
    forexYearly, err := client.GetYearlyBar(ctx, "USD-RON", 2015)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(forexYearly.Open, forexYearly.High, forexYearly.Low, forexYearly.Close, forexYearly.Avg)
}
```

`GetQuote` accepts any Yahoo Finance symbol — stocks (`AAPL`), crypto (`BTC-USD`), and currency pairs (`USD-EUR`, `RON-USD`).

`FetchQuotes` fetches multiple symbols in parallel via the v8 chart endpoint (no crumb required) and returns a `map[string]float64`. Both the original ticker and its normalized form (e.g. `"BRK B"` and `"BRK-B"`) are stored in the map.

`FetchFXRates` fetches spot rates for a list of currencies relative to a base currency in parallel. The base currency always gets rate `1.0`.

`NormalizeTicker` converts broker-style tickers to Yahoo Finance format (spaces → hyphens).

`GetMonthlyBar` and `GetYearlyBar` accept any Yahoo Finance symbol — stocks (`AAPL`), crypto (`BTC-USD`), and currency pairs (`USD-RON`).

## Development(examples)

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
