// Command yearlybar prints OHLC data for a ticker aggregated over a year.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
	ticker := "AAPL"
	year := 2024
	if len(os.Args) > 2 {
		ticker = os.Args[1]
		year, _ = strconv.Atoi(os.Args[2])
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	bar, err := client.GetYearlyBar(context.Background(), ticker, year)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", bar)
}
