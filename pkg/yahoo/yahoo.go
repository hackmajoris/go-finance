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
	defaultBaseURL   = "https://query2.finance.yahoo.com"
	defaultV8BaseURL = "https://query1.finance.yahoo.com"
	crumbURL         = "https://query2.finance.yahoo.com/v1/test/getcrumb"
	financeURL       = "https://finance.yahoo.com/"
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
	v8BaseURL  string
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
		v8BaseURL:  defaultV8BaseURL,
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

// WithV8BaseURL overrides the Yahoo Finance v8 chart endpoint base URL (useful for testing).
func WithV8BaseURL(u string) Option {
	return func(c *Client) { c.v8BaseURL = u }
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

type v7QuoteResult struct {
	Symbol             string  `json:"symbol"`
	RegularMarketPrice float64 `json:"regularMarketPrice"`
	Currency           string  `json:"currency"`
	TrailingPE         float64 `json:"trailingPE"`
	ForwardPE          float64 `json:"forwardPE"`
}

// fetchV7Quote fetches the raw v7 quote result for a symbol (requires crumb).
func (c *Client) fetchV7Quote(ctx context.Context, symbol string) (*v7QuoteResult, error) {
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
			Result []v7QuoteResult `json:"result"`
			Error  interface{}     `json:"error"`
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
	return &r, nil
}

func (c *Client) doGetQuote(ctx context.Context, symbol string) (*Quote, error) {
	r, err := c.fetchV7Quote(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if r == nil {
		return nil, nil
	}
	return &Quote{
		Symbol:   r.Symbol,
		Price:    r.RegularMarketPrice,
		Currency: r.Currency,
	}, nil
}

// PERatio holds the trailing and forward P/E ratios for a symbol.
// ForwardPE is 0 when Yahoo has no analyst earnings estimate for the stock.
type PERatio struct {
	Symbol         string  `json:"symbol"`         // Yahoo Finance ticker
	PE             float64 `json:"pe"`             // trailing twelve-month price/earnings ratio
	ForwardPE      float64 `json:"forwardPE"`      // price / next fiscal year's estimated earnings
	Interpretation string  `json:"interpretation"` // plain-language read of the ratio; compare against sector peers, not in isolation
}

// describePE explains what the trailing/forward P/E combination signals.
func describePE(pe, forwardPE float64) string {
	switch {
	case pe == 0:
		return "No trailing P/E (company has no positive trailing earnings). Forward P/E, if present, reflects analyst earnings estimates instead."
	case forwardPE == 0:
		return "No forward P/E (no analyst earnings estimate). Trailing P/E only — compare against sector peers to judge if the stock is cheap or expensive."
	case forwardPE < pe:
		return "Forward P/E below trailing P/E: earnings expected to grow, market pricing in improvement."
	case forwardPE > pe:
		return "Forward P/E above trailing P/E: earnings expected to shrink, or current earnings include a one-off boost."
	default:
		return "Forward P/E roughly equals trailing P/E: earnings expected to stay flat."
	}
}

// GetPE returns the trailing and forward P/E ratios for a stock ticker.
func (c *Client) GetPE(ctx context.Context, ticker string) (*PERatio, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	r, err := c.fetchV7Quote(ctx, ticker)
	if err != nil {
		return nil, err
	}

	if r == nil || (r.TrailingPE == 0 && r.ForwardPE == 0) {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	return &PERatio{
		Symbol:         ticker,
		PE:             r.TrailingPE,
		ForwardPE:      r.ForwardPE,
		Interpretation: describePE(r.TrailingPE, r.ForwardPE),
	}, nil
}

// FreeCashFlow holds the trailing twelve-month free cash flow for a symbol.
type FreeCashFlow struct {
	Symbol         string  `json:"symbol"`         // Yahoo Finance ticker
	FCF            float64 `json:"fcf"`            // trailing twelve-month free cash flow, in the reporting currency
	Interpretation string  `json:"interpretation"` // plain-language read of the value
}

// describeFCF explains what the free cash flow sign signals.
func describeFCF(fcf float64) string {
	switch {
	case fcf > 0:
		return "Positive free cash flow: the business generates more cash than it spends on operations and capex, self-funding without relying on debt or share issuance."
	case fcf < 0:
		return "Negative free cash flow: the business is burning cash on operations/capex. Normal for early-growth or heavy-capex phases, a warning sign if sustained without a credible path to positive FCF."
	default:
		return "Zero free cash flow: operating cash flow exactly offset by capital expenditure."
	}
}

// GetFreeCashFlow returns the trailing twelve-month free cash flow for a stock ticker.
func (c *Client) GetFreeCashFlow(ctx context.Context, ticker string) (*FreeCashFlow, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	fcf, err := c.fetchFreeCashFlow(ctx, ticker)
	if err != nil {
		return nil, err
	}
	if fcf == nil {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	return &FreeCashFlow{Symbol: ticker, FCF: *fcf, Interpretation: describeFCF(*fcf)}, nil
}

// fetchFreeCashFlow fetches the financialData.freeCashflow value for a symbol (requires crumb).
func (c *Client) fetchFreeCashFlow(ctx context.Context, symbol string) (*float64, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v10/finance/quoteSummary/%s", c.baseURL, symbol))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("modules", "financialData")
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}

	var payload struct {
		QuoteSummary struct {
			Result []struct {
				FinancialData struct {
					FreeCashflow struct {
						Raw float64 `json:"raw"`
					} `json:"freeCashflow"`
				} `json:"financialData"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"quoteSummary"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if payload.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, payload.QuoteSummary.Error)
	}

	if len(payload.QuoteSummary.Result) == 0 {
		return nil, nil
	}

	fcf := payload.QuoteSummary.Result[0].FinancialData.FreeCashflow.Raw
	return &fcf, nil
}

// CashFlowQuality holds operating cash flow against net income for a symbol.
// Ratio near 1 signals earnings backed by cash. Well below 1 (or negative) signals
// earnings inflated by non-cash items (accruals, one-off gains) — a quality warning.
// Well above 1 usually means net income was suppressed by non-cash charges (D&A,
// impairments) rather than "extra good" — investigate rather than read as bullish.
// The ratio is unstable whenever NetIncome sits near zero, since any OCF then
// produces an extreme value regardless of how normal cash generation actually is.
type CashFlowQuality struct {
	Symbol            string  `json:"symbol"`            // Yahoo Finance ticker
	OperatingCashFlow float64 `json:"operatingCashFlow"` // trailing twelve-month cash from operations
	NetIncome         float64 `json:"netIncome"`         // trailing twelve-month net income
	Ratio             float64 `json:"ratio"`             // OperatingCashFlow / NetIncome; 0 when NetIncome is 0
	Interpretation    string  `json:"interpretation"`    // plain-language read of the ratio
}

// describeCashFlowQuality explains what the OCF/NetIncome ratio signals.
func describeCashFlowQuality(ratio, netIncome float64) string {
	switch {
	case netIncome == 0:
		return "Net income is zero: ratio undefined, cannot assess earnings quality this way."
	case ratio >= 0.8 && ratio <= 1.3:
		return "Ratio close to 1: earnings are roughly cash-backed, a healthy sign."
	case ratio < 0.8 && ratio >= 0:
		return "Ratio well below 1: earnings not fully backed by cash — possible accruals or aggressive revenue recognition inflating net income. Worth investigating."
	case ratio < 0:
		return "Negative ratio: operating cash flow and net income have opposite signs. Worth investigating which one is the outlier and why."
	default:
		return "Ratio well above 1: net income likely suppressed by non-cash charges (depreciation, impairments, write-downs) rather than a sign of extra strength. Also unstable when net income is near zero — check the absolute net income figure before drawing conclusions."
	}
}

// GetOperatingCashFlowVsNetIncome returns trailing twelve-month operating cash flow
// against net income for a stock ticker.
func (c *Client) GetOperatingCashFlowVsNetIncome(ctx context.Context, ticker string) (*CashFlowQuality, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	q, err := c.fetchCashFlowQuality(ctx, ticker)
	if err != nil {
		return nil, err
	}
	if q == nil {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	q.Symbol = ticker
	if q.NetIncome != 0 {
		q.Ratio = q.OperatingCashFlow / q.NetIncome
	}
	q.Interpretation = describeCashFlowQuality(q.Ratio, q.NetIncome)
	return q, nil
}

// fetchCashFlowQuality fetches operatingCashflow (financialData) and netIncomeToCommon
// (defaultKeyStatistics) for a symbol in a single quoteSummary call (requires crumb).
func (c *Client) fetchCashFlowQuality(ctx context.Context, symbol string) (*CashFlowQuality, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v10/finance/quoteSummary/%s", c.baseURL, symbol))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("modules", "financialData,defaultKeyStatistics")
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}

	var payload struct {
		QuoteSummary struct {
			Result []struct {
				FinancialData struct {
					OperatingCashflow struct {
						Raw float64 `json:"raw"`
					} `json:"operatingCashflow"`
				} `json:"financialData"`
				DefaultKeyStatistics struct {
					NetIncomeToCommon struct {
						Raw float64 `json:"raw"`
					} `json:"netIncomeToCommon"`
				} `json:"defaultKeyStatistics"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"quoteSummary"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if payload.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, payload.QuoteSummary.Error)
	}

	if len(payload.QuoteSummary.Result) == 0 {
		return nil, nil
	}

	r := payload.QuoteSummary.Result[0]
	return &CashFlowQuality{
		OperatingCashFlow: r.FinancialData.OperatingCashflow.Raw,
		NetIncome:         r.DefaultKeyStatistics.NetIncomeToCommon.Raw,
	}, nil
}

// DebtToEquity holds the debt-to-equity ratio for a symbol.
// Yahoo reports it as a percentage (e.g. 150.5 means total debt is 1.5x total equity).
type DebtToEquity struct {
	Symbol         string  `json:"symbol"`         // Yahoo Finance ticker
	Ratio          float64 `json:"ratio"`          // total debt / total equity, as a percentage
	Interpretation string  `json:"interpretation"` // plain-language read of the ratio; compare against sector peers, not an absolute threshold
}

// describeDebtToEquity explains what the debt-to-equity percentage signals.
func describeDebtToEquity(ratio float64) string {
	switch {
	case ratio == 0:
		return "No reported debt relative to equity — a net-cash or debt-free balance sheet. Not automatically a strength; may just mean the company doesn't use leverage."
	case ratio < 100:
		return "Equity funds more of the business than debt. Generally lower interest-rate and refinancing risk, but compare against sector peers — capital-heavy industries (shipping, REITs, utilities) run naturally higher leverage than this and it's still normal for them."
	default:
		return "Debt exceeds equity. Higher leverage means higher interest burden and higher risk in a downturn, but common and often healthy in capital-heavy or asset-backed industries (shipping, REITs, utilities). Read against sector peers and pair with cash flow strength, not in isolation."
	}
}

// GetDebtToEquity returns the debt-to-equity ratio for a stock ticker.
func (c *Client) GetDebtToEquity(ctx context.Context, ticker string) (*DebtToEquity, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	ratio, err := c.fetchDebtToEquity(ctx, ticker)
	if err != nil {
		return nil, err
	}
	if ratio == nil {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	return &DebtToEquity{Symbol: ticker, Ratio: *ratio, Interpretation: describeDebtToEquity(*ratio)}, nil
}

// fetchDebtToEquity fetches the financialData.debtToEquity value for a symbol (requires crumb).
func (c *Client) fetchDebtToEquity(ctx context.Context, symbol string) (*float64, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v10/finance/quoteSummary/%s", c.baseURL, symbol))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("modules", "financialData")
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}

	var payload struct {
		QuoteSummary struct {
			Result []struct {
				FinancialData struct {
					DebtToEquity struct {
						Raw float64 `json:"raw"`
					} `json:"debtToEquity"`
				} `json:"financialData"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"quoteSummary"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if payload.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, payload.QuoteSummary.Error)
	}

	if len(payload.QuoteSummary.Result) == 0 {
		return nil, nil
	}

	ratio := payload.QuoteSummary.Result[0].FinancialData.DebtToEquity.Raw
	return &ratio, nil
}

// EVToEBITDA holds the enterprise-value-to-EBITDA ratio for a symbol.
// Unlike P/E, it's capital-structure neutral (accounts for debt and cash),
// which makes it more comparable across companies with different leverage.
type EVToEBITDA struct {
	Symbol         string  `json:"symbol"`         // Yahoo Finance ticker
	Ratio          float64 `json:"ratio"`          // enterprise value / trailing twelve-month EBITDA
	Interpretation string  `json:"interpretation"` // plain-language read of the ratio; compare against sector peers, not an absolute threshold
}

// describeEVToEBITDA explains what the EV/EBITDA ratio signals.
func describeEVToEBITDA(ratio float64) string {
	switch {
	case ratio < 0:
		return "Negative ratio: EBITDA is negative, the business isn't generating positive operating earnings. Common for early-stage or distressed companies — treat as a warning, not a bargain."
	case ratio == 0:
		return "Ratio is zero or unavailable — Yahoo has no enterprise value or EBITDA figure for this symbol."
	case ratio < 10:
		return "Below 10x: cheap relative to operating earnings by common rule-of-thumb standards, but compare against sector peers — capital-light or high-growth sectors often trade structurally higher."
	case ratio <= 15:
		return "Roughly 10x–15x: in the typical range for a stable, moderately valued business. Compare against sector peers for context."
	default:
		return "Above 15x: expensive relative to operating earnings — market is pricing in strong growth, or EBITDA is temporarily depressed. Compare against sector peers before reading as overvalued."
	}
}

// GetEVToEBITDA returns the enterprise-value-to-EBITDA ratio for a stock ticker.
func (c *Client) GetEVToEBITDA(ctx context.Context, ticker string) (*EVToEBITDA, error) {
	if c.crumb == "" {
		if err := c.fetchCrumb(ctx); err != nil {
			return nil, err
		}
	}

	ratio, err := c.fetchEVToEBITDA(ctx, ticker)
	if err != nil {
		return nil, err
	}
	if ratio == nil {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	return &EVToEBITDA{Symbol: ticker, Ratio: *ratio, Interpretation: describeEVToEBITDA(*ratio)}, nil
}

// fetchEVToEBITDA fetches the defaultKeyStatistics.enterpriseToEbitda value for a symbol (requires crumb).
func (c *Client) fetchEVToEBITDA(ctx context.Context, symbol string) (*float64, error) {
	u, err := url.Parse(fmt.Sprintf("%s/v10/finance/quoteSummary/%s", c.baseURL, symbol))
	if err != nil {
		return nil, fmt.Errorf("parsing url: %w", err)
	}
	q := u.Query()
	q.Set("modules", "defaultKeyStatistics")
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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}

	var payload struct {
		QuoteSummary struct {
			Result []struct {
				DefaultKeyStatistics struct {
					EnterpriseToEbitda struct {
						Raw float64 `json:"raw"`
					} `json:"enterpriseToEbitda"`
				} `json:"defaultKeyStatistics"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"quoteSummary"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	if payload.QuoteSummary.Error != nil {
		return nil, fmt.Errorf("%w: %v", ErrAPIError, payload.QuoteSummary.Error)
	}

	if len(payload.QuoteSummary.Result) == 0 {
		return nil, nil
	}

	ratio := payload.QuoteSummary.Result[0].DefaultKeyStatistics.EnterpriseToEbitda.Raw
	return &ratio, nil
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

// FetchMonthlyBar returns the closing price for a symbol in a given calendar month
// using the v8 chart endpoint — no crumb or consent flow required, works in Docker.
// Returns ErrNoData when Yahoo has no data for the requested period.
func (c *Client) FetchMonthlyBar(ctx context.Context, symbol string, year, month int) (float64, error) {
	period1 := time.Date(year, time.Month(month), 1, 0, 0, 0, 0, time.UTC).Unix()
	period2 := time.Date(year, time.Month(month+1), 1, 0, 0, 0, 0, time.UTC).Unix()

	rawURL := fmt.Sprintf(
		"%s/v8/finance/chart/%s?interval=1mo&period1=%d&period2=%d",
		c.v8BaseURL, symbol, period1, period2,
	)
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

	if resp.StatusCode == http.StatusNotFound {
		return 0, fmt.Errorf("%w: %s", ErrTickerNotFound, symbol)
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("%w: HTTP %d for %s", ErrAPIError, resp.StatusCode, symbol)
	}

	var payload struct {
		Chart struct {
			Result []struct {
				Indicators struct {
					Quote []struct {
						Close []float64 `json:"close"`
					} `json:"quote"`
				} `json:"indicators"`
			} `json:"result"`
			Error interface{} `json:"error"`
		} `json:"chart"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return 0, err
	}
	if payload.Chart.Error != nil {
		return 0, fmt.Errorf("%w: %v", ErrNoData, payload.Chart.Error)
	}
	if len(payload.Chart.Result) == 0 ||
		len(payload.Chart.Result[0].Indicators.Quote) == 0 ||
		len(payload.Chart.Result[0].Indicators.Quote[0].Close) == 0 {
		return 0, fmt.Errorf("%w: %s %d/%02d", ErrNoData, symbol, year, month)
	}
	cls := payload.Chart.Result[0].Indicators.Quote[0].Close[0]
	if cls == 0 {
		return 0, fmt.Errorf("%w: zero close for %s %d/%02d", ErrNoData, symbol, year, month)
	}
	return cls, nil
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
				FiftyTwoWeekHigh   float64 `json:"fiftyTwoWeekHigh"`
				FiftyTwoWeekLow    float64 `json:"fiftyTwoWeekLow"`
			} `json:"meta"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

// fetchChartMeta fetches the v8 chart meta block for a symbol (no crumb required).
func (c *Client) fetchChartMeta(ctx context.Context, symbol string) (*chartResponse, error) {
	rawURL := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=1d", c.v8BaseURL, symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrAPIError, resp.StatusCode)
	}

	var cr chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

func (c *Client) fetchOneChart(ctx context.Context, symbol string) (float64, error) {
	cr, err := c.fetchChartMeta(ctx, symbol)
	if err != nil {
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

// FiftyTwoWeekRange holds the 52-week high/low price range for a symbol.
type FiftyTwoWeekRange struct {
	Symbol  string  `json:"symbol"` // Yahoo Finance ticker
	High    float64 `json:"high"`
	Low     float64 `json:"low"`
	Current float64 `json:"current"` // regular market price
	Pct     float64 `json:"pct"`     // position of Current between Low (0) and High (1)
}

// FetchFiftyTwoWeekRange returns the 52-week high/low for a ticker using the v8 chart
// endpoint — no crumb or consent flow required. Forex pairs like "USD-EUR" are resolved automatically.
func (c *Client) FetchFiftyTwoWeekRange(ctx context.Context, ticker string) (*FiftyTwoWeekRange, error) {
	rng, err := c.doFetchFiftyTwoWeekRange(ctx, NormalizeTicker(ticker))
	if err != nil {
		return nil, err
	}

	// Attempt forex pair format for unrecognized symbols.
	if (rng == nil || (rng.High == 0 && rng.Low == 0)) && reForexPair.MatchString(ticker) {
		m := reForexPair.FindStringSubmatch(ticker)
		rng, err = c.doFetchFiftyTwoWeekRange(ctx, m[1]+m[2]+"=X")
		if err != nil {
			return nil, err
		}
	}

	if rng == nil || (rng.High == 0 && rng.Low == 0) {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	rng.Symbol = ticker
	return rng, nil
}

func (c *Client) doFetchFiftyTwoWeekRange(ctx context.Context, symbol string) (*FiftyTwoWeekRange, error) {
	cr, err := c.fetchChartMeta(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if len(cr.Chart.Result) == 0 {
		return nil, nil
	}
	meta := cr.Chart.Result[0].Meta
	rng := &FiftyTwoWeekRange{
		High:    meta.FiftyTwoWeekHigh,
		Low:     meta.FiftyTwoWeekLow,
		Current: meta.RegularMarketPrice,
	}
	if rng.High > rng.Low {
		rng.Pct = (rng.Current - rng.Low) / (rng.High - rng.Low)
		if rng.Pct < 0 {
			rng.Pct = 0
		} else if rng.Pct > 1 {
			rng.Pct = 1
		}
	}
	return rng, nil
}

// PerformanceReturns holds percentage price change over several look-back periods.
// A period is 0 when the symbol has no trading history that far back (e.g. a recent IPO).
type PerformanceReturns struct {
	Symbol    string  `json:"symbol"`    // Yahoo Finance ticker
	YTD       float64 `json:"ytd"`       // % change since Jan 1 of the current year
	OneYear   float64 `json:"oneYear"`   // % change over the trailing 1 year
	ThreeYear float64 `json:"threeYear"` // % change over the trailing 3 years
	FiveYear  float64 `json:"fiveYear"`  // % change over the trailing 5 years
}

type chartHistoryResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
			} `json:"meta"`
			Timestamp  []int64 `json:"timestamp"`
			Indicators struct {
				Quote []struct {
					Close []float64 `json:"close"`
				} `json:"quote"`
			} `json:"indicators"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

// fetchPriceHistory fetches 5 years of daily closes for a symbol (no crumb required).
func (c *Client) fetchPriceHistory(ctx context.Context, symbol string) (*chartHistoryResponse, error) {
	rawURL := fmt.Sprintf("%s/v8/finance/chart/%s?interval=1d&range=5y", c.v8BaseURL, symbol)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: HTTP %d", ErrAPIError, resp.StatusCode)
	}

	var cr chartHistoryResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return nil, err
	}
	return &cr, nil
}

// closeAtOrBefore returns the last non-zero close at or before target (unix seconds),
// assuming timestamps is sorted ascending.
// closeAtOrBefore compares by calendar date rather than exact timestamp: intraday
// trading timestamps (e.g. market open) can fall later in the day than target's
// time-of-day even when they land on the same or an earlier calendar date.
func closeAtOrBefore(timestamps []int64, closes []float64, target time.Time) (float64, bool) {
	targetDay := target.Truncate(24 * time.Hour)
	price, found := 0.0, false
	for i, ts := range timestamps {
		day := time.Unix(ts, 0).UTC().Truncate(24 * time.Hour)
		if day.After(targetDay) {
			break
		}
		if i < len(closes) && closes[i] != 0 {
			price, found = closes[i], true
		}
	}
	return price, found
}

// FetchPerformance returns YTD, 1-year, 3-year, and 5-year percentage price change for a
// ticker using the v8 chart endpoint — no crumb or consent flow required. Forex pairs like
// "USD-EUR" are resolved automatically. A period is 0 when history doesn't reach that far back.
func (c *Client) FetchPerformance(ctx context.Context, ticker string) (*PerformanceReturns, error) {
	perf, err := c.doFetchPerformance(ctx, NormalizeTicker(ticker))
	if err != nil {
		return nil, err
	}

	// Attempt forex pair format for unrecognized symbols.
	if perf == nil && reForexPair.MatchString(ticker) {
		m := reForexPair.FindStringSubmatch(ticker)
		perf, err = c.doFetchPerformance(ctx, m[1]+m[2]+"=X")
		if err != nil {
			return nil, err
		}
	}

	if perf == nil {
		return nil, fmt.Errorf("%w: %s", ErrTickerNotFound, ticker)
	}

	perf.Symbol = ticker
	return perf, nil
}

func (c *Client) doFetchPerformance(ctx context.Context, symbol string) (*PerformanceReturns, error) {
	cr, err := c.fetchPriceHistory(ctx, symbol)
	if err != nil {
		return nil, err
	}
	if len(cr.Chart.Result) == 0 {
		return nil, nil
	}
	result := cr.Chart.Result[0]
	if len(result.Indicators.Quote) == 0 || len(result.Timestamp) == 0 {
		return nil, nil
	}
	closes := result.Indicators.Quote[0].Close

	current := result.Meta.RegularMarketPrice
	if current == 0 {
		// fall back to the most recent close in the series
		for i := len(closes) - 1; i >= 0; i-- {
			if closes[i] != 0 {
				current = closes[i]
				break
			}
		}
	}
	if current == 0 {
		return nil, nil
	}

	now := time.Now().UTC()
	pctSince := func(target time.Time) float64 {
		start, found := closeAtOrBefore(result.Timestamp, closes, target)
		if !found || start == 0 {
			return 0
		}
		return (current - start) / start * 100
	}

	return &PerformanceReturns{
		YTD:       pctSince(time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)),
		OneYear:   pctSince(now.AddDate(-1, 0, 0)),
		ThreeYear: pctSince(now.AddDate(-3, 0, 0)),
		FiveYear:  pctSince(now.AddDate(-5, 0, 0)),
	}, nil
}
