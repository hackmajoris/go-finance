// Command summary prints every crumb-free-of-params indicator for a single stock ticker
// as a table: quote, health rating, valuation rating, P/E, free cash flow, cash flow
// quality, debt-to-equity, EV/EBITDA, market cap, price/sales, price/book, FCF yield,
// profit & operating margins, quarterly earnings & revenue growth, cash/debt/net,
// dividend yield/payout ratio/payout date, 52-week range, and performance returns.
//
// Indicators are fetched concurrently. The crumb is warmed with a single sequential
// call first, so the parallel fan-out only reads the (now-set) crumb — safe for
// concurrent use — instead of racing to run the handshake many times at once.
package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"os"
	"strings"
	"sync"
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

	// Warm the crumb with one sequential call before fanning out; this also gives the
	// Price row. Every parallel call below then finds the crumb already set and only
	// reads it, avoiding a data race on the lazy handshake.
	quote, quoteErr := client.GetQuote(ctx, ticker)

	var (
		pe      *yahoo.PERatio
		peErr   error
		fcf     *yahoo.FreeCashFlow
		fcfErr  error
		cfq     *yahoo.CashFlowQuality
		cfqErr  error
		d2e     *yahoo.DebtToEquity
		d2eErr  error
		ev      *yahoo.EVToEBITDA
		evErr   error
		mc      *yahoo.MarketCap
		mcErr   error
		ps      *yahoo.PriceToSales
		psErr   error
		pb      *yahoo.PriceToBook
		pbErr   error
		fy      *yahoo.FreeCashFlowYield
		fyErr   error
		pm      *yahoo.ProfitMargin
		pmErr   error
		om      *yahoo.OperatingMargin
		omErr   error
		eg      *yahoo.QuarterlyEarningsGrowth
		egErr   error
		rg      *yahoo.QuarterlyRevenueGrowth
		rgErr   error
		cash    *yahoo.Cash
		cashErr error
		debt    *yahoo.Debt
		debtErr error
		dy      *yahoo.DividendYield
		dyErr   error
		prr     *yahoo.PayoutRatio
		prErr   error
		pd      *yahoo.PayoutDate
		pdErr   error
		rng     *yahoo.FiftyTwoWeekRange
		rngErr  error
		perf    *yahoo.PerformanceReturns
		perfErr error
	)

	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}

	run(func() { pe, peErr = client.GetPE(ctx, ticker) })
	run(func() { fcf, fcfErr = client.GetFreeCashFlow(ctx, ticker) })
	run(func() { cfq, cfqErr = client.GetOperatingCashFlowVsNetIncome(ctx, ticker) })
	run(func() { d2e, d2eErr = client.GetDebtToEquity(ctx, ticker) })
	run(func() { ev, evErr = client.GetEVToEBITDA(ctx, ticker) })
	run(func() { mc, mcErr = client.GetMarketCap(ctx, ticker) })
	run(func() { ps, psErr = client.GetPriceToSales(ctx, ticker) })
	run(func() { pb, pbErr = client.GetPriceToBook(ctx, ticker) })
	run(func() { fy, fyErr = client.GetFreeCashFlowYield(ctx, ticker) })
	run(func() { pm, pmErr = client.GetProfitMargin(ctx, ticker) })
	run(func() { om, omErr = client.GetOperatingMargin(ctx, ticker) })
	run(func() { eg, egErr = client.GetQuarterlyEarningsGrowth(ctx, ticker) })
	run(func() { rg, rgErr = client.GetQuarterlyRevenueGrowth(ctx, ticker) })
	run(func() { cash, cashErr = client.GetCash(ctx, ticker) })
	run(func() { debt, debtErr = client.GetDebt(ctx, ticker) })
	run(func() { dy, dyErr = client.GetDividendYield(ctx, ticker) })
	run(func() { prr, prErr = client.GetPayoutRatio(ctx, ticker) })
	run(func() { pd, pdErr = client.GetPayoutDate(ctx, ticker) })
	run(func() { rng, rngErr = client.FetchFiftyTwoWeekRange(ctx, ticker) })
	run(func() { perf, perfErr = client.FetchPerformance(ctx, ticker) })

	wg.Wait()

	var rows []row

	if quoteErr != nil {
		rows = append(rows, row{"Price", "error: " + quoteErr.Error(), ""})
	} else {
		rows = append(rows, row{"Price", fmt.Sprintf("%.2f %s", quote.Price, quote.Currency), ""})
	}

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

	if mcErr != nil {
		rows = append(rows, row{"Market Cap", "error: " + mcErr.Error(), ""})
	} else {
		rows = append(rows, row{"Market Cap", formatMoney(mc.MarketCap), mc.Interpretation})
	}

	if psErr != nil {
		rows = append(rows, row{"Price / Sales", "error: " + psErr.Error(), ""})
	} else {
		rows = append(rows, row{"Price / Sales", fmt.Sprintf("%.2fx", ps.Ratio), ps.Interpretation})
	}

	if pbErr != nil {
		rows = append(rows, row{"Price / Book", "error: " + pbErr.Error(), ""})
	} else {
		rows = append(rows, row{"Price / Book", fmt.Sprintf("%.2fx", pb.Ratio), pb.Interpretation})
	}

	if fyErr != nil {
		rows = append(rows, row{"FCF Yield", "error: " + fyErr.Error(), ""})
	} else {
		rows = append(rows, row{"FCF Yield", fmt.Sprintf("%.2f%%", fy.Yield), fy.Interpretation})
	}

	if pmErr != nil {
		rows = append(rows, row{"Profit Margin", "error: " + pmErr.Error(), ""})
	} else {
		rows = append(rows, row{"Profit Margin", fmt.Sprintf("%.2f%%", pm.Margin), pm.Interpretation})
	}

	if omErr != nil {
		rows = append(rows, row{"Operating Margin", "error: " + omErr.Error(), ""})
	} else {
		rows = append(rows, row{"Operating Margin", fmt.Sprintf("%.2f%%", om.Margin), om.Interpretation})
	}

	if egErr != nil {
		rows = append(rows, row{"Q Earnings (YoY)", "error: " + egErr.Error(), ""})
	} else {
		rows = append(rows, row{"Q Earnings (YoY)", fmt.Sprintf("%+.2f%%", eg.Growth), eg.Interpretation})
	}

	if rgErr != nil {
		rows = append(rows, row{"Q Revenue (YoY)", "error: " + rgErr.Error(), ""})
	} else {
		rows = append(rows, row{"Q Revenue (YoY)", fmt.Sprintf("%+.2f%%", rg.Growth), rg.Interpretation})
	}

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

	if dyErr != nil {
		rows = append(rows, row{"Dividend Yield", "error: " + dyErr.Error(), ""})
	} else {
		rows = append(rows, row{"Dividend Yield", fmt.Sprintf("%.2f%%", dy.Yield), dy.Interpretation})
	}

	if prErr != nil {
		rows = append(rows, row{"Payout Ratio", "error: " + prErr.Error(), ""})
	} else {
		rows = append(rows, row{"Payout Ratio", fmt.Sprintf("%.2f%%", prr.Ratio), prr.Interpretation})
	}

	if pdErr != nil {
		rows = append(rows, row{"Payout Date", "error: " + pdErr.Error(), ""})
	} else {
		val := "—"
		if !pd.Date.IsZero() {
			val = pd.Date.Format("2006-01-02")
		}
		rows = append(rows, row{"Payout Date", val, pd.Interpretation})
	}

	if rngErr != nil {
		rows = append(rows, row{"52-Week Range", "error: " + rngErr.Error(), ""})
	} else {
		rows = append(rows, row{"52-Week Range", fmt.Sprintf("%.2f - %.2f (%.0f%%)", rng.Low, rng.High, rng.Pct*100), ""})
	}

	if perfErr != nil {
		rows = append(rows, row{"Performance", "error: " + perfErr.Error(), ""})
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
