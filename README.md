# go-finance

Fetches real-time and historical stock, crypto, and currency prices from Yahoo Finance.

```json
{"symbol":"AAPL","price":270.17,"currency":"USD"}
```

## Prerequisites

- Go 1.22+
- [golangci-lint](https://golangci-lint.run/)

## Using the package in another Go app

```bash
go get github.com/hackmajoris/go-finance@v0.1.4
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

    // Monthly close price (v8 endpoint — no crumb, works in Docker/containers)
    close, err := client.FetchMonthlyBar(ctx, "^GSPC", 2024, 3)
    if err != nil {
        log.Fatal(err)
    }
    fmt.Println(close) // e.g. 5218.19

    // Monthly OHLC (v7 endpoint — requires crumb/consent flow)
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
| `WithBaseURL(u string)` | Override the Yahoo Finance v7 API base URL (useful for testing). |
| `WithV8BaseURL(u string)` | Override the Yahoo Finance v8 chart endpoint base URL (useful for testing). |
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

#### `GetPE(ctx, ticker) (*PERatio, error)`

Returns the trailing twelve-month and forward P/E ratios for a stock. `ForwardPE` is `0` when Yahoo has no analyst earnings estimate.

```go
pe, err := client.GetPE(ctx, "AAPL")
// pe.PE        → 34.71 (trailing)
// pe.ForwardPE → 28.05
```

Accepts stocks (`AAPL`), crypto (`BTC-USD`), and currency pairs (`USD-EUR`, `RON-USD`). Forex pairs are resolved to the Yahoo Finance `=X` suffix automatically.

#### `GetFreeCashFlow(ctx, ticker) (*FreeCashFlow, error)`

Returns the trailing twelve-month free cash flow for a stock, plus a plain-language `Interpretation` of the sign.

```go
fcf, err := client.GetFreeCashFlow(ctx, "AAPL")
// fcf.FCF            → 1.07721875456e+11
// fcf.Interpretation → "Positive free cash flow: the business generates more cash..."
```

#### `GetOperatingCashFlowVsNetIncome(ctx, ticker) (*CashFlowQuality, error)`

Returns trailing twelve-month operating cash flow against net income — an earnings-quality check. `Ratio` near `1` means earnings are cash-backed; well below `1` (or negative) flags possible accrual-inflated earnings; well above `1` usually means net income was suppressed by non-cash charges rather than a bullish signal, and the ratio grows unstable as `NetIncome` approaches zero. `Interpretation` explains which bucket the result falls into.

```go
q, err := client.GetOperatingCashFlowVsNetIncome(ctx, "AAPL")
// q.OperatingCashFlow → 1.46723995648e+11
// q.NetIncome         → 1.289299968e+11
// q.Ratio             → 1.138...
// q.Interpretation    → "Ratio close to 1: earnings are roughly cash-backed..."
```

#### `GetDebtToEquity(ctx, ticker) (*DebtToEquity, error)`

Returns the debt-to-equity ratio (total debt / total equity, as a percentage). `Ratio` is `0` for a legitimate debt-free/net-cash company, not an error. Read against sector peers, not an absolute threshold — capital-heavy industries (shipping, REITs, utilities) run naturally higher leverage.

```go
d2e, err := client.GetDebtToEquity(ctx, "AAPL")
// d2e.Ratio          → 78.445
// d2e.Interpretation → "Equity funds more of the business than debt..."
```

#### `GetEVToEBITDA(ctx, ticker) (*EVToEBITDA, error)`

Returns the enterprise-value-to-EBITDA ratio — capital-structure neutral (accounts for debt and cash), which makes it more comparable across companies with different leverage than P/E. A negative ratio means EBITDA is negative and is returned as valid data, not an error.

```go
ev, err := client.GetEVToEBITDA(ctx, "AAPL")
// ev.Ratio          → 27.01
// ev.Interpretation → "Above 15x: expensive relative to operating earnings..."
```

#### `ClassifyHealth(fcf, cfq, d2e) (HealthRating, string)`

Composite read on financial soundness — cash generation, earnings quality, and leverage — from already-fetched `FreeCashFlow`, `CashFlowQuality`, and `DebtToEquity`. Any argument may be `nil` if that indicator failed to fetch; the rating is based on whatever's available. No network call — pure function over data you already have. Deliberately separate from valuation: a company can be financially healthy and still expensive, or shaky and still "cheap" (value trap).

```go
health, reason := yahoo.ClassifyHealth(fcf, cfq, d2e)
// health → yahoo.HealthHealthy | HealthFair | HealthWeak | HealthUnhealthy
```

#### `ClassifyValuation(pe, ev) (ValuationRating, string)`

Composite read on whether the price looks cheap or expensive relative to earnings and operating cash flow, from already-fetched `PERatio` (forward vs. trailing trend) and `EVToEBITDA` (level). No network call.

```go
valuation, reason := yahoo.ClassifyValuation(pe, ev)
// valuation → yahoo.ValuationUndervalued | ValuationFair | ValuationOvervalued | ValuationUnclear
```

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

#### `FetchMonthlyBar(ctx, symbol, year, month) (float64, error)`

Returns the closing price for a symbol in a given calendar month using the **v8 chart endpoint** — no crumb or consent flow required. Works reliably in Docker and other containerised environments.

```go
close, err := client.FetchMonthlyBar(ctx, "^GSPC", 2024, 3)
// close → 5218.19
```

Use this instead of `GetMonthlyBar` when the consent flow is unavailable (e.g. running in a container whose IP is flagged by Yahoo's bot-detection).

#### `GetMonthlyBar(ctx, ticker, year, month) (*HistoricalBar, error)`

Returns OHLC + average price for a symbol in a given calendar month using the v7 endpoint (requires crumb).

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

#### `FetchFiftyTwoWeekRange(ctx, ticker) (*FiftyTwoWeekRange, error)`

Returns the 52-week high/low for a symbol using the **v8 chart endpoint** — no crumb or consent flow required. `Pct` is the position of `Current` between `Low` (0) and `High` (1), clamped to `[0, 1]`, handy for rendering a range bar.

```go
rng, err := client.FetchFiftyTwoWeekRange(ctx, "AAPL")
// rng.High, rng.Low, rng.Current, rng.Pct
```

Accepts stocks, crypto, and currency pairs.

#### `FetchPerformance(ctx, ticker) (*PerformanceReturns, error)`

Returns YTD, 1-year, 3-year, and 5-year percentage price change using the **v8 chart endpoint** — no crumb or consent flow required. A period is `0` when the symbol has no trading history that far back (e.g. a recent IPO).

```go
perf, err := client.FetchPerformance(ctx, "AAPL")
// perf.YTD, perf.OneYear, perf.ThreeYear, perf.FiveYear
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

type PERatio struct {
    Symbol         string  `json:"symbol"`
    PE             float64 `json:"pe"`
    ForwardPE      float64 `json:"forwardPE"`
    Interpretation string  `json:"interpretation"`
}

type FreeCashFlow struct {
    Symbol         string  `json:"symbol"`
    FCF            float64 `json:"fcf"`
    Interpretation string  `json:"interpretation"`
}

type CashFlowQuality struct {
    Symbol            string  `json:"symbol"`
    OperatingCashFlow float64 `json:"operatingCashFlow"`
    NetIncome         float64 `json:"netIncome"`
    Ratio             float64 `json:"ratio"`
    Interpretation    string  `json:"interpretation"`
}

type DebtToEquity struct {
    Symbol         string  `json:"symbol"`
    Ratio          float64 `json:"ratio"`
    Interpretation string  `json:"interpretation"`
}

type EVToEBITDA struct {
    Symbol         string  `json:"symbol"`
    Ratio          float64 `json:"ratio"`
    Interpretation string  `json:"interpretation"`
}

// HealthRating: "healthy" | "fair" | "weak" | "unhealthy"
type HealthRating string

// ValuationRating: "undervalued" | "fair" | "overvalued" | "unclear"
type ValuationRating string

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

type FiftyTwoWeekRange struct {
    Symbol  string  `json:"symbol"`
    High    float64 `json:"high"`
    Low     float64 `json:"low"`
    Current float64 `json:"current"`
    Pct     float64 `json:"pct"`
}

type PerformanceReturns struct {
    Symbol    string  `json:"symbol"`
    YTD       float64 `json:"ytd"`
    OneYear   float64 `json:"oneYear"`
    ThreeYear float64 `json:"threeYear"`
    FiveYear  float64 `json:"fiveYear"`
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

## Examples

Runnable examples live in `examples/`, one per method:

```bash
go run ./examples/quote AAPL
go run ./examples/pe AAPL
go run ./examples/freecashflow AAPL
go run ./examples/cashflowquality AAPL
go run ./examples/debttoequity AAPL
go run ./examples/evtoebitda AAPL
go run ./examples/monthlybar AAPL 2024 3
go run ./examples/yearlybar AAPL 2024
go run ./examples/fetchmonthlybar               # ^GSPC 2024/3 by default
go run ./examples/fetchquotes AAPL BTC-USD       # defaults to AAPL, BTC-USD, "BRK B"
go run ./examples/fetchfxrates USD EUR RON       # base currency first
go run ./examples/fiftytwoweekrange AAPL
go run ./examples/performance AAPL
go run ./examples/summary AAPL                   # every no-extra-params indicator for one ticker
```

## Development

```bash
make test    # run tests with race detector
make lint    # run linter
```

## License

MIT
