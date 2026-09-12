// Command sectorbenchmark prints a stock's P/E and EV/EBITDA against its sector benchmark.
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

	b, err := client.GetSectorBenchmark(context.Background(), ticker)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", b)
}
