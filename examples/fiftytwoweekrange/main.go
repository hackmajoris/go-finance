// Command fiftytwoweekrange prints the 52-week high/low for a ticker,
// with today's price plotted between them.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/hackmajoris/go-finance/pkg/yahoo"
)

const barWidth = 30

func main() {
	ticker := "AAPL"
	if len(os.Args) > 1 {
		ticker = os.Args[1]
	}

	client, err := yahoo.New()
	if err != nil {
		log.Fatal(err)
	}

	rng, err := client.FetchFiftyTwoWeekRange(context.Background(), ticker)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Printf("%+v\n", rng)

	pos := int(rng.Pct * float64(barWidth-1))
	bar := strings.Repeat("─", pos) + "●" + strings.Repeat("─", barWidth-1-pos)
	fmt.Printf("%.2f [%s] %.2f  (%.0f%%)\n", rng.Low, bar, rng.High, rng.Pct*100)
}
