package justetf_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/hackmajoris/go-finance/pkg/justetf"
)

func TestGetTER(t *testing.T) {
	profile, err := os.ReadFile("testdata/profile.html")
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name    string
		handler http.HandlerFunc
		want    float64
		wantErr error
	}{
		{
			name: "profile page",
			handler: func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/en/etf-profile.html" || r.URL.Query().Get("isin") != "IE00BK5BQT80" {
					t.Errorf("unexpected request %s", r.URL)
				}
				_, _ = w.Write(profile)
			},
			want: 0.14,
		},
		{
			// Some locales render "0,19%"; reading it as 0 or 19 would show a wrong cost.
			name: "comma decimal separator",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<div class="val" data-testid="tl_etf-basics_value_ter">0,19% p.a.</div>`))
			},
			want: 0.19,
		},
		{
			// justETF answers unknown ISINs with 200 + the search page; that must be
			// ErrNotFound so callers show "—" rather than a 0% TER.
			name: "unknown isin lands on search page",
			handler: func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(`<html><body>ETF search</body></html>`))
			},
			wantErr: justetf.ErrNotFound,
		},
		{
			name:    "http error status",
			handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
			wantErr: justetf.ErrAPIError,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(tc.handler)
			defer srv.Close()
			c, err := justetf.New(justetf.WithBaseURL(srv.URL))
			if err != nil {
				t.Fatal(err)
			}
			got, err := c.GetTER(context.Background(), "IE00BK5BQT80")
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("err: got %v, want %v", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.TER != tc.want || got.ISIN != "IE00BK5BQT80" {
				t.Errorf("got %+v, want TER %v", got, tc.want)
			}
		})
	}
}
