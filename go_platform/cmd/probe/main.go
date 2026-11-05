// probe performs one read-only retailer HTTP check for connector diagnostics.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"ahsanly/go_platform/internal/fetch"
	"ahsanly/go_platform/internal/sources"
)

func main() {
	retailer := flag.String("retailer", "amazon_eg", "retailer code")
	productID := flag.String("id", "", "retailer product ID")
	rawURL := flag.String("url", "", "HTTPS product URL")
	flag.Parse()
	if *rawURL == "" || *productID == "" {
		fmt.Fprintln(os.Stderr, "-url and -id are required")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	client := fetch.NewClient(fetch.Policy{MaxPerHour: 2, MinInterval: 30 * time.Second, Timeout: 15 * time.Second, UseHTTP2: *retailer == "jumia_eg"})
	start := time.Now()
	body, fetchErr := client.GetHTML(ctx, *rawURL, map[string]string{"amazon_eg": "amazon.eg", "noon_eg": "noon.com", "jumia_eg": "jumia.com.eg"}[*retailer])
	if fetchErr == nil && os.Getenv("AHSANLY_SAVE_BODY") != "" {
		_ = os.WriteFile(os.Getenv("AHSANLY_SAVE_BODY"), body, 0644)
	}
	offer, err := sources.ParseOffer(*retailer, *productID, body)
	if fetchErr != nil {
		err = fetchErr
	}
	if err != nil {
		class := "network_or_parser_error"
		switch {
		case errors.Is(err, fetch.ErrBlocked):
			class = "blocked"
		case errors.Is(err, sources.ErrPrice):
			class = "missing_price"
		case errors.Is(err, sources.ErrSeller):
			class = "missing_seller"
		case errors.Is(err, context.DeadlineExceeded):
			class = "timeout"
		}
		fmt.Printf("result=failure class=%s error=%v elapsed=%s\n", class, err, time.Since(start).Round(time.Millisecond))
		os.Exit(1)
	}
	fmt.Printf("result=valid price=%s currency=%s seller=%q title=%q elapsed=%s\n", offer.PriceString(), offer.Currency, offer.Seller, offer.Title, time.Since(start).Round(time.Millisecond))
}
