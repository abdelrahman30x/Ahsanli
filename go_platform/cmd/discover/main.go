// Command discover performs one read-only Amazon category probe. It neither
// changes the database nor schedules follow-up requests.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"ahsanly/go_platform/internal/fetch"
	"ahsanly/go_platform/internal/sources"
)

func main() {
	raw := flag.String("url", "", "Amazon Egypt category URL")
	flag.Parse()
	if err := fetch.ValidateURL(*raw, "amazon.eg"); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	client := fetch.NewClient(fetch.Policy{MaxPerHour: 1, MinInterval: time.Second, Timeout: 20 * time.Second})
	body, err := client.GetHTML(ctx, *raw, "amazon.eg")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	page, err := sources.ParseAmazonCategory(body, *raw)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Printf("products=%d next=%t\n", len(page.Listings), page.NextURL != "")
	for i, p := range page.Listings {
		if i >= 3 {
			break
		}
		fmt.Println(p.ProductID)
	}
}
