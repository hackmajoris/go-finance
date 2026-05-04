# go-finance

Fetches real-time and historical stock, crypto, and currency prices from Yahoo Finance.

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
go get github.com/hackmajoris/go-finance@v0.1.3
```

### Quick start

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

## API Reference

### Creating a client

```go
client, err := yahoo.New()                         // default — auto-fetches crumb & cookie
client, err := yahoo.New(yahoo.WithCrumb("abc123")) // inject a pre-fetched crumb
client, err := yahoo.New(yahoo.WithHTTPClient(hc))  // bring your own http.Client
```

`New` initialises a cookie jar, performs the Yahoo Finance consent flow, and fetches an API crumb. All options are optional.

#### Options

| Option | Description |
|--------|-------------|
| `WithHTTPClient(hc *http.Client)` | Replace the default HTTP client (e.g. to set timeouts or a proxy). |
| `WithBaseURL(u string)` | Override the Yahoo Finance API base URL (useful for testing). |
| `WithCrumbURL(u string)` | Override the crumb endpoint URL. |
| `WithCrumb(crumb string)` | Inject a pre-fetched crumb, skipping the consent/crumb-fetch flow. |

### Methods

#### `GetQuote(ctx, ticker) (*Quote, error)`

Returns the current price for a symbol.

```go
quote, err := client.GetQuote(ctx, "AAPL")
// quote.Symbol   → "AAPL"
// quote.Price    → 270.17
// quote.Currency → "USD"
```

Accepts stocks (`AAPL`), crypto (`BTC-USD`), and currency pairs (`USD-EUR`, `RON-USD`). Forex pairs are resolved to the Yahoo Finance `=X` suffix automatically.

#### `FetchQuotes(ctx, symbols) (map[string]float64, error)`

Fetches current prices for multiple symbols in parallel using the v8 chart endpoint (no crumb required). Returns a `map[string]float64` keyed by both the original and normalised ticker (e.g. both `"BRK B"` and `"BRK-B"`).

```go
prices, err := client.FetchQuotes(ctx, []string{"AAPL", "BTC-USD", "BRK B"})
fmt.Println(prices["AAPL"])    // 270.17
fmt.Println(prices["BRK-B"])   // also accessible as prices["BRK B"]
```

#### `FetchFXRates(ctx, currencies, base) (map[string]float64, error)`

Fetches spot FX rates for a list of currencies relative to a base currency, in parallel. The base currency is always `1.0`.

```go
rates, err := client.FetchFXRates(ctx, []string{"EUR", "RON", "USD"}, "USD")
fmt.Println(rates["EUR"])  // e.g. 0.92
fmt.Println(rates["RON"])  // e.g. 4.57
fmt.Println(rates["USD"])  // 1.0
```

#### `GetMonthlyBar(ctx, ticker, year, month) (*HistoricalBar, error)`

Returns OHLC + average price for a symbol in a given calendar month.

```go
bar, err := client.GetMonthlyBar(ctx, "AAPL", 2024, 3)
// bar.Open, bar.High, bar.Low, bar.Close, bar.Avg
```

Accepts stocks, crypto, and currency pairs.

#### `GetYearlyBar(ctx, ticker, year) (*YearlyBar, error)`

Returns OHLC + average price for a full year, aggregated from quarterly data.

```go
yearly, err := client.GetYearlyBar(ctx, "AAPL", 2024)
// yearly.Open, yearly.High, yearly.Low, yearly.Close, yearly.Avg
```

Accepts stocks, crypto, and currency pairs.

### Helper functions

#### `NormalizeTicker(sym string) string`

Converts broker-style tickers to Yahoo Finance format by replacing spaces with hyphens.

```go
yahoo.NormalizeTicker("BRK B")  // → "BRK-B"
yahoo.NormalizeTicker("AAPL")   // → "AAPL"
```

### Types

```go
type Quote struct {
    Symbol   string  `json:"symbol"`
    Price    float64 `json:"price"`
    Currency string  `json:"currency"`
}

type HistoricalBar struct {
    Symbol string  `json:"symbol"`
    Year   int     `json:"year"`
    Month  int     `json:"month"`
    Open   float64 `json:"open"`
    High   float64 `json:"high"`
    Low    float64 `json:"low"`
    Close  float64 `json:"close"`
    Avg    float64 `json:"avg"`
}

type YearlyBar struct {
    Symbol string  `json:"symbol"`
    Year   int     `json:"year"`
    Open   float64 `json:"open"`
    High   float64 `json:"high"`
    Low    float64 `json:"low"`
    Close  float64 `json:"close"`
    Avg    float64 `json:"avg"`
}
```

### Sentinel errors

| Error | When returned |
|-------|---------------|
| `yahoo.ErrTickerNotFound` | The symbol returned no results from Yahoo Finance. |
| `yahoo.ErrAPIError` | Yahoo Finance responded with an API-level error. |
| `yahoo.ErrNoData` | Yahoo Finance has no data for the requested period. |

```go
quote, err := client.GetQuote(ctx, "INVALID")
if errors.Is(err, yahoo.ErrTickerNotFound) {
    // handle missing symbol
}
```

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
