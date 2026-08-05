// Command performance prints YTD, 1-year, 3-year, and 5-year % price change for a ticker.
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
