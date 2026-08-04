// Command monthlybar prints OHLC data for a ticker in a given month (v7 endpoint, requires crumb).
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
	year, month := 2024, 3
	if len(os.Args) > 3 {
		ticker = os.Args[1]
		year, _ = strconv.Atoi(os.Args[2])
		month, _ = strconv.Atoi(os.Args[3])
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	bar, err := client.GetMonthlyBar(context.Background(), ticker, year, month)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", bar)
}
