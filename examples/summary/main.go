// Command summary prints every crumb-free-of-params indicator for a single stock ticker
// as a table: quote, health rating, valuation rating, P/E, free cash flow, cash flow
// quality, debt-to-equity, EV/EBITDA, market cap, price/sales, price/book, FCF yield,
// profit & operating margins, quarterly earnings & revenue growth, cash/debt/net,
// dividend yield/payout ratio/payout date, 52-week range, and performance returns.
//
// Indicators are fetched concurrently. All quoteSummary-based indicators come from a
// single GetFundamentals request; the client serialises the crumb handshake itself.
package main

import (
	"context"
	"errors"
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

// formatMoney scales a raw amount to the nearest K/M/B/T suffix and labels it with its
// ISO currency code, e.g. (1.07721875456e+11, "EUR") → "107.72B EUR".
func formatMoney(v float64, currency string) string {
	abs := math.Abs(v)
	switch {
	case abs >= 1e12:
		return fmt.Sprintf("%.2fT %s", v/1e12, currency)
	case abs >= 1e9:
		return fmt.Sprintf("%.2fB %s", v/1e9, currency)
	case abs >= 1e6:
		return fmt.Sprintf("%.2fM %s", v/1e6, currency)
	case abs >= 1e3:
		return fmt.Sprintf("%.2fK %s", v/1e3, currency)
	default:
		return fmt.Sprintf("%.2f %s", v, currency)
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

	var (
		quote     *yahoo.Quote
		quoteErr  error
		fund      *yahoo.Fundamentals
		fundErr   error
		pe        *yahoo.PERatio
		peErr     error
		rng       *yahoo.FiftyTwoWeekRange
		rngErr    error
		perf      *yahoo.PerformanceReturns
		perfErr   error
		bench     *yahoo.SectorBenchmark
		benchErr  error
		rating    *yahoo.AnalystRating
		ratingErr error
	)

	var wg sync.WaitGroup
	run := func(f func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			f()
		}()
	}

	run(func() { quote, quoteErr = client.GetQuote(ctx, ticker) })
	run(func() { fund, fundErr = client.GetFundamentals(ctx, ticker) })
	run(func() { pe, peErr = client.GetPE(ctx, ticker) })
	run(func() { rng, rngErr = client.FetchFiftyTwoWeekRange(ctx, ticker) })
	run(func() { perf, perfErr = client.FetchPerformance(ctx, ticker) })
	run(func() { bench, benchErr = client.GetSectorBenchmark(ctx, ticker) })
	run(func() { rating, ratingErr = client.GetAnalystRating(ctx, ticker) })

	wg.Wait()

	var (
		ter        *yahoo.ExpenseRatio
		terErr     error
		holdings   *yahoo.TopHoldings
		holdingErr error
	)
	isETF := quoteErr == nil && quote.QuoteType == "ETF"
	if isETF {
		run(func() { ter, terErr = client.GetExpenseRatio(ctx, ticker) })
		run(func() { holdings, holdingErr = client.GetTopHoldings(ctx, ticker) })
		wg.Wait()
	}

	if fund == nil {
		fund = &yahoo.Fundamentals{}
	}
	var priceCcy string
	if quoteErr == nil {
		priceCcy = quote.Currency
	}
	ccy := fund.Currency
	if ccy == "" {
		ccy = priceCcy
	}
	sector, sectorErr := fund.Sector, fundErr
	fcf, fcfErr := fund.FreeCashFlow, fundErr
	cfq, cfqErr := fund.CashFlowQuality, fundErr
	d2e, d2eErr := fund.DebtToEquity, fundErr
	ev, evErr := fund.EVToEBITDA, fundErr
	mc, mcErr := fund.MarketCap, fundErr
	ps, psErr := fund.PriceToSales, fundErr
	pb, pbErr := fund.PriceToBook, fundErr
	fy, fyErr := fund.FreeCashFlowYield, fundErr
	pm, pmErr := fund.ProfitMargin, fundErr
	om, omErr := fund.OperatingMargin, fundErr
	eg, egErr := fund.QuarterlyEarningsGrowth, fundErr
	rg, rgErr := fund.QuarterlyRevenueGrowth, fundErr
	cash, cashErr := fund.Cash, fundErr
	debt, debtErr := fund.Debt, fundErr
	dy, dyErr := fund.DividendYield, fundErr
	prr, prErr := fund.PayoutRatio, fundErr
	pd, pdErr := fund.PayoutDate, fundErr

	var rows []row

	if quoteErr != nil {
		rows = append(rows, row{"Price", "error: " + quoteErr.Error(), ""})
	} else {
		rows = append(rows, row{"Price", fmt.Sprintf("%.2f %s", quote.Price, quote.Currency), ""})
		note := ""
		if fund.Currency != "" && fund.Currency != quote.Currency {
			note = "financials reported in " + fund.Currency
		}
		rows = append(rows, row{"Currency", quote.Currency, note})
	}

	if isETF {
		switch {
		case errors.Is(terErr, yahoo.ErrTickerNotFound):
			rows = append(rows, row{"TER", "—", "Yahoo has no fund data for this ticker"})
		case terErr != nil:
			rows = append(rows, row{"TER", "error: " + terErr.Error(), ""})
		case ter.TER == 0:
			rows = append(rows, row{"TER", "—", ter.Interpretation})
		default:
			rows = append(rows, row{"TER", fmt.Sprintf("%.2f%%", ter.TER), ter.Interpretation + " (source: " + ter.Source + ")"})
		}

		switch {
		case errors.Is(holdingErr, yahoo.ErrTickerNotFound):
			rows = append(rows, row{"Top Holdings", "—", "Yahoo has no fund data for this ticker"})
		case holdingErr != nil:
			rows = append(rows, row{"Top Holdings", "error: " + holdingErr.Error(), ""})
		case len(holdings.Holdings) == 0:
			rows = append(rows, row{"Top Holdings", "—", "No holdings reported."})
		default:
			for i, h := range holdings.Holdings {
				rows = append(rows, row{fmt.Sprintf("Holding #%d", i+1), fmt.Sprintf("%s %.2f%%", h.Symbol, h.Weight), h.Name})
			}
		}
	}

	if sectorErr != nil {
		rows = append(rows, row{"Sector", "error: " + sectorErr.Error(), ""})
	} else {
		rows = append(rows, row{"Sector", sector.Sector, ""})
	}

	if ratingErr != nil {
		rows = append(rows, row{"Avg. Analyst Rating", "error: " + ratingErr.Error(), ""})
	} else if rating.Rating == "" {
		rows = append(rows, row{"Avg. Analyst Rating", "no analyst coverage", ""})
	} else {
		rows = append(rows, row{"Avg. Analyst Rating", rating.Rating, ""})
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

	if benchErr != nil {
		rows = append(rows, row{"Sector P/E", "error: " + benchErr.Error(), ""})
	} else {
		rows = append(rows, row{"Sector P/E", fmt.Sprintf("%.2f", bench.SectorPE), fmt.Sprintf("%.1f%% vs sector", bench.PEVsSectorPercent)})
		rows = append(rows, row{"Sector EV/EBITDA", fmt.Sprintf("%.2fx", bench.SectorEVToEBITDA), fmt.Sprintf("%.1f%% vs sector", bench.EVToEBITDAVsSectorPercent)})
	}

	if fcfErr != nil {
		rows = append(rows, row{"Free Cash Flow", "error: " + fcfErr.Error(), ""})
	} else {
		rows = append(rows, row{"Free Cash Flow", formatMoney(fcf.FCF, ccy), fcf.Interpretation})
	}

	if cfqErr != nil {
		rows = append(rows, row{"OCF / Net Income", "error: " + cfqErr.Error(), ""})
	} else {
		rows = append(rows, row{"Operating Cash Flow", formatMoney(cfq.OperatingCashFlow, ccy), ""})
		rows = append(rows, row{"Net Income", formatMoney(cfq.NetIncome, ccy), ""})
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
		rows = append(rows, row{"Market Cap", formatMoney(mc.MarketCap, priceCcy), mc.Interpretation})
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
		rows = append(rows, row{"Cash", formatMoney(cash.Cash, ccy), cash.Interpretation})
	}
	if debtErr != nil {
		rows = append(rows, row{"Debt", "error: " + debtErr.Error(), ""})
	} else {
		rows = append(rows, row{"Debt", formatMoney(debt.Debt, ccy), debt.Interpretation})
	}
	if cashErr == nil && debtErr == nil {
		net := cash.Cash - debt.Debt
		note := "net cash position (cash exceeds debt)"
		if net < 0 {
			note = "net debt position (debt exceeds cash)"
		}
		rows = append(rows, row{"Net", formatMoney(net, ccy), note})
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
		rows = append(rows, row{"Today", fmt.Sprintf("%+.2f%%", perf.Today), ""})
		rows = append(rows, row{"1-Week", fmt.Sprintf("%+.2f%%", perf.OneWeek), ""})
		rows = append(rows, row{"1-Month", fmt.Sprintf("%+.2f%%", perf.OneMonth), ""})
		rows = append(rows, row{"YTD", fmt.Sprintf("%+.2f%%", perf.YTD), ""})
		rows = append(rows, row{"1-Year", fmt.Sprintf("%+.2f%%", perf.OneYear), ""})
		rows = append(rows, row{"3-Year", fmt.Sprintf("%+.2f%%", perf.ThreeYear), ""})
		rows = append(rows, row{"5-Year", fmt.Sprintf("%+.2f%%", perf.FiveYear), ""})
		rows = append(rows, row{"10-Year", fmt.Sprintf("%+.2f%%", perf.TenYear), ""})
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
