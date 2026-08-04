package prices

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

var httpClient = &http.Client{Timeout: 10 * time.Second}

// FetchQuotes returns a map of symbol → current price.
// Uses the Yahoo Finance v8 chart endpoint (no crumb required), fetching in parallel.
func FetchQuotes(symbols []string) (map[string]float64, error) {
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
			price, err := fetchOne(NormalizeTicker(sym))
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
		out[NormalizeTicker(r.sym)] = r.price // also store normalized form
	}

	// Return partial results with a soft error summary
	if len(errs) > 0 && len(out) == 0 {
		return nil, fmt.Errorf("all price fetches failed: %s", strings.Join(errs[:min(3, len(errs))], "; "))
	}
	return out, nil
}

// NormalizeTicker converts broker tickers to Yahoo Finance format.
// e.g. "BRK B" → "BRK-B"
func NormalizeTicker(sym string) string {
	return strings.ReplaceAll(sym, " ", "-")
}

type chartResponse struct {
	Chart struct {
		Result []struct {
			Meta struct {
				RegularMarketPrice float64 `json:"regularMarketPrice"`
				Symbol             string  `json:"symbol"`
			} `json:"meta"`
		} `json:"result"`
		Error interface{} `json:"error"`
	} `json:"chart"`
}

func fetchOne(symbol string) (float64, error) {
	url := fmt.Sprintf(
		"https://query1.finance.yahoo.com/v8/finance/chart/%s?interval=1d&range=1d",
		symbol,
	)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/124.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return 0, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	var cr chartResponse
	if err := json.NewDecoder(resp.Body).Decode(&cr); err != nil {
		return 0, err
	}
	if len(cr.Chart.Result) == 0 {
		return 0, fmt.Errorf("no data")
	}
	price := cr.Chart.Result[0].Meta.RegularMarketPrice
	if price == 0 {
		return 0, fmt.Errorf("zero price")
	}
	return price, nil
}

// FetchFXRates returns spot rates for each currency relative to base (e.g. "USD").
// Uses Yahoo Finance tickers like "EURUSD=X". Currencies equal to base get rate 1.0.
func FetchFXRates(currencies []string, base string) (map[string]float64, error) {
	rates := map[string]float64{base: 1.0}
	var toFetch []string
	for _, c := range currencies {
		if c != "" && c != base {
			toFetch = append(toFetch, c)
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

	for _, c := range toFetch {
		wg.Add(1)
		go func(c string) {
			defer wg.Done()
			ticker := c + base + "=X"
			rate, err := fetchOne(ticker)
			ch <- result{c, rate, err}
		}(c)
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
