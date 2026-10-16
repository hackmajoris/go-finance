package yahoo_test

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

// summaryResult wraps a quoteSummary result[0] body (module → fields) in the standard envelope.
func summaryResult(modules map[string]interface{}) interface{} {
	return map[string]interface{}{
		"quoteSummary": map[string]interface{}{
			"result": []map[string]interface{}{modules},
			"error":  nil,
		},
	}
}

// raw builds Yahoo's {"raw": v} field wrapper.
func raw(v float64) map[string]interface{} { return map[string]interface{}{"raw": v} }

// emptySummary is the "no data for this symbol" response (empty result slice).
func emptySummary() interface{} {
	return map[string]interface{}{
		"quoteSummary": map[string]interface{}{"result": []interface{}{}, "error": nil},
	}
}

func TestGetMarketCap(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    float64
		wantErr error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
					"summaryDetail": map[string]interface{}{"marketCap": raw(3.0e12)},
				}))
			},
			want: 3.0e12,
		},
		{
			name:    "not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(emptySummary()) },
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name:    "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: yahoo.ErrAPIError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			got, err := newTestClient(t, srv).GetMarketCap(context.Background(), "AAPL")
			if !checkErr(t, err, tc.wantErr) {
				return
			}
			if got.MarketCap != tc.want {
				t.Errorf("marketCap: got %f, want %f", got.MarketCap, tc.want)
			}
			assertMeta(t, got.Symbol, got.Interpretation)
		})
	}
}

func TestGetExpenseRatio(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    float64
		wantErr error
	}{
		{
			name: "fund profile preferred over key statistics",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
					"fundProfile":          map[string]interface{}{"feesExpensesInvestment": map[string]interface{}{"annualReportExpenseRatio": raw(0.0022)}},
					"defaultKeyStatistics": map[string]interface{}{"annualReportExpenseRatio": raw(0.0099)},
				}))
			},
			want: 0.22,
		},
		{
			name: "falls back to key statistics",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
					"defaultKeyStatistics": map[string]interface{}{"annualReportExpenseRatio": raw(0.0003)},
				}))
			},
			want: 0.03,
		},
		{
			name: "no expense ratio reported",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{}))
			},
			want: 0,
		},
		{
			name:    "not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(emptySummary()) },
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name:    "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: yahoo.ErrAPIError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			got, err := newTestClient(t, srv).GetExpenseRatio(context.Background(), "VWCE.DE")
			if !checkErr(t, err, tc.wantErr) {
				return
			}
			if math.Abs(got.TER-tc.want) > 1e-9 {
				t.Errorf("TER: got %f, want %f", got.TER, tc.want)
			}
			if got.Symbol != "VWCE.DE" || got.Interpretation == "" {
				t.Errorf("meta: symbol %q, interpretation %q", got.Symbol, got.Interpretation)
			}
		})
	}
}

func TestGetTopHoldings(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    []yahoo.Holding
		wantErr error
	}{
		{
			// Order matters: the summary table lists holdings as ranked by Yahoo (largest first).
			name: "weights as percentages, order preserved",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
					"topHoldings": map[string]interface{}{"holdings": []map[string]interface{}{
						{"symbol": "NVDA", "holdingName": "NVIDIA Corp", "holdingPercent": raw(0.0808)},
						{"symbol": "AAPL", "holdingName": "Apple Inc", "holdingPercent": raw(0.0703)},
					}},
				}))
			},
			want: []yahoo.Holding{
				{Symbol: "NVDA", Name: "NVIDIA Corp", Weight: 8.08},
				{Symbol: "AAPL", Name: "Apple Inc", Weight: 7.03},
			},
		},
		{
			name: "no holdings reported",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{}))
			},
			want: []yahoo.Holding{},
		},
		{
			name:    "not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(emptySummary()) },
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name:    "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: yahoo.ErrAPIError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			got, err := newTestClient(t, srv).GetTopHoldings(context.Background(), "VOO")
			if !checkErr(t, err, tc.wantErr) {
				return
			}
			if got.Symbol != "VOO" {
				t.Errorf("symbol: got %q, want %q", got.Symbol, "VOO")
			}
			if len(got.Holdings) != len(tc.want) {
				t.Fatalf("holdings: got %d, want %d", len(got.Holdings), len(tc.want))
			}
			for i, h := range got.Holdings {
				if h.Symbol != tc.want[i].Symbol || h.Name != tc.want[i].Name || math.Abs(h.Weight-tc.want[i].Weight) > 1e-9 {
					t.Errorf("holding %d: got %+v, want %+v", i, h, tc.want[i])
				}
			}
		})
	}
}

func TestGetSector(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    string
		wantErr error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
					"assetProfile": map[string]interface{}{"sector": "Technology"},
				}))
			},
			want: "Technology",
		},
		{
			name:    "not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(emptySummary()) },
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name:    "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: yahoo.ErrAPIError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			got, err := newTestClient(t, srv).GetSector(context.Background(), "AAPL")
			if !checkErr(t, err, tc.wantErr) {
				return
			}
			if got.Sector != tc.want {
				t.Errorf("sector: got %q, want %q", got.Sector, tc.want)
			}
			if got.Symbol != "AAPL" {
				t.Errorf("symbol: got %q, want %q", got.Symbol, "AAPL")
			}
		})
	}
}

func TestGetSectorBenchmark(t *testing.T) {
	peerValuation := map[string]map[string]interface{}{
		"PEER1": {
			"summaryDetail":        map[string]interface{}{"marketCap": raw(1000)},
			"defaultKeyStatistics": map[string]interface{}{"enterpriseValue": raw(1200), "netIncomeToCommon": raw(100)},
			"financialData":        map[string]interface{}{"ebitda": raw(150)},
		},
		"PEER2": {
			"summaryDetail":        map[string]interface{}{"marketCap": raw(3000)},
			"defaultKeyStatistics": map[string]interface{}{"enterpriseValue": raw(3600), "netIncomeToCommon": raw(300)},
			"financialData":        map[string]interface{}{"ebitda": raw(450)},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v7/finance/quote":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"quoteResponse": map[string]interface{}{
					"result": []map[string]interface{}{{"symbol": "AAPL", "trailingPE": 20.0, "forwardPE": 22.0}},
					"error":  nil,
				},
			})
		case r.URL.Path == "/v1/finance/sectors/technology":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"topCompanies": []map[string]interface{}{{"symbol": "PEER1"}, {"symbol": "PEER2"}},
				},
			})
		case r.URL.Path == "/v10/finance/quoteSummary/AAPL" && r.URL.Query().Get("modules") == "assetProfile":
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"assetProfile": map[string]interface{}{"sector": "Technology"},
			}))
		case r.URL.Path == "/v10/finance/quoteSummary/AAPL" && r.URL.Query().Get("modules") == "defaultKeyStatistics":
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"defaultKeyStatistics": map[string]interface{}{"enterpriseToEbitda": raw(25)},
			}))
		default:
			symbol := strings.TrimPrefix(r.URL.Path, "/v10/finance/quoteSummary/")
			modules, ok := peerValuation[symbol]
			if !ok {
				_ = json.NewEncoder(w).Encode(emptySummary())
				return
			}
			_ = json.NewEncoder(w).Encode(summaryResult(modules))
		}
	}))
	defer srv.Close()

	got, err := newTestClient(t, srv).GetSectorBenchmark(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got.Sector != "Technology" {
		t.Errorf("sector: got %q, want %q", got.Sector, "Technology")
	}
	if got.PeerCount != 2 {
		t.Errorf("peerCount: got %d, want 2", got.PeerCount)
	}
	if got.PE != 20 {
		t.Errorf("PE: got %f, want 20", got.PE)
	}
	if got.SectorPE != 10 {
		t.Errorf("SectorPE: got %f, want 10", got.SectorPE)
	}
	if got.PEVsSectorPercent != 100 {
		t.Errorf("PEVsSectorPercent: got %f, want 100", got.PEVsSectorPercent)
	}
	if got.EVToEBITDA != 25 {
		t.Errorf("EVToEBITDA: got %f, want 25", got.EVToEBITDA)
	}
	if got.SectorEVToEBITDA != 8 {
		t.Errorf("SectorEVToEBITDA: got %f, want 8", got.SectorEVToEBITDA)
	}
	if got.EVToEBITDAVsSectorPercent != 212.5 {
		t.Errorf("EVToEBITDAVsSectorPercent: got %f, want 212.5", got.EVToEBITDAVsSectorPercent)
	}
}

func TestGetAnalystRating(t *testing.T) {
	tests := []struct {
		name       string
		handler    http.HandlerFunc
		wantScore  float64
		wantRating string
		wantErr    error
	}{
		{
			name: "happy path",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteResponse": map[string]interface{}{
						"result": []map[string]interface{}{
							{"symbol": "AAPL", "averageAnalystRating": "2.2 - Buy"},
						},
						"error": nil,
					},
				})
			},
			wantScore:  2.2,
			wantRating: "Buy",
		},
		{
			name: "no analyst coverage — ticker exists, rating field empty",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteResponse": map[string]interface{}{
						"result": []map[string]interface{}{
							{"symbol": "ZIM", "averageAnalystRating": ""},
						},
						"error": nil,
					},
				})
			},
			wantScore:  0,
			wantRating: "",
		},
		{
			name: "not found — empty result",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(map[string]interface{}{
					"quoteResponse": map[string]interface{}{"result": []interface{}{}, "error": nil},
				})
			},
			wantErr: yahoo.ErrTickerNotFound,
		},
		{
			name:    "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) },
			wantErr: yahoo.ErrAPIError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			got, err := newTestClient(t, srv).GetAnalystRating(context.Background(), "AAPL")
			if !checkErr(t, err, tc.wantErr) {
				return
			}
			if got.Score != tc.wantScore {
				t.Errorf("score: got %f, want %f", got.Score, tc.wantScore)
			}
			if got.Rating != tc.wantRating {
				t.Errorf("rating: got %q, want %q", got.Rating, tc.wantRating)
			}
		})
	}
}

func TestGetSectorBenchmark_NoPeerData(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v7/finance/quote":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"quoteResponse": map[string]interface{}{
					"result": []map[string]interface{}{{"symbol": "AAPL", "trailingPE": 20.0, "forwardPE": 22.0}},
					"error":  nil,
				},
			})
		case r.URL.Path == "/v1/finance/sectors/technology":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{
					"topCompanies": []map[string]interface{}{{"symbol": "PEER1"}},
				},
			})
		case r.URL.Path == "/v10/finance/quoteSummary/AAPL" && r.URL.Query().Get("modules") == "assetProfile":
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"assetProfile": map[string]interface{}{"sector": "Technology"},
			}))
		case r.URL.Path == "/v10/finance/quoteSummary/AAPL" && r.URL.Query().Get("modules") == "defaultKeyStatistics":
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"defaultKeyStatistics": map[string]interface{}{"enterpriseToEbitda": raw(25)},
			}))
		default:
			_ = json.NewEncoder(w).Encode(emptySummary())
		}
	}))
	defer srv.Close()

	_, err := newTestClient(t, srv).GetSectorBenchmark(context.Background(), "AAPL")
	checkErr(t, err, yahoo.ErrAPIError)
}

func TestGetPriceToSales(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"summaryDetail": map[string]interface{}{"priceToSalesTrailing12Months": raw(7.5)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetPriceToSales(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Ratio != 7.5 {
		t.Errorf("ratio: got %f, want 7.5", got.Ratio)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetPriceToBook(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"defaultKeyStatistics": map[string]interface{}{"priceToBook": raw(45.2)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetPriceToBook(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Ratio != 45.2 {
		t.Errorf("ratio: got %f, want 45.2", got.Ratio)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetFreeCashFlowYield(t *testing.T) {
	tests := []struct {
		name      string
		fcf, mc   float64
		wantYield float64
	}{
		{name: "positive yield", fcf: 1.0e11, mc: 2.0e12, wantYield: 5},
		{name: "negative yield", fcf: -5.0e9, mc: 1.0e11, wantYield: -5},
		// zero market cap must not panic on division; yield stays 0
		{name: "zero market cap guards division", fcf: 1.0e11, mc: 0, wantYield: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
					"financialData": map[string]interface{}{"freeCashflow": raw(tc.fcf)},
					"summaryDetail": map[string]interface{}{"marketCap": raw(tc.mc)},
				}))
			}))
			defer srv.Close()
			got, err := newTestClient(t, srv).GetFreeCashFlowYield(context.Background(), "AAPL")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if diff := got.Yield - tc.wantYield; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("yield: got %f, want %f", got.Yield, tc.wantYield)
			}
			assertMeta(t, got.Symbol, got.Interpretation)
		})
	}
}

func TestGetProfitMargin(t *testing.T) {
	// Yahoo reports a fraction (0.25); the client must scale it to a percentage (25%).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"financialData": map[string]interface{}{"profitMargins": raw(0.2534)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetProfitMargin(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := got.Margin - 25.34; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("margin: got %f, want 25.34", got.Margin)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetOperatingMargin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"financialData": map[string]interface{}{"operatingMargins": raw(0.31)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetOperatingMargin(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := got.Margin - 31.0; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("margin: got %f, want 31", got.Margin)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetQuarterlyEarningsGrowth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"defaultKeyStatistics": map[string]interface{}{"earningsQuarterlyGrowth": raw(-0.12)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetQuarterlyEarningsGrowth(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := got.Growth - (-12.0); diff > 1e-9 || diff < -1e-9 {
		t.Errorf("growth: got %f, want -12", got.Growth)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetQuarterlyRevenueGrowth(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"financialData": map[string]interface{}{"revenueGrowth": raw(0.08)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetQuarterlyRevenueGrowth(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := got.Growth - 8.0; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("growth: got %f, want 8", got.Growth)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetCashAndDebt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"financialData": map[string]interface{}{
				"totalCash": raw(6.5e10),
				"totalDebt": raw(1.1e11),
			},
		}))
	}))
	defer srv.Close()
	client := newTestClient(t, srv)

	cash, err := client.GetCash(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetCash: %v", err)
	}
	if cash.Cash != 6.5e10 {
		t.Errorf("cash: got %f, want 6.5e10", cash.Cash)
	}
	assertMeta(t, cash.Symbol, cash.Interpretation)

	debt, err := client.GetDebt(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("GetDebt: %v", err)
	}
	if debt.Debt != 1.1e11 {
		t.Errorf("debt: got %f, want 1.1e11", debt.Debt)
	}
	assertMeta(t, debt.Symbol, debt.Interpretation)
}

func TestGetDividendYield(t *testing.T) {
	tests := []struct {
		name     string
		body     interface{}
		wantYld  float64
		wantErr  error
		wantNote string
	}{
		{
			name: "dividend payer",
			body: summaryResult(map[string]interface{}{
				"summaryDetail": map[string]interface{}{"dividendYield": raw(0.0052)},
			}),
			wantYld: 0.52,
		},
		{
			// a present result with no dividend field is a non-payer, NOT a not-found error
			name: "non-payer — zero yield is valid, not an error",
			body: summaryResult(map[string]interface{}{
				"summaryDetail": map[string]interface{}{},
			}),
			wantYld: 0,
		},
		{
			name:    "not found — empty result",
			body:    emptySummary(),
			wantErr: yahoo.ErrTickerNotFound,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer srv.Close()
			got, err := newTestClient(t, srv).GetDividendYield(context.Background(), "AAPL")
			if !checkErr(t, err, tc.wantErr) {
				return
			}
			if diff := got.Yield - tc.wantYld; diff > 1e-9 || diff < -1e-9 {
				t.Errorf("yield: got %f, want %f", got.Yield, tc.wantYld)
			}
			assertMeta(t, got.Symbol, got.Interpretation)
		})
	}
}

func TestGetPayoutRatio(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"summaryDetail": map[string]interface{}{"payoutRatio": raw(0.152)},
		}))
	}))
	defer srv.Close()
	got, err := newTestClient(t, srv).GetPayoutRatio(context.Background(), "AAPL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if diff := got.Ratio - 15.2; diff > 1e-9 || diff < -1e-9 {
		t.Errorf("ratio: got %f, want 15.2", got.Ratio)
	}
	assertMeta(t, got.Symbol, got.Interpretation)
}

func TestGetPayoutDate(t *testing.T) {
	want := time.Date(2025, 8, 14, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name     string
		body     interface{}
		wantZero bool
	}{
		{
			name: "has payment date",
			body: summaryResult(map[string]interface{}{
				"calendarEvents": map[string]interface{}{"dividendDate": raw(float64(want.Unix()))},
			}),
		},
		{
			// non-payer: missing date must decode to the zero time, not an error
			name:     "no payment date — zero time, not an error",
			body:     summaryResult(map[string]interface{}{"calendarEvents": map[string]interface{}{}}),
			wantZero: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_ = json.NewEncoder(w).Encode(tc.body)
			}))
			defer srv.Close()
			got, err := newTestClient(t, srv).GetPayoutDate(context.Background(), "AAPL")
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.wantZero {
				if !got.Date.IsZero() {
					t.Errorf("date: got %v, want zero time", got.Date)
				}
			} else if !got.Date.Equal(want) {
				t.Errorf("date: got %v, want %v", got.Date, want)
			}
			assertMeta(t, got.Symbol, got.Interpretation)
		})
	}
}

// checkErr asserts the error against wantErr. It returns false (stop the test) when an
// error was expected — so happy-path assertions only run when wantErr is nil.
func checkErr(t *testing.T, err, wantErr error) bool {
	t.Helper()
	if wantErr != nil {
		if err == nil {
			t.Fatalf("expected error wrapping %v, got nil", wantErr)
		}
		if !errIs(err, wantErr) {
			t.Fatalf("expected error %v, got %v", wantErr, err)
		}
		return false
	}
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return true
}

func assertMeta(t *testing.T, symbol, interpretation string) {
	t.Helper()
	if symbol != "AAPL" {
		t.Errorf("symbol: got %q, want %q", symbol, "AAPL")
	}
	if interpretation == "" {
		t.Error("interpretation: got empty string, want a non-empty explanation")
	}
}

// Sector, PE and EV/EBITDA are independent lookups; running them one after another
// put three round-trips on the critical path of every benchmark (and the summary CLI).
func TestGetSectorBenchmark_FetchesIndependentInputsConcurrently(t *testing.T) {
	const delay = 100 * time.Millisecond
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		modules := r.URL.Query().Get("modules")
		switch {
		case r.URL.Path == "/v7/finance/quote":
			time.Sleep(delay)
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"quoteResponse": map[string]interface{}{
					"result": []map[string]interface{}{{"symbol": "AAPL", "trailingPE": 20.0}},
				},
			})
		case r.URL.Path == "/v10/finance/quoteSummary/AAPL" && modules == "assetProfile":
			time.Sleep(delay)
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"assetProfile": map[string]interface{}{"sector": "Technology"},
			}))
		case r.URL.Path == "/v10/finance/quoteSummary/AAPL" && modules == "defaultKeyStatistics":
			time.Sleep(delay)
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"defaultKeyStatistics": map[string]interface{}{"enterpriseToEbitda": raw(25)},
			}))
		case r.URL.Path == "/v1/finance/sectors/technology":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"data": map[string]interface{}{"topCompanies": []map[string]interface{}{{"symbol": "PEER1"}}},
			})
		default:
			_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
				"summaryDetail":        map[string]interface{}{"marketCap": raw(1000)},
				"defaultKeyStatistics": map[string]interface{}{"enterpriseValue": raw(1200), "netIncomeToCommon": raw(100)},
				"financialData":        map[string]interface{}{"ebitda": raw(150)},
			}))
		}
	}))
	defer srv.Close()

	start := time.Now()
	if _, err := newTestClient(t, srv).GetSectorBenchmark(context.Background(), "AAPL"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed := time.Since(start); elapsed >= 2*delay {
		t.Errorf("took %v; sector/PE/EV lookups appear to run sequentially (want < %v)", elapsed, 2*delay)
	}
}

// GetFundamentals exists so callers can swap ~18 requests for one; that is only safe if
// every field matches what the single-indicator method would return for the same data.
func TestGetFundamentals_MatchesSingleIndicatorMethods(t *testing.T) {
	var batchedModules []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if m := r.URL.Query().Get("modules"); strings.Count(m, ",") > 1 {
			batchedModules = append(batchedModules, m)
		}
		_ = json.NewEncoder(w).Encode(summaryResult(map[string]interface{}{
			"financialData": map[string]interface{}{
				"freeCashflow": raw(6e10), "operatingCashflow": raw(1.1e11), "debtToEquity": raw(150),
				"profitMargins": raw(0.25), "operatingMargins": raw(0.3), "revenueGrowth": raw(0.08),
				"totalCash": raw(6.5e10), "totalDebt": raw(1.1e11), "financialCurrency": "EUR",
			},
			"defaultKeyStatistics": map[string]interface{}{
				"netIncomeToCommon": raw(1e11), "enterpriseToEbitda": raw(22), "priceToBook": raw(45),
				"earningsQuarterlyGrowth": raw(-0.05),
			},
			"summaryDetail": map[string]interface{}{
				"marketCap": raw(3e12), "priceToSalesTrailing12Months": raw(8), "dividendYield": raw(0.005),
				"payoutRatio": raw(0.15),
			},
			"calendarEvents": map[string]interface{}{"dividendDate": raw(1.7e9)},
			"assetProfile":   map[string]interface{}{"sector": "Technology"},
		}))
	}))
	defer srv.Close()

	client := newTestClient(t, srv)
	ctx := context.Background()
	got, err := client.GetFundamentals(ctx, "AAPL")
	if err != nil {
		t.Fatalf("GetFundamentals: %v", err)
	}
	if len(batchedModules) != 1 {
		t.Fatalf("batched requests: got %d, want 1", len(batchedModules))
	}
	for _, m := range []string{"financialData", "defaultKeyStatistics", "summaryDetail", "calendarEvents", "assetProfile"} {
		if !strings.Contains(batchedModules[0], m) {
			t.Errorf("batched request missing module %q: %q", m, batchedModules[0])
		}
	}

	must := func(v interface{}, err error) interface{} {
		t.Helper()
		if err != nil {
			t.Fatalf("single-indicator call: %v", err)
		}
		return v
	}
	checks := []struct {
		name   string
		batch  interface{}
		single interface{}
	}{
		{"Sector", got.Sector, must(client.GetSector(ctx, "AAPL"))},
		{"FreeCashFlow", got.FreeCashFlow, must(client.GetFreeCashFlow(ctx, "AAPL"))},
		{"CashFlowQuality", got.CashFlowQuality, must(client.GetOperatingCashFlowVsNetIncome(ctx, "AAPL"))},
		{"DebtToEquity", got.DebtToEquity, must(client.GetDebtToEquity(ctx, "AAPL"))},
		{"EVToEBITDA", got.EVToEBITDA, must(client.GetEVToEBITDA(ctx, "AAPL"))},
		{"MarketCap", got.MarketCap, must(client.GetMarketCap(ctx, "AAPL"))},
		{"PriceToSales", got.PriceToSales, must(client.GetPriceToSales(ctx, "AAPL"))},
		{"PriceToBook", got.PriceToBook, must(client.GetPriceToBook(ctx, "AAPL"))},
		{"FreeCashFlowYield", got.FreeCashFlowYield, must(client.GetFreeCashFlowYield(ctx, "AAPL"))},
		{"ProfitMargin", got.ProfitMargin, must(client.GetProfitMargin(ctx, "AAPL"))},
		{"OperatingMargin", got.OperatingMargin, must(client.GetOperatingMargin(ctx, "AAPL"))},
		{"QuarterlyEarningsGrowth", got.QuarterlyEarningsGrowth, must(client.GetQuarterlyEarningsGrowth(ctx, "AAPL"))},
		{"QuarterlyRevenueGrowth", got.QuarterlyRevenueGrowth, must(client.GetQuarterlyRevenueGrowth(ctx, "AAPL"))},
		{"Cash", got.Cash, must(client.GetCash(ctx, "AAPL"))},
		{"Debt", got.Debt, must(client.GetDebt(ctx, "AAPL"))},
		{"DividendYield", got.DividendYield, must(client.GetDividendYield(ctx, "AAPL"))},
		{"PayoutRatio", got.PayoutRatio, must(client.GetPayoutRatio(ctx, "AAPL"))},
		{"PayoutDate", got.PayoutDate, must(client.GetPayoutDate(ctx, "AAPL"))},
	}
	for _, c := range checks {
		if !reflect.DeepEqual(c.batch, c.single) {
			t.Errorf("%s: batched %+v != single %+v", c.name, c.batch, c.single)
		}
	}
	// Money values are labelled with this code; it must come from the reporting currency, not be assumed USD.
	if got.Currency != "EUR" {
		t.Errorf("currency: got %q, want %q", got.Currency, "EUR")
	}
	if got.CashFlowQuality.Ratio != 1.1 || got.MarketCap.MarketCap != 3e12 || got.Sector.Sector != "Technology" {
		t.Errorf("fixture values not decoded: %+v %+v %+v", got.CashFlowQuality, got.MarketCap, got.Sector)
	}
}

func TestGetFundamentals_Errors(t *testing.T) {
	tests := []struct {
		name    string
		handler http.HandlerFunc
		wantErr error
	}{
		{"not found — 404", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNotFound) }, yahoo.ErrTickerNotFound},
		{"not found — empty result", func(w http.ResponseWriter, _ *http.Request) { _ = json.NewEncoder(w).Encode(emptySummary()) }, yahoo.ErrTickerNotFound},
		{"http error status", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusInternalServerError) }, yahoo.ErrAPIError},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			_, err := newTestClient(t, srv).GetFundamentals(context.Background(), "AAPL")
			checkErr(t, err, tc.wantErr)
		})
	}
}
