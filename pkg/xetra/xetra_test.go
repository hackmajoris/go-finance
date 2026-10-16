package xetra_test

import (
	"testing"

	"github.com/hackmajoris/go-finance/pkg/xetra"
)

// LookupYahoo is how the summary finds an ISIN for Xetra ETFs Yahoo has no TER for;
// only ".DE" tickers are Xetra listings, so other suffixes must not match even when
// the bare symbol exists on Xetra.
func TestLookupYahoo(t *testing.T) {
	tests := []struct {
		ticker   string
		wantISIN string
		wantOK   bool
	}{
		{"VWCE.DE", "IE00BK5BQT80", true},
		{"vwce.de", "IE00BK5BQT80", true},
		{"EUNL.DE", "IE00B4L5Y983", true},
		{"VWCE.AS", "", false},
		{"VWCE", "", false},
		{"NOPE.DE", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.ticker, func(t *testing.T) {
			got, ok := xetra.LookupYahoo(tc.ticker)
			if ok != tc.wantOK || got.ISIN != tc.wantISIN {
				t.Errorf("got (%q, %v), want (%q, %v)", got.ISIN, ok, tc.wantISIN, tc.wantOK)
			}
		})
	}
}
