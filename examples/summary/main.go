// Command summary prints every crumb-free-of-params indicator for a single stock ticker
// as a table: quote, health rating, valuation rating, P/E, free cash flow, cash flow
// quality, debt-to-equity, EV/EBITDA, market cap, price/sales, price/book, FCF yield,
// profit & operating margins, quarterly earnings & revenue growth, cash/debt/net,
// dividend yield/payout ratio/payout date, 52-week range, and performance returns.
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
	ticker := "NVDA"
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

	if mc, err := client.GetMarketCap(ctx, ticker); err != nil {
		rows = append(rows, row{"Market Cap", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Market Cap", formatMoney(mc.MarketCap), mc.Interpretation})
	}

	if ps, err := client.GetPriceToSales(ctx, ticker); err != nil {
		rows = append(rows, row{"Price / Sales", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Price / Sales", fmt.Sprintf("%.2fx", ps.Ratio), ps.Interpretation})
	}

	if pb, err := client.GetPriceToBook(ctx, ticker); err != nil {
		rows = append(rows, row{"Price / Book", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Price / Book", fmt.Sprintf("%.2fx", pb.Ratio), pb.Interpretation})
	}

	if fy, err := client.GetFreeCashFlowYield(ctx, ticker); err != nil {
		rows = append(rows, row{"FCF Yield", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"FCF Yield", fmt.Sprintf("%.2f%%", fy.Yield), fy.Interpretation})
	}

	if pm, err := client.GetProfitMargin(ctx, ticker); err != nil {
		rows = append(rows, row{"Profit Margin", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Profit Margin", fmt.Sprintf("%.2f%%", pm.Margin), pm.Interpretation})
	}

	if om, err := client.GetOperatingMargin(ctx, ticker); err != nil {
		rows = append(rows, row{"Operating Margin", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Operating Margin", fmt.Sprintf("%.2f%%", om.Margin), om.Interpretation})
	}

	if eg, err := client.GetQuarterlyEarningsGrowth(ctx, ticker); err != nil {
		rows = append(rows, row{"Q Earnings (YoY)", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Q Earnings (YoY)", fmt.Sprintf("%+.2f%%", eg.Growth), eg.Interpretation})
	}

	if rg, err := client.GetQuarterlyRevenueGrowth(ctx, ticker); err != nil {
		rows = append(rows, row{"Q Revenue (YoY)", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Q Revenue (YoY)", fmt.Sprintf("%+.2f%%", rg.Growth), rg.Interpretation})
	}

	cash, cashErr := client.GetCash(ctx, ticker)
	debt, debtErr := client.GetDebt(ctx, ticker)
	if cashErr != nil {
		rows = append(rows, row{"Cash", "error: " + cashErr.Error(), ""})
	} else {
		rows = append(rows, row{"Cash", formatMoney(cash.Cash), cash.Interpretation})
	}
	if debtErr != nil {
		rows = append(rows, row{"Debt", "error: " + debtErr.Error(), ""})
	} else {
		rows = append(rows, row{"Debt", formatMoney(debt.Debt), debt.Interpretation})
	}
	if cashErr == nil && debtErr == nil {
		net := cash.Cash - debt.Debt
		note := "net cash position (cash exceeds debt)"
		if net < 0 {
			note = "net debt position (debt exceeds cash)"
		}
		rows = append(rows, row{"Net", formatMoney(net), note})
	}

	if dy, err := client.GetDividendYield(ctx, ticker); err != nil {
		rows = append(rows, row{"Dividend Yield", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Dividend Yield", fmt.Sprintf("%.2f%%", dy.Yield), dy.Interpretation})
	}

	if pr, err := client.GetPayoutRatio(ctx, ticker); err != nil {
		rows = append(rows, row{"Payout Ratio", "error: " + err.Error(), ""})
	} else {
		rows = append(rows, row{"Payout Ratio", fmt.Sprintf("%.2f%%", pr.Ratio), pr.Interpretation})
	}

	if pd, err := client.GetPayoutDate(ctx, ticker); err != nil {
		rows = append(rows, row{"Payout Date", "error: " + err.Error(), ""})
	} else {
		val := "—"
		if !pd.Date.IsZero() {
			val = pd.Date.Format("2006-01-02")
		}
		rows = append(rows, row{"Payout Date", val, pd.Interpretation})
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
