// Command fetchmonthlybar prints the closing price for a ticker in a given month
// using the v8 endpoint (no crumb required, works in Docker/containers).
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
	ticker := "^GSPC"
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

	closePrice, err := client.FetchMonthlyBar(context.Background(), ticker, year, month)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(closePrice)
}
