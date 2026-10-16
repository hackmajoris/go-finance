package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

// The generator must keep only ETFs: shares, ETNs and ETCs in the same file would
// otherwise be treated as funds and sent to justETF for a TER that does not exist.
func TestParseETFs_KeepsOnlyETFsSortedByMnemonic(t *testing.T) {
	f, err := os.Open("testdata/instruments.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()

	rows, err := parseETFs(f)
	if err != nil {
		t.Fatalf("parseETFs: %v", err)
	}
	want := [][3]string{
		{"EUNL", "IE00B4L5Y983", "ISHSIII-CORE MSCI WLD DLA"},
		{"VWCE", "IE00BK5BQT80", "VANG.FTSE A.W. DLA"},
	}
	if len(rows) != len(want) {
		t.Fatalf("rows: got %d %v, want %d", len(rows), rows, len(want))
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Errorf("row %d: got %v, want %v", i, rows[i], want[i])
		}
	}

	var buf bytes.Buffer
	if err := writeETFs(&buf, rows); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.HasPrefix(got, "Mnemonic;ISIN;Name\nEUNL;IE00B4L5Y983;") {
		t.Errorf("output: %q", got)
	}
}

// Deutsche Börse reorders columns between releases; a missing column must fail the
// regeneration instead of silently writing the wrong field as the ISIN.
func TestParseETFs_MissingColumn(t *testing.T) {
	in := "Market:;XETR\nDate Last Update:;01.01.2026\nInstrument;ISIN;Instrument Type\nX;IE0000000000;ETF\n"
	if _, err := parseETFs(strings.NewReader(in)); err == nil || !strings.Contains(err.Error(), "Mnemonic") {
		t.Errorf("got %v, want missing Mnemonic column error", err)
	}
}
