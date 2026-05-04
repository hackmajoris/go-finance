// Package yahoo provides a client for fetching quotes from Yahoo Finance.
package yahoo

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
)

var (
	// ErrTickerNotFound is returned when the requested symbol has no results on Yahoo Finance.
	ErrTickerNotFound = errors.New("ticker not found")
	// ErrAPIError is returned when Yahoo Finance responds with a non-200 status or an API-level error.
	ErrAPIError = errors.New("yahoo finance api error")
	// ErrNoData is returned when Yahoo Finance has no data for the requested period (e.g. future dates or delisted symbols).
	ErrNoData = errors.New("no data available for the requested period")
)

const (
	defaultBaseURL = "https://query2.finance.yahoo.com"
	crumbURL       = "https://query2.finance.yahoo.com/v1/test/getcrumb"
	financeURL     = "https://finance.yahoo.com/"
)

var reCRSF = regexp.MustCompile(`csrfToken" value="([^"]+)"`)
var reForexPair = regexp.MustCompile(`^([A-Z]{3})-([A-Z]{3})$`)

// Option is a functional option for configuring a Client.
type Option func(*Client)

// Client fetches real-time and historical quotes from Yahoo Finance.
// Use New to create a Client; it handles the session cookie and crumb
// handshake required by the Yahoo Finance API automatically.
type Client struct {
	httpClient *http.Client
	baseURL    string
	crumbURL   string
	crumb      string
}

// Quote holds the current price data returned for a single symbol.
type Quote struct {
	Symbol   string  `json:"symbol"`   // Yahoo Finance ticker (e.g. "AAPL", "BTC-USD", "USD-EUR")
	Price    float64 `json:"price"`    // Regular market price
	Currency string  `json:"currency"` // ISO 4217 currency code (e.g. "USD", "EUR")
}

// HistoricalBar holds OHLC price data for a single calendar month.
// Avg is the simple average of Open, High, Low, and Close.
type HistoricalBar struct {
	Symbol string  `json:"symbol"` // Yahoo Finance ticker
	Year   int     `json:"year"`   // Calendar year (e.g. 2024)
	Month  int     `json:"month"`  // Calendar month (1–12)
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Avg    float64 `json:"avg"` // (Open + High + Low + Close) / 4
}

// YearlyBar holds OHLC price data aggregated across a full calendar year.
// Open comes from Q1, Close from Q4, High and Low are the extremes across all four quarters.
// Avg is the simple average of Open, High, Low, and Close.
type YearlyBar struct {
	Symbol string  `json:"symbol"` // Yahoo Finance ticker
	Year   int     `json:"year"`   // Calendar year (e.g. 2024)
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Avg    float64 `json:"avg"` // (Open + High + Low + Close) / 4
}

// New creates a Client with a cookie jar and optional overrides.
func New(opts ...Option) (*Client, error) {
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}
	c := &Client{
		httpClient: &http.Client{Jar: jar},
		baseURL:    defaultBaseURL,
		crumbURL:   crumbURL,
	}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL overrides the Yahoo Finance API base URL.
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// WithCrumbURL overrides the crumb endpoint URL.
func WithCrumbURL(u string) Option {
	return func(c *Client) { c.crumbURL = u }
}

// WithCrumb injects a pre-fetched crumb, skipping the consent flow.
func WithCrumb(crumb string) Option {
	return func(c *Client) { c.crumb = crumb }
}

func (c *Client) get(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.5")
	return c.httpClient.Do(req)
}

// fetchCrumb handles the Yahoo Finance consent flow and then retrieves the crumb.
func (c *Client) fetchCrumb(ctx context.Context) error {
	// Step 1: visit Yahoo Finance; may redirect to consent.yahoo.com
	resp, err := c.get(ctx, financeURL)
	if err != nil {
		return fmt.Errorf("warming session: %w", err)
	}
	finalURL := resp.Request.URL.String()
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	// Step 2: if we landed on the consent page, accept it
	if strings.Contains(finalURL, "consent.yahoo.com") {
		if err := c.acceptConsent(ctx, finalURL, string(body)); err != nil {
			return err
		}
	}

	// Step 3: fetch the crumb
	return c.doFetchCrumb(ctx)
}

func (c *Client) acceptConsent(ctx context.Context, consentPageURL, html string) error {
	matches := reCRSF.FindStringSubmatch(html)
	if len(matches) < 2 {
		// Page may not require consent (e.g. mock) — skip silently
		return nil
	}
	csrfToken := matches[1]

	u, err := url.Parse(consentPageURL)
	if err != nil {
		return fmt.Errorf("parsing consent URL: %w", err)
	}
	sessionID := u.Query().Get("sessionId")

	form := url.Values{}
	form.Set("csrfToken", csrfToken)
	form.Set("sessionId", sessionID)
	form.Set("originalDoneUrl", financeURL)
	form.Set("namespace", "yahoo")
	form.Add("agree", "agree")
	form.Add("agree", "agree")

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, consentPageURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("building consent POST: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("Referer", consentPageURL)

	postResp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("posting consent: %w", err)
	}
	_ = postResp.Body.Close()
	return nil
}

func (c *Client) doFetchCrumb(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.crumbURL, nil)
	if err != nil {
		return fmt.Errorf("building crumb request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("fetching crumb: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: crumb status %d", ErrAPIError, resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("reading crumb: %w", err)
	}
	c.crumb = strings.TrimSpace(string(b))
	return nil
}

// GetQuote returns the current price for a ticker. Forex pairs like "USD-EUR" are resolved automatically.
func (c *Client) GetQuote(ctx context.Context, ticker string) (*Quote, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	quote, err := c.doGetQuote(ctx, ticker)
	if err != nil {
		return nil, err
	}

	// Yahoo can return a result with price 0 for unrecognized symbols.
	// Fiat forex pairs (e.g. "RON-USD", "USD-EUR") need the "XXXYYY=X" format.
	if (quote == nil || quote.Price == 0) && reForexPair.MatchString(ticker) {
		m := reForexPair.FindStringSubmatch(ticker)
		quote, err = c.doGetQuote(ctx, m[1]+m[2]+"=X")
		if err != nil {
			return nil, err
		}
	}

	if quote == nil || quote.Price == 0 {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	quote.Symbol = ticker
	return quote, nil
}

func (c *Client) doGetQuote(ctx context.Context, symbol string) (*Quote, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v7/finance/quote", c.baseURL))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("symbols", symbol)
	q.Set("crumb", c.crumb)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}

	var payload struct {
		QuoteResponse struct {
			Result []struct {
				Symbol             string  `json:"symbol"`
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				Currency           string  `json:"currency"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"quoteResponse"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if payload.QuoteResponse.Error != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, payload.QuoteResponse.Error)
	}

	if len(payload.QuoteResponse.Result) == 0 {
		return nil, nil
	}

	r := payload.QuoteResponse.Result[0]
	return &Quote{
		Symbol:   r.Symbol,
		Price:    r.RegularMarketPrice,
		Currency: r.Currency,
	}, nil
}

// GetMonthlyBar returns the OHLC data for a symbol in a given month. Forex pairs like "USD-EUR" are resolved automatically.
func (c *Client) GetMonthlyBar(ctx context.Context, ticker string, year, month int) (*HistoricalBar, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	bar, err := c.doGetMonthlyBar(ctx, ticker, year, month)
	if err != nil {
		return nil, err
	}

	// Attempt forex pair format for unrecognized symbols.
	if (bar == nil || bar.Close == 0) && reForexPair.MatchString(ticker) {
		m := reForexPair.FindStringSubmatch(ticker)
		bar, err = c.doGetMonthlyBar(ctx, m[1]+m[2]+"=X", year, month)
		if err != nil {
			return nil, err
		}
	}

	if bar == nil || bar.Close == 0 {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	bar.Symbol = ticker
	return bar, nil
}

func (c *Client) doGetMonthlyBar(ctx context.Context, symbol string, year, month int) (*HistoricalBar, error) {
	// Calculate period1 (first second of the month) and period2 (first second of next month)
	period1 := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Unix()
	period2 := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).AddDate(0, 1, 0).Unix()

	u, err := url.Parse(fmt.Sprintf("%s/v7/finance/chart/%s", c.baseURL, symbol))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("interval", "1mo")
	q.Set("period1", fmt.Sprintf("%d", period1))
	q.Set("period2", fmt.Sprintf("%d", period2))
	q.Set("crumb", c.crumb)
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil
	}
	if resp.StatusCode == http.StatusBadRequest {
		var errBody struct {
			Chart struct {
				Error struct {
					Description string `json:"description"`
				} `json:"error"`
			} `json:"chart"`
		}
		if jsonErr := json.NewDecoder(resp.Body).Decode(&errBody); jsonErr == nil && errBody.Chart.Error.Description != "" {
			return nil, fmt.Errorf("%w: %s", ErrNoData, errBody.Chart.Error.Description)
		}
		return nil, ErrNoData
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Indicators struct {
					Quote []struct {
						Open  []float64 `json:"open"`
						High  []float64 `json:"high"`
						Low   []float64 `json:"low"`
						Close []float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if payload.Chart.Error != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, payload.Chart.Error)
	}

	if len(payload.Chart.Result) == 0 || len(payload.Chart.Result[0].Indicators.Quote) == 0 || len(payload.Chart.Result[0].Indicators.Quote[0].Close) == 0 {
		return nil, nil
	}

	quote := payload.Chart.Result[0].Indicators.Quote[0]
	open := quote.Open[0]
	high := quote.High[0]
	low := quote.Low[0]
	closePrice := quote.Close[0]
	avg := (open + high + low + closePrice) / 4

	return &HistoricalBar{
		Year:  year,
		Month: month,
		Open:  open,
		High:  high,
		Low:   low,
		Close: closePrice,
		Avg:   avg,
	}, nil
}

// GetYearlyBar returns yearly OHLC data by aggregating 4 quarters. Forex pairs like "USD-EUR" are resolved automatically.
func (c *Client) GetYearlyBar(ctx context.Context, ticker string, year int) (*YearlyBar, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	bar, err := c.doGetYearlyBar(ctx, ticker, year)
	if err != nil {
		return nil, err
	}

	// Attempt forex pair format for unrecognized symbols.
	if (bar == nil || bar.Close == 0) && reForexPair.MatchString(ticker) {
		m := reForexPair.FindStringSubmatch(ticker)
		bar, err = c.doGetYearlyBar(ctx, m[1]+m[2]+"=X", year)
		if err != nil {
			return nil, err
		}
	}

	if bar == nil || bar.Close == 0 {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	bar.Symbol = ticker
	return bar, nil
}

func (c *Client) doGetYearlyBar(ctx context.Context, symbol string, year int) (*YearlyBar, error) {
	// Fetch 4 quarters and aggregate
	quarters := make([]*HistoricalBar, 4)
	for q := 0; q < 4; q++ {
		month := q*3 + 1
		bar, err := c.doGetMonthlyBar(ctx, symbol, year, month)
		if err != nil {
			return nil, err
		}
		if bar == nil {
			return nil, nil
		}
		quarters[q] = bar
	}

	// Aggregate: open from Q1, close from Q4, high/low from all
	open := quarters[0].Open
	closePrice := quarters[3].Close
	high := quarters[0].High
	low := quarters[0].Low

	for _, q := range quarters {
		if q.High > high {
			high = q.High
		}
		if q.Low < low {
			low = q.Low
		}
	}

	avg := (open + high + low + closePrice) / 4

	return &YearlyBar{
		Year:  year,
		Open:  open,
		High:  high,
		Low:   low,
		Close: closePrice,
		Avg:   avg,
	}, nil
}

// NormalizeTicker converts broker tickers to Yahoo Finance format.
// e.g. "BRK B" → "BRK-B"
func NormalizeTicker(sym string) string {
	return strings.ReplaceAll(sym, " ", "-")
}

// FetchQuotes returns a map of symbol → current price for each symbol in the list,
// fetching in parallel via the v8 chart endpoint (no crumb required).
// Both the original and normalized ticker are stored in the result map.
// Partial results are returned when only some fetches fail.
func (c *Client) FetchQuotes(ctx context.Context, symbols []string) (map[string]float64, error) {
	type result struct {
		sym   string
		price float64
		err   error
	}

	ch := make(chan result, len(symbols))
	var wg sync.WaitGroup

	for _, sym := range symbols {
		wg.Add(1)
		go func(sym string) {
			defer wg.Done()
			price, err := c.fetchOneChart(ctx, NormalizeTicker(sym))
			ch <- result{sym, price, err}
		}(sym)
	}

	wg.Wait()
	close(ch)

	out := make(map[string]float64, len(symbols))
	var errs []string
	for r := range ch {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.sym, r.err))
			continue
		}
		out[r.sym] = r.price
		out[NormalizeTicker(r.sym)] = r.price
	}

	if len(errs) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("all price fetches failed: %s", strings.Join(errs[:min(3, len(errs))], "; "))
	}
	return out, nil
}

// FetchFXRates returns spot rates for each currency relative to base (e.g. "USD"),
// fetching in parallel via the v8 chart endpoint. The base currency always gets rate 1.0.
// Partial results are returned when only some fetches fail.
func (c *Client) FetchFXRates(ctx context.Context, currencies []string, base string) (map[string]float64, error) {
	rates := map[string]float64{base: 1.0}
	var toFetch []string
	for _, cur := range currencies {
		if cur != "" && cur != base {
			toFetch = append(toFetch, cur)
		}
	}
	if len(toFetch) == 0 {
		return rates, nil
	}

	type result struct {
		currency string
		rate     float64
		err      error
	}
	ch := make(chan result, len(toFetch))
	var wg sync.WaitGroup

	for _, cur := range toFetch {
		wg.Add(1)
		go func(cur string) {
			defer wg.Done()
			rate, err := c.fetchOneChart(ctx, cur+base+"=X")
			ch <- result{cur, rate, err}
		}(cur)
	}
	wg.Wait()
	close(ch)

	var errs []string
	for r := range ch {
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.currency, r.err))
			continue
		}
		rates[r.currency] = r.rate
	}
	if len(errs) > 0 && len(rates) == 1 {
		return nil, fmt.Errorf("all FX fetches failed: %s", strings.Join(errs, "; "))
	}
	return rates, nil
}

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

func (c *Client) fetchOneChart(ctx context.Context, symbol string) (float64, error) {
	rawURL := fmt.Sprintf("https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d", symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: HTTP %d", ErrAPIError, resp.StatusCode)
	}

	var cr chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return 0, err
	}
	if len(cr.Chart.Result) == 0 {
		return 0, fmt.Errorf("%w: %s", ErrTickerNotFound, symbol)
	}
	price := cr.Chart.Result[0].Meta.RegularMarketPrice
	if price == 0 {
		return 0, fmt.Errorf("%w: %s", ErrTickerNotFound, symbol)
	}
	return price, nil
}
