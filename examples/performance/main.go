// Command performance prints today, 1-week, 1-month, YTD, 1-year, 3-year, 5-year, and 10-year % price change for a ticker.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
	ticker := "AAPL"
	if len(os.Args) > 1 {
		ticker = os.Args[1]
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	perf, err := client.FetchPerformance(context.Background(), ticker)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", perf)
}
