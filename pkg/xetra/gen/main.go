// Command gen downloads Deutsche Börse's Xetra "All tradable instruments" CSV and writes
// the ETF rows as "Mnemonic;ISIN;Name" for embedding in package xetra.
package main

import (
	"bufio"
	"encoding/csv"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strings"
)

const sourceURL = "https://www.cashmarket.deutsche-boerse.com/resource/blob/1528/b52ea43a2edac92e8283d40645d1c076/data/t7-xetr-allTradableInstruments.csv"

func main() {
	out := flag.String("out", "etfs.csv", "output file")
	flag.Parse()

	resp, err := http.Get(sourceURL)
	if err != nil {
		log.Fatalf("downloading instruments: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		log.Fatalf("downloading instruments: status %d", resp.StatusCode)
	}

	rows, err := parseETFs(resp.Body)
	if err != nil {
		log.Fatal(err)
	}

	f, err := os.Create(*out)
	if err != nil {
		log.Fatal(err)
	}
	if err := writeETFs(f, rows); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("wrote %d ETFs to %s\n", len(rows), *out)
}

// parseETFs reads the Deutsche Börse instruments CSV (two metadata lines, then a
// semicolon-separated header) and returns [mnemonic, isin, name] for every ETF,
// sorted by mnemonic. Columns are located by header name, not position.
func parseETFs(r io.Reader) ([][3]string, error) {
	br := bufio.NewReader(r)
	for i := 0; i < 2; i++ {
		if _, err := br.ReadString('\n'); err != nil {
			return nil, fmt.Errorf("reading metadata line %d: %w", i+1, err)
		}
	}

	cr := csv.NewReader(br)
	cr.Comma = ';'
	cr.FieldsPerRecord = -1
	cr.LazyQuotes = true

	header, err := cr.Read()
	if err != nil {
		return nil, fmt.Errorf("reading header: %w", err)
	}
	col := map[string]int{}
	for i, h := range header {
		col[strings.TrimSpace(h)] = i
	}
	for _, name := range []string{"Mnemonic", "ISIN", "Instrument", "Instrument Type"} {
		if _, ok := col[name]; !ok {
			return nil, fmt.Errorf("missing column %q", name)
		}
	}

	var rows [][3]string
	for {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("reading row: %w", err)
		}
		if len(rec) <= col["Instrument Type"] || rec[col["Instrument Type"]] != "ETF" {
			continue
		}
		m, isin := rec[col["Mnemonic"]], rec[col["ISIN"]]
		if m == "" || isin == "" {
			continue
		}
		rows = append(rows, [3]string{m, isin, strings.ReplaceAll(rec[col["Instrument"]], ";", " ")})
	}
	if len(rows) == 0 {
		return nil, errors.New("no ETF rows found")
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
	return rows, nil
}

func writeETFs(w io.Writer, rows [][3]string) error {
	bw := bufio.NewWriter(w)
	if _, err := bw.WriteString("Mnemonic;ISIN;Name\n"); err != nil {
		return err
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(bw, "%s;%s;%s\n", r[0], r[1], r[2]); err != nil {
			return err
		}
	}
	return bw.Flush()
}
