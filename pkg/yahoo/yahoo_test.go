package yahoo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func quotePayload(symbol string, price float64, currency string) interface{} {
	return map[string]interface{}{
		"quoteResponse": map[string]interface{}{
			"result": []map[string]interface{}{
				{"symbol": symbol, "regularMarketPrice": price, "currency": currency},
			},
			"error": nil,
		},
	}
}

// newTestClient creates a client pre-loaded with a crumb and pointed at srv,
// so unit tests bypass the cookie/crumb dance entirely.
func newTestClient(t *testing.T, srv *httptest.Server) *yahoo.Client {
	t.Helper()
	client, err := yahoo.New(
		yahoo.WithBaseURL(srv.URL),
		yahoo.WithCrumbURL(srv.URL+"/crumb"),
		yahoo.WithCrumb("test-crumb"),
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return client
}

func TestGetQuote(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ticker     string
		wantSymbol string
		wantPrice  float64
		wantCurr   string
		wantErr    error
		wantDecErr bool
	}{
		{
			name: "happy path stock",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(quotePayload("AAPL", 189.43, "USD"))
			},
			ticker:     "AAPL",
			wantSymbol: "AAPL",
			wantPrice:  189.43,
			wantCurr:   "USD",
		},
		{
			name: "happy path crypto",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(quotePayload("BTC-USD", 94234.56, "USD"))
			},
			ticker:     "BTC-USD",
			wantSymbol: "BTC-USD",
			wantPrice:  94234.56,
			wantCurr:   "USD",
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteResponse": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "forex fallback — empty result",
			handler: func(w http.ResponseWriter, r *http.Request) {
				sym := r.URL.Query().Get("symbols")
				if sym == "RONUSD=X" {
					_ = json.NewEncoder(w).Encode(quotePayload("RONUSD=X", 0.2234, "USD"))
				} else {
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"quoteResponse": map[string]interface{}{"result": []interface{}{}, "error": nil},
					})
				}
			},
			ticker:     "RON-USD",
			wantSymbol: "RON-USD",
			wantPrice:  0.2234,
			wantCurr:   "USD",
		},
		{
			name: "forex fallback — zero price",
			handler: func(w http.ResponseWriter, r *http.Request) {
				sym := r.URL.Query().Get("symbols")
				if sym == "USDEUR=X" {
					_ = json.NewEncoder(w).Encode(quotePayload("USDEUR=X", 0.9012, "EUR"))
				} else {
					_ = json.NewEncoder(w).Encode(quotePayload("USD-EUR", 0, "EUR"))
				}
			},
			ticker:     "USD-EUR",
			wantSymbol: "USD-EUR",
			wantPrice:  0.9012,
			wantCurr:   "EUR",
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
		{
			name: "malformed json",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte("{not valid json"))
			},
			ticker:     "AAPL",
			wantDecErr: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClient(t, srv)
			quote, err := client.GetQuote(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if tc.wantDecErr {
				if err == nil {
					t.Fatal("expected decode error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if quote.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", quote.Symbol, tc.wantSymbol)
			}
			if quote.Price != tc.wantPrice {
				t.Errorf("price: got %f, want %f", quote.Price, tc.wantPrice)
			}
			if quote.Currency != tc.wantCurr {
				t.Errorf("currency: got %q, want %q", quote.Currency, tc.wantCurr)
			}
		})
	}
}

func peQuotePayload(symbol string, trailingPE, forwardPE float64) interface{} {
	return map[string]interface{}{
		"quoteResponse": map[string]interface{}{
			"result": []map[string]interface{}{
				{"symbol": symbol, "trailingPE": trailingPE, "forwardPE": forwardPE},
			},
			"error": nil,
		},
	}
}

func TestGetPE(t *testing.T) {
	tests := []struct {
		name          string
		handler       http.HandlerFunc
		ticker        string
		wantSymbol    string
		wantPE        float64
		wantForwardPE float64
		wantErr       error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(peQuotePayload("AAPL", 34.71, 28.05))
			},
			ticker:        "AAPL",
			wantSymbol:    "AAPL",
			wantPE:        34.71,
			wantForwardPE: 28.05,
		},
		{
			name: "no trailing earnings — forward PE only",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(peQuotePayload("GROWCO", 0, 55.2))
			},
			ticker:        "GROWCO",
			wantSymbol:    "GROWCO",
			wantPE:        0,
			wantForwardPE: 55.2,
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteResponse": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "both PE fields zero — treated as unavailable",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(peQuotePayload("XYZ", 0, 0))
			},
			ticker:  "XYZ",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClient(t, srv)
			got, err := client.GetPE(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.ForwardPE != tc.wantForwardPE {
				t.Errorf("forwardPE: got %f, want %f", got.ForwardPE, tc.wantForwardPE)
			}
			if got.PE != tc.wantPE {
				t.Errorf("pe: got %f, want %f", got.PE, tc.wantPE)
			}
			if got.Interpretation == "" {
				t.Error("interpretation: got empty string, want a non-empty explanation")
			}
		})
	}
}

func fcfPayload(freeCashflow float64) interface{} {
	return map[string]interface{}{
		"quoteSummary": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"financialData": map[string]interface{}{
						"freeCashflow": map[string]interface{}{"raw": freeCashflow},
					},
				},
			},
			"error": nil,
		},
	}
}

func TestGetFreeCashFlow(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ticker     string
		wantSymbol string
		wantFCF    float64
		wantErr    error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(fcfPayload(1.1e11))
			},
			ticker:     "AAPL",
			wantSymbol: "AAPL",
			wantFCF:    1.1e11,
		},
		{
			name: "negative free cash flow",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(fcfPayload(-5.2e9))
			},
			ticker:     "BURNCO",
			wantSymbol: "BURNCO",
			wantFCF:    -5.2e9,
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteSummary": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "404 not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			ticker:  "INVALID",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClient(t, srv)
			got, err := client.GetFreeCashFlow(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.FCF != tc.wantFCF {
				t.Errorf("fcf: got %f, want %f", got.FCF, tc.wantFCF)
			}
			if got.Interpretation == "" {
				t.Error("interpretation: got empty string, want a non-empty explanation")
			}
		})
	}
}

func cashFlowQualityPayload(operatingCashflow, netIncome float64) interface{} {
	return map[string]interface{}{
		"quoteSummary": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"financialData": map[string]interface{}{
						"operatingCashflow": map[string]interface{}{"raw": operatingCashflow},
					},
					"defaultKeyStatistics": map[string]interface{}{
						"netIncomeToCommon": map[string]interface{}{"raw": netIncome},
					},
				},
			},
			"error": nil,
		},
	}
}

func TestGetOperatingCashFlowVsNetIncome(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ticker     string
		wantSymbol string
		wantOCF    float64
		wantNI     float64
		wantRatio  float64
		wantErr    error
	}{
		{
			name: "happy path — cash-backed earnings",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(cashFlowQualityPayload(1.2e11, 1.0e11))
			},
			ticker:     "AAPL",
			wantSymbol: "AAPL",
			wantOCF:    1.2e11,
			wantNI:     1.0e11,
			wantRatio:  1.2,
		},
		{
			name: "net income negative — ratio negative",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(cashFlowQualityPayload(2.0e9, -1.0e9))
			},
			ticker:     "LOSSCO",
			wantSymbol: "LOSSCO",
			wantOCF:    2.0e9,
			wantNI:     -1.0e9,
			wantRatio:  -2.0,
		},
		{
			name: "net income zero — ratio stays zero",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(cashFlowQualityPayload(5.0e8, 0))
			},
			ticker:     "BREAKEVEN",
			wantSymbol: "BREAKEVEN",
			wantOCF:    5.0e8,
			wantNI:     0,
			wantRatio:  0,
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteSummary": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "404 not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			ticker:  "INVALID",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClient(t, srv)
			got, err := client.GetOperatingCashFlowVsNetIncome(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.OperatingCashFlow != tc.wantOCF {
				t.Errorf("operatingCashFlow: got %f, want %f", got.OperatingCashFlow, tc.wantOCF)
			}
			if got.NetIncome != tc.wantNI {
				t.Errorf("netIncome: got %f, want %f", got.NetIncome, tc.wantNI)
			}
			if diff := got.Ratio - tc.wantRatio; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("ratio: got %f, want %f", got.Ratio, tc.wantRatio)
			}
			if got.Interpretation == "" {
				t.Error("interpretation: got empty string, want a non-empty explanation")
			}
		})
	}
}

func debtToEquityPayload(ratio float64) interface{} {
	return map[string]interface{}{
		"quoteSummary": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"financialData": map[string]interface{}{
						"debtToEquity": map[string]interface{}{"raw": ratio},
					},
				},
			},
			"error": nil,
		},
	}
}

func TestGetDebtToEquity(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ticker     string
		wantSymbol string
		wantRatio  float64
		wantErr    error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(debtToEquityPayload(150.5))
			},
			ticker:     "AAPL",
			wantSymbol: "AAPL",
			wantRatio:  150.5,
		},
		{
			name: "zero debt — legitimate net-cash company, not an error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(debtToEquityPayload(0))
			},
			ticker:     "DEBTFREE",
			wantSymbol: "DEBTFREE",
			wantRatio:  0,
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteSummary": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "404 not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			ticker:  "INVALID",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClient(t, srv)
			got, err := client.GetDebtToEquity(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.Ratio != tc.wantRatio {
				t.Errorf("ratio: got %f, want %f", got.Ratio, tc.wantRatio)
			}
			if got.Interpretation == "" {
				t.Error("interpretation: got empty string, want a non-empty explanation")
			}
		})
	}
}

func evToEBITDAPayload(ratio float64) interface{} {
	return map[string]interface{}{
		"quoteSummary": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"defaultKeyStatistics": map[string]interface{}{
						"enterpriseToEbitda": map[string]interface{}{"raw": ratio},
					},
				},
			},
			"error": nil,
		},
	}
}

func TestGetEVToEBITDA(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		ticker     string
		wantSymbol string
		wantRatio  float64
		wantErr    error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(evToEBITDAPayload(12.4))
			},
			ticker:     "AAPL",
			wantSymbol: "AAPL",
			wantRatio:  12.4,
		},
		{
			name: "negative EBITDA — negative ratio, not an error",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(evToEBITDAPayload(-8.1))
			},
			ticker:     "BURNCO",
			wantSymbol: "BURNCO",
			wantRatio:  -8.1,
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteSummary": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "404 not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			ticker:  "INVALID",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClient(t, srv)
			got, err := client.GetEVToEBITDA(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.Ratio != tc.wantRatio {
				t.Errorf("ratio: got %f, want %f", got.Ratio, tc.wantRatio)
			}
			if got.Interpretation == "" {
				t.Error("interpretation: got empty string, want a non-empty explanation")
			}
		})
	}
}

// newTestClientV8 creates a client pointed at srv for v8 endpoint tests.
func newTestClientV8(t *testing.T, srv *httptest.Server) *yahoo.Client {
	t.Helper()
	client, err := yahoo.New(
		yahoo.WithV8BaseURL(srv.URL),
		yahoo.WithCrumb("test-crumb"),
	)
	if err != nil {
		t.Fatalf("New(): %v", err)
	}
	return client
}

func chartPayload(closePrice float64) interface{} {
	return map[string]interface{}{
		"chart": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"indicators": map[string]interface{}{
						"quote": []map[string]interface{}{
							{"close": []float64{closePrice}},
						},
					},
				},
			},
			"error": nil,
		},
	}
}

func TestFetchMonthlyBar(t *testing.T) {
	tests := []struct {
		name      string
		handler   http.HandlerFunc
		symbol    string
		year      int
		month     int
		wantClose float64
		wantErr   error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(chartPayload(5218.19))
			},
			symbol:    "^GSPC",
			year:      2024,
			month:     3,
			wantClose: 5218.19,
		},
		{
			name: "empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"chart": map[string]interface{}{
						"result": []interface{}{},
						"error":  nil,
					},
				})
			},
			symbol:  "^GSPC",
			year:    2024,
			month:   3,
			wantErr: yahoo.ErrNoData,
		},
		{
			name: "zero close treated as no data",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(chartPayload(0))
			},
			symbol:  "^GSPC",
			year:    2024,
			month:   3,
			wantErr: yahoo.ErrNoData,
		},
		{
			name: "404 not found",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNotFound)
			},
			symbol:  "INVALID",
			year:    2024,
			month:   3,
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			symbol:  "^GSPC",
			year:    2024,
			month:   3,
			wantErr: yahoo.ErrAPIError,
		},
		{
			name: "url contains symbol and period params",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/v8/finance/chart/^IXIC" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if r.URL.Query().Get("interval") != "1mo" {
					w.WriteHeader(http.StatusBadRequest)
					return
				}
				_ = json.NewEncoder(w).Encode(chartPayload(18003.21))
			},
			symbol:    "^IXIC",
			year:      2024,
			month:     6,
			wantClose: 18003.21,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClientV8(t, srv)
			got, err := client.FetchMonthlyBar(context.Background(), tc.symbol, tc.year, tc.month)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantClose {
				t.Errorf("close: got %f, want %f", got, tc.wantClose)
			}
		})
	}
}

func rangePayload(high, low, current float64) interface{} {
	return map[string]interface{}{
		"chart": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"meta": map[string]interface{}{
						"fiftyTwoWeekHigh":   high,
						"fiftyTwoWeekLow":    low,
						"regularMarketPrice": current,
					},
				},
			},
			"error": nil,
		},
	}
}

func TestFetchFiftyTwoWeekRange(t *testing.T) {
	tests := []struct {
		name        string
		handler     http.HandlerFunc
		ticker      string
		wantSymbol  string
		wantHigh    float64
		wantLow     float64
		wantCurrent float64
		wantPct     float64
		wantErr     error
	}{
		{
			name: "happy path stock",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(rangePayload(260.10, 164.08, 212.09))
			},
			ticker:      "AAPL",
			wantSymbol:  "AAPL",
			wantHigh:    260.10,
			wantLow:     164.08,
			wantCurrent: 212.09,
			wantPct:     0.5, // (212.09-164.08)/(260.10-164.08) ≈ 0.5
		},
		{
			name: "current at low — pct 0",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(rangePayload(260.10, 164.08, 164.08))
			},
			ticker:      "AAPL",
			wantSymbol:  "AAPL",
			wantHigh:    260.10,
			wantLow:     164.08,
			wantCurrent: 164.08,
			wantPct:     0,
		},
		{
			name: "forex fallback — empty result",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/v8/finance/chart/RONUSD=X" {
					_ = json.NewEncoder(w).Encode(rangePayload(0.235, 0.201, 0.218))
				} else {
					_ = json.NewEncoder(w).Encode(map[string]interface{}{
						"chart": map[string]interface{}{"result": []interface{}{}, "error": nil},
					})
				}
			},
			ticker:      "RON-USD",
			wantSymbol:  "RON-USD",
			wantHigh:    0.235,
			wantLow:     0.201,
			wantCurrent: 0.218,
			wantPct:     0.5,
		},
		{
			name: "ticker not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"chart": map[string]interface{}{"result": []interface{}{}, "error": nil},
				})
			},
			ticker:  "UNKNOWN",
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name: "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusInternalServerError)
			},
			ticker:  "AAPL",
			wantErr: yahoo.ErrAPIError,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()

			client := newTestClientV8(t, srv)
			got, err := client.FetchFiftyTwoWeekRange(context.Background(), tc.ticker)

			if tc.wantErr != nil {
				if err == nil {
					t.Fatalf("expected error wrapping %v, got nil", tc.wantErr)
				}
				if !errIs(err, tc.wantErr) {
					t.Fatalf("expected error %v, got %v", tc.wantErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Symbol != tc.wantSymbol {
				t.Errorf("symbol: got %q, want %q", got.Symbol, tc.wantSymbol)
			}
			if got.High != tc.wantHigh {
				t.Errorf("high: got %f, want %f", got.High, tc.wantHigh)
			}
			if got.Low != tc.wantLow {
				t.Errorf("low: got %f, want %f", got.Low, tc.wantLow)
			}
			if got.Current != tc.wantCurrent {
				t.Errorf("current: got %f, want %f", got.Current, tc.wantCurrent)
			}
			if diff := got.Pct - tc.wantPct; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("pct: got %f, want %f", got.Pct, tc.wantPct)
			}
		})
	}
}

func historyPayload(current float64, points map[time.Time]float64) interface{} {
	// sort by time ascending
	times := make([]time.Time, 0, len(points))
	for t := range points {
		times = append(times, t)
	}
	for i := 1; i < len(times); i++ {
		for j := i; j > 0 && times[j].Before(times[j-1]); j-- {
			times[j], times[j-1] = times[j-1], times[j]
		}
	}

	timestamps := make([]int64, len(times))
	closes := make([]float64, len(times))
	for i, t := range times {
		timestamps[i] = t.Unix()
		closes[i] = points[t]
	}

	return map[string]interface{}{
		"chart": map[string]interface{}{
			"result": []map[string]interface{}{
				{
					"meta":      map[string]interface{}{"regularMarketPrice": current},
					"timestamp": timestamps,
					"indicators": map[string]interface{}{
						"quote": []map[string]interface{}{
							{"close": closes},
						},
					},
				},
			},
			"error": nil,
		},
	}
}

func TestFetchPerformance(t *testing.T) {
	now := time.Now().UTC()
	ytdTarget := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, time.UTC)
	oneYearTarget := now.AddDate(-1, 0, 0)
	threeYearTarget := now.AddDate(-3, 0, 0)
	fiveYearTarget := now.AddDate(-5, 0, 0)

	t.Run("happy path", func(t *testing.T) {
		current, five, three, one, ytd := 120.0, 50.0, 80.0, 90.0, 95.0
		payload := historyPayload(current, map[time.Time]float64{
			fiveYearTarget:  five,
			threeYearTarget: three,
			oneYearTarget:   one,
			ytdTarget:       ytd,
		})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(payload)
		}))
		defer srv.Close()

		client := newTestClientV8(t, srv)
		got, err := client.FetchPerformance(context.Background(), "AAPL")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if got.Symbol != "AAPL" {
			t.Errorf("symbol: got %q, want %q", got.Symbol, "AAPL")
		}
		// computed via variables (not constants) to match the runtime float64 rounding the library does
		wantYTD := (current - ytd) / ytd * 100
		wantOneYear := (current - one) / one * 100
		wantThreeYear := (current - three) / three * 100
		wantFiveYear := (current - five) / five * 100
		if got.YTD != wantYTD {
			t.Errorf("ytd: got %f, want %f", got.YTD, wantYTD)
		}
		if got.OneYear != wantOneYear {
			t.Errorf("oneYear: got %f, want %f", got.OneYear, wantOneYear)
		}
		if got.ThreeYear != wantThreeYear {
			t.Errorf("threeYear: got %f, want %f", got.ThreeYear, wantThreeYear)
		}
		if got.FiveYear != wantFiveYear {
			t.Errorf("fiveYear: got %f, want %f", got.FiveYear, wantFiveYear)
		}
	})

	t.Run("short history — older periods stay zero", func(t *testing.T) {
		payload := historyPayload(120, map[time.Time]float64{
			oneYearTarget: 90,
			ytdTarget:     95,
		})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(payload)
		}))
		defer srv.Close()

		client := newTestClientV8(t, srv)
		got, err := client.FetchPerformance(context.Background(), "RECENTIPO")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ThreeYear != 0 {
			t.Errorf("threeYear: got %f, want 0 (no history that far back)", got.ThreeYear)
		}
		if got.FiveYear != 0 {
			t.Errorf("fiveYear: got %f, want 0 (no history that far back)", got.FiveYear)
		}
		if got.OneYear == 0 {
			t.Errorf("oneYear: got 0, want a non-zero computed value")
		}
	})

	t.Run("ticker not found — empty result", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"chart": map[string]interface{}{"result": []interface{}{}, "error": nil},
			})
		}))
		defer srv.Close()

		client := newTestClientV8(t, srv)
		_, err := client.FetchPerformance(context.Background(), "UNKNOWN")
		if !errIs(err, yahoo.ErrTickerNotFound) {
			t.Fatalf("expected error %v, got %v", yahoo.ErrTickerNotFound, err)
		}
	})

	t.Run("http error status", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		client := newTestClientV8(t, srv)
		_, err := client.FetchPerformance(context.Background(), "AAPL")
		if !errIs(err, yahoo.ErrAPIError) {
			t.Fatalf("expected error %v, got %v", yahoo.ErrAPIError, err)
		}
	})
}

func errIs(got, target error) bool {
	for got != nil {
		if got == target {
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := got.(unwrapper)
		if !ok {
			break
		}
		got = u.Unwrap()
	}
	return false
}
