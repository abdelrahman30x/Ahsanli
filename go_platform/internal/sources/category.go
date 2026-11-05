package sources

import (
	"bytes"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"ahsanly/go_platform/internal/fetch"
	"github.com/PuerkitoBio/goquery"
)

var asinPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

type DiscoveredListing struct {
	ProductID string
	URL       string
	Title     string
	ImageURL  string
}

type ListingPage struct {
	Listings []DiscoveredListing
	NextURL  string
}

// ParseAmazonCategory extracts product identities only. Search-result prices
// are never written as verified offer observations.
func ParseAmazonCategory(body []byte, currentURL string) (ListingPage, error) {
	if err := fetch.ValidateURL(currentURL, "amazon.eg"); err != nil {
		return ListingPage{}, err
	}
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "captcha") || strings.Contains(lower, "robot check") {
		return ListingPage{}, fetch.ErrBlocked
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return ListingPage{}, err
	}
	page := ListingPage{}
	seen := map[string]bool{}
	doc.Find(`[data-component-type="s-search-result"][data-asin]`).Each(func(_ int, card *goquery.Selection) {
		id := strings.ToUpper(strings.TrimSpace(card.AttrOr("data-asin", "")))
		if !asinPattern.MatchString(id) || seen[id] {
			return
		}
		if card.Find(".puis-sponsored-label-text,.s-sponsored-label-info-icon").Length() > 0 {
			return
		}
		title := strings.Join(strings.Fields(card.Find("h2").First().Text()), " ")
		if title == "" {
			return
		}
		image := card.Find("img.s-image").First().AttrOr("src", "")
		seen[id] = true
		page.Listings = append(page.Listings, DiscoveredListing{ProductID: id, URL: "https://www.amazon.eg/dp/" + id, Title: title, ImageURL: image})
	})
	if len(page.Listings) == 0 {
		return ListingPage{}, errors.New("no product identities found")
	}
	next := doc.Find("a.s-pagination-next:not(.s-pagination-disabled)").First().AttrOr("href", "")
	if next != "" {
		base, _ := url.Parse(currentURL)
		rel, err := url.Parse(next)
		if err != nil {
			return ListingPage{}, err
		}
		resolved := base.ResolveReference(rel).String()
		if err = fetch.ValidateURL(resolved, "amazon.eg"); err != nil {
			return ListingPage{}, err
		}
		if resolved == currentURL {
			return ListingPage{}, errors.New("category cursor did not advance")
		}
		page.NextURL = resolved
	}
	return page, nil
}
