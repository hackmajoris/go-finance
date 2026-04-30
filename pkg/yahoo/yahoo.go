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
	"time"
)

// ErrTickerNotFound is returned when the requested symbol has no results.
// ErrAPIError is returned when Yahoo Finance responds with an error.
// ErrNoData is returned when Yahoo Finance has no data for the requested period.
var (
	ErrTickerNotFound = errors.New("ticker not found")
	ErrAPIError       = errors.New("yahoo finance api error")
	ErrNoData         = errors.New("no data available for the requested period")
)

const (
	defaultBaseURL = "https://query2.finance.yahoo.com"
	crumbURL       = "https://query2.finance.yahoo.com/v1/test/getcrumb"
	financeURL     = "https://finance.yahoo.com/"
)

var reCRSF = regexp.MustCompile(`csrfToken" value="([^"]+)"`)
var reForexPair = regexp.MustCompile(`^([A-Z]{3})-([A-Z]{3})$`)

// Option configures a Client.
type Option func(*Client)

// Client fetches quotes from Yahoo Finance.
type Client struct {
	httpClient *http.Client
	baseURL    string
	crumbURL   string
	crumb      string
}

// Quote holds the price data returned for a symbol.
type Quote struct {
	Symbol   string  `json:"symbol"`
	Price    float64 `json:"price"`
	Currency string  `json:"currency"`
}

// HistoricalBar holds monthly OHLC data for a symbol.
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

// YearlyBar holds yearly OHLC data aggregated from quarterly data.
type YearlyBar struct {
	Symbol string  `json:"symbol"`
	Year   int     `json:"year"`
	Open   float64 `json:"open"`
	High   float64 `json:"high"`
	Low    float64 `json:"low"`
	Close  float64 `json:"close"`
	Avg    float64 `json:"avg"`
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
