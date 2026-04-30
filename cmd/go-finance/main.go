// Package main is the entry point for the go-finance CLI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

const usage = "usage: go-finance <TICKER> [--year YEAR [--month MONTH]]"

// parseArgs extracts ticker, --year and --month from args in any order.
func parseArgs(args []string) (ticker string, year, month int, err error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--year", "-year":
			if i+1 >= len(args) {
				return "", 0, 0, errors.New("--year requires a value")
			}
			i++
			year, err = strconv.Atoi(args[i])
			if err != nil {
				return "", 0, 0, fmt.Errorf("invalid year: %w", err)
			}
		case "--month", "-month":
			if i+1 >= len(args) {
				return "", 0, 0, errors.New("--month requires a value")
			}
			i++
			month, err = strconv.Atoi(args[i])
			if err != nil {
				return "", 0, 0, fmt.Errorf("invalid month: %w", err)
			}
		default:
			if strings.HasPrefix(args[i], "-") {
				return "", 0, 0, fmt.Errorf("unknown flag: %s", args[i])
			}
			if ticker != "" {
				return "", 0, 0, fmt.Errorf("unexpected argument: %s", args[i])
			}
			ticker = args[i]
		}
	}
	return
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New(usage)
	}

	ticker, year, month, err := parseArgs(args)
	if err != nil {
		return err
	}
	if ticker == "" {
		return errors.New(usage)
	}
	if month > 0 && year == 0 {
		return errors.New(usage)
	}

	client, err := yahoo.New()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	ctx := context.Background()
	switch {
	case year > 0 && month > 0:
		return runHistory(ctx, out, client, ticker, year, month)
	case year > 0:
		return runYearly(ctx, out, client, ticker, year)
	default:
		return runQuote(ctx, out, client, ticker)
	}
}

func runQuote(ctx context.Context, out io.Writer, client *yahoo.Client, ticker string) error {
	quote, err := client.GetQuote(ctx, ticker)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", ticker, err)
	}

	return json.NewEncoder(out).Encode(struct {
		Symbol   string  `json:"symbol"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
	}{quote.Symbol, quote.Price, quote.Currency})
}

func runHistory(ctx context.Context, out io.Writer, client *yahoo.Client, ticker string, year, month int) error {
	bar, err := client.GetMonthlyBar(ctx, ticker, year, month)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", ticker, err)
	}

	return json.NewEncoder(out).Encode(bar)
}

func runYearly(ctx context.Context, out io.Writer, client *yahoo.Client, ticker string, year int) error {
	bar, err := client.GetYearlyBar(ctx, ticker, year)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", ticker, err)
	}

	return json.NewEncoder(out).Encode(bar)
}
