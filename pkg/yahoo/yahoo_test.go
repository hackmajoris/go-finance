package yahoo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

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
