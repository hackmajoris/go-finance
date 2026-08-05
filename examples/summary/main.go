// Command summary prints every crumb-free-of-params indicator for a single stock ticker
// as a table: quote, health rating, valuation rating, P/E, free cash flow, cash flow
// quality, debt-to-equity, EV/EBITDA, 52-week range, and performance returns.
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

type row struct {
	metric string
	value  string
	note   string
}

// formatMoney scales a raw dollar amount to the nearest K/M/B/T suffix, e.g. 1.07721875456e+11 → "$107.72B".
func formatMoney(v float64) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1e12:
		return fmt.Sprintf("$%.2fT", v/1e12)
	case abs >= 1e9:
		return fmt.Sprintf("$%.2fB", v/1e9)
	case abs >= 1e6:
		return fmt.Sprintf("$%.2fM", v/1e6)
	case abs >= 1e3:
		return fmt.Sprintf("$%.2fK", v/1e3)
	default:
		return fmt.Sprintf("$%.2f", v)
	}
}

func main() {
	ticker := "AAPL"
	if len(os.Args) > 1 {
		ticker = os.Args[1]
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	ctx := context.Background()
	var rows []row

	if quote, err := client.GetQuote(ctx, ticker); err != nil {
		rows = append(rows, row{"Price", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Price", fmt.Sprintf("%.2f %s", quote.Price, quote.Currency), ""})
	}

	// fetched up front (rather than inline in the table build below) so ClassifyHealth
	// and ClassifyValuation can reuse them without extra requests
	pe, peErr := client.GetPE(ctx, ticker)
	fcf, fcfErr := client.GetFreeCashFlow(ctx, ticker)
	cfq, cfqErr := client.GetOperatingCashFlowVsNetIncome(ctx, ticker)
	d2e, d2eErr := client.GetDebtToEquity(ctx, ticker)
	ev, evErr := client.GetEVToEBITDA(ctx, ticker)

	health, healthReason := yahoo.ClassifyHealth(fcf, cfq, d2e)
	valuation, valuationReason := yahoo.ClassifyValuation(pe, ev)
	rows = append(rows, row{"Health", strings.ToUpper(string(health)), healthReason})
	rows = append(rows, row{"Valuation", strings.ToUpper(string(valuation)), valuationReason})

	if peErr != nil {
		rows = append(rows, row{"P/E", "error: " + peErr.Error(), ""})
	} else {
		rows = append(rows, row{"P/E (trailing)", fmt.Sprintf("%.2f", pe.PE), pe.Interpretation})
		rows = append(rows, row{"P/E (forward)", fmt.Sprintf("%.2f", pe.ForwardPE), ""})
	}

	if fcfErr != nil {
		rows = append(rows, row{"Free Cash Flow", "error: " + fcfErr.Error(), ""})
	} else {
		rows = append(rows, row{"Free Cash Flow", formatMoney(fcf.FCF), fcf.Interpretation})
	}

	if cfqErr != nil {
		rows = append(rows, row{"OCF / Net Income", "error: " + cfqErr.Error(), ""})
	} else {
		rows = append(rows, row{"Operating Cash Flow", formatMoney(cfq.OperatingCashFlow), ""})
		rows = append(rows, row{"Net Income", formatMoney(cfq.NetIncome), ""})
		rows = append(rows, row{"OCF / Net Income", fmt.Sprintf("%.2fx", cfq.Ratio), cfq.Interpretation})
	}

	if d2eErr != nil {
		rows = append(rows, row{"Debt / Equity", "error: " + d2eErr.Error(), ""})
	} else {
		rows = append(rows, row{"Debt / Equity", fmt.Sprintf("%.1f%%", d2e.Ratio), d2e.Interpretation})
	}

	if evErr != nil {
		rows = append(rows, row{"EV / EBITDA", "error: " + evErr.Error(), ""})
	} else {
		rows = append(rows, row{"EV / EBITDA", fmt.Sprintf("%.2fx", ev.Ratio), ev.Interpretation})
	}

	if rng, err := client.FetchFiftyTwoWeekRange(ctx, ticker); err != nil {
		rows = append(rows, row{"52-Week Range", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"52-Week Range", fmt.Sprintf("%.2f - %.2f (%.0f%%)", rng.Low, rng.High, rng.Pct*100), ""})
	}

	if perf, err := client.FetchPerformance(ctx, ticker); err != nil {
		rows = append(rows, row{"Performance", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"YTD", fmt.Sprintf("%+.2f%%", perf.YTD), ""})
		rows = append(rows, row{"1-Year", fmt.Sprintf("%+.2f%%", perf.OneYear), ""})
		rows = append(rows, row{"3-Year", fmt.Sprintf("%+.2f%%", perf.ThreeYear), ""})
		rows = append(rows, row{"5-Year", fmt.Sprintf("%+.2f%%", perf.FiveYear), ""})
	}

	fmt.Printf("%s\n\n", ticker)

	w := tabwriter.NewWriter(os.Stdout, 0, 2, 2, ' ', 0)
	if _, err := fmt.Fprintln(w, "METRIC\tVALUE\tNOTES"); err != nil {
		log.Fatal(err)
	}
	for _, r := range rows {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\n", r.metric, r.value, r.note); err != nil {
			log.Fatal(err)
		}
	}
	if err := w.Flush(); err != nil {
		log.Fatal(err)
	}
}
