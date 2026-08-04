// Command quote prints the current price for a ticker.
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

	quote, err := client.GetQuote(context.Background(), ticker)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", quote)
}
