// Package xetra maps Xetra-listed ETF tickers to their ISINs using Deutsche Börse's
// "All tradable instruments" list, embedded at build time. Regenerate etfs.csv with
// `make etf-isins` (or `go generate ./pkg/xetra`).
package xetra

//go:generate go run ./gen -out etfs.csv

import (
	_ "embed"
	"strings"
	"sync"
)

//go:embed etfs.csv
var etfsCSV string

// ETF is one Xetra-listed exchange traded fund.
type ETF struct {
	Mnemonic string // Xetra ticker (e.g. "VWCE")
	ISIN     string // fund share class ISIN (e.g. "IE00BK5BQT80")
	Name     string // Deutsche Börse's abbreviated instrument name
}

var etfs = sync.OnceValue(func() map[string]ETF {
	m := make(map[string]ETF)
	for _, line := range strings.Split(etfsCSV, "\n") {
		f := strings.SplitN(line, ";", 3)
		if len(f) != 3 || f[0] == "Mnemonic" {
			continue
		}
		m[f[0]] = ETF{Mnemonic: f[0], ISIN: f[1], Name: f[2]}
	}
	return m
})

// Lookup returns the ETF listed on Xetra under mnemonic.
func Lookup(mnemonic string) (ETF, bool) {
	e, ok := etfs()[strings.ToUpper(mnemonic)]
	return e, ok
}

// LookupYahoo resolves a Yahoo Finance Xetra ticker (e.g. "VWCE.DE") to its ETF.
// Tickers from other exchanges are not covered and return false.
func LookupYahoo(ticker string) (ETF, bool) {
	mnemonic, ok := strings.CutSuffix(strings.ToUpper(ticker), ".DE")
	if !ok {
		return ETF{}, false
	}
	return Lookup(mnemonic)
}
