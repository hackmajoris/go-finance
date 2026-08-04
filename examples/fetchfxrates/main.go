// Command fetchfxrates prints spot FX rates for currencies relative to a base currency.
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

func main() {
	base := "USD"
	currencies := []string{"EUR", "RON", "USD"}
	if len(os.Args) > 2 {
		base = os.Args[1]
		currencies = os.Args[2:]
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	rates, err := client.FetchFXRates(context.Background(), currencies, base)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", rates)
}
