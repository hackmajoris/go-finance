// Package justetf reads fund data from justETF profile pages (https://www.justetf.com),
// which cover European UCITS ETFs that Yahoo Finance often lacks data for.
package justetf

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

var (
	// ErrNotFound is returned when justETF has no profile (or no TER) for the ISIN.
	ErrNotFound = errors.New("not found")
	// ErrAPIError is returned when justETF responds with a non-200 status.
	ErrAPIError = errors.New("justetf error")
)

const (
	defaultBaseURL = "https://www.justetf.com"
	userAgent      = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36"
)

var reTER = regexp.MustCompile(`data-testid="tl_etf-basics_value_ter"[^>]*>\s*([\d.,]+)\s*%`)

// Option is a functional option for configuring a Client.
type Option func(*Client)

// Client fetches ETF profile data from justETF.
type Client struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a Client with optional overrides.
func New(opts ...Option) (*Client, error) {
	c := &Client{httpClient: http.DefaultClient, baseURL: defaultBaseURL}
	for _, o := range opts {
		o(c)
	}
	return c, nil
}

// WithHTTPClient overrides the default HTTP client.
func WithHTTPClient(hc *http.Client) Option {
	return func(c *Client) { c.httpClient = hc }
}

// WithBaseURL overrides the justETF base URL (useful for testing).
func WithBaseURL(u string) Option {
	return func(c *Client) { c.baseURL = u }
}

// ExpenseRatio holds an ETF's total expense ratio as published by justETF.
type ExpenseRatio struct {
	ISIN string  `json:"isin"` // fund share class ISIN
	TER  float64 `json:"ter"`  // annual expense ratio as a percentage of assets (0.19 = 0.19%)
}

// GetTER returns the total expense ratio for the ETF with the given ISIN.
func (c *Client) GetTER(ctx context.Context, isin string) (*ExpenseRatio, error) {
	u := fmt.Sprintf("%s/en/etf-profile.html?isin=%s", c.baseURL, url.QueryEscape(isin))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("building request: %w", err)
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: status %d", ErrAPIError, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Unknown ISINs redirect to the search page, which has no TER field.
	m := reTER.FindSubmatch(body)
	if m == nil {
		return nil, fmt.Errorf("%w: %s", ErrNotFound, isin)
	}
	ter, err := strconv.ParseFloat(strings.ReplaceAll(string(m[1]), ",", "."), 64)
	if err != nil {
		return nil, fmt.Errorf("parsing TER %q: %w", m[1], err)
	}
	return &ExpenseRatio{ISIN: isin, TER: ter}, nil
}
