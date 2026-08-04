// Command fetchquotes prints current prices for multiple tickers fetched in parallel.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
	symbols := []string{"AAPL", "BTC-USD", "BRK B"}
	if len(os.Args) > 1 {
		symbols = os.Args[1:]
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	prices, err := client.FetchQuotes(context.Background(), symbols)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", prices)
}
