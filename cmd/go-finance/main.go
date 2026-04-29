// Package main is the entry point for the go-finance CLI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: go-finance <TICKER>")
	}

	ticker := args[0]
	client, err := yahoo.New()
	if err != nil {
		return fmt.Errorf("creating client: %w", err)
	}

	quote, err := client.GetQuote(context.Background(), ticker)
	if err != nil {
		return fmt.Errorf("fetching %s: %w", ticker, err)
	}

	return json.NewEncoder(out).Encode(struct {
		Symbol   string  `json:"symbol"`
		Price    float64 `json:"price"`
		Currency string  `json:"currency"`
	}{quote.Symbol, quote.Price, quote.Currency})
}
