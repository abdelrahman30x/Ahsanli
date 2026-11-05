// Command capture is a read-only screenshot diagnostic. It never publishes.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"ahsanly/go_platform/internal/capture"
	"ahsanly/go_platform/internal/sources"
)

func main() {
	browser := flag.String("browser", "", "Chromium executable")
	raw := flag.String("url", "", "Amazon Egypt product URL")
	price := flag.String("price", "", "Expected directly observed EGP price")
	seller := flag.String("seller", "", "Expected directly observed seller")
	output := flag.String("output", "", "PNG output path")
	flag.Parse()
	if *browser == "" || *raw == "" || *seller == "" || *output == "" {
		flag.Usage()
		os.Exit(2)
	}
	cents, err := sources.ParsePrice(*price)
	if err != nil {
		fmt.Fprintln(os.Stderr, "invalid expected price")
		os.Exit(2)
	}
	evidence, err := capture.CaptureAmazon(context.Background(), *browser, *raw, *output, sources.Offer{PriceCents: cents, Currency: "EGP", Seller: *seller})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("verified price=%d.%02d seller=%q stock=%d discount=%q screenshot=%s\n", evidence.PriceCents/100, evidence.PriceCents%100, evidence.Seller, evidence.LimitedStock, evidence.DisplayedDiscount, evidence.Path)
}
