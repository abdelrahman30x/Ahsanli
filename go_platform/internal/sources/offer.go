package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"ahsanly/go_platform/internal/fetch"
	"github.com/PuerkitoBio/goquery"
)

var (
	ErrPrice       = errors.New("price missing or invalid")
	ErrSeller      = errors.New("seller missing")
	ErrCurrency    = errors.New("currency mismatch")
	ErrUnavailable = errors.New("product unavailable")
	pricePattern   = regexp.MustCompile(`[0-9٠-٩۰-۹][0-9٠-٩۰-۹.,٬٫\s]*`)
)

type Offer struct {
	Title      string
	PriceCents int64
	Currency   string
	Seller     string
	Stock      string
	ImageURL   string
}

func (o Offer) PriceString() string {
	return fmt.Sprintf("%d.%02d", o.PriceCents/100, o.PriceCents%100)
}

func ParsePrice(s string) (int64, error) {
	match := pricePattern.FindString(s)
	if match == "" {
		return 0, ErrPrice
	}
	var b strings.Builder
	for _, r := range match {
		switch {
		case r >= '٠' && r <= '٩':
			b.WriteRune('0' + r - '٠')
		case r >= '۰' && r <= '۹':
			b.WriteRune('0' + r - '۰')
		case r == '٬':
			b.WriteByte(',')
		case r == '٫':
			b.WriteByte('.')
		case r == ' ' || r == '\u00a0':
		default:
			b.WriteRune(r)
		}
	}
	n := b.String()
	lastDot, lastComma := strings.LastIndexByte(n, '.'), strings.LastIndexByte(n, ',')
	sep := -1
	if lastDot > lastComma {
		sep = lastDot
	} else {
		sep = lastComma
	}
	decimals := ""
	if sep >= 0 && len(n)-sep-1 <= 2 && len(n)-sep-1 > 0 {
		decimals = n[sep+1:]
	}
	whole := n
	if decimals != "" {
		whole = n[:sep]
	}
	whole = strings.NewReplacer(",", "", ".", "").Replace(whole)
	if whole == "" {
		return 0, ErrPrice
	}
	major, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || major < 0 || major > 100_000_000 {
		return 0, ErrPrice
	}
	minor := int64(0)
	if decimals != "" {
		if len(decimals) == 1 {
			decimals += "0"
		}
		minor, err = strconv.ParseInt(decimals, 10, 64)
		if err != nil {
			return 0, ErrPrice
		}
	}
	cents := major*100 + minor
	if cents <= 0 {
		return 0, ErrPrice
	}
	return cents, nil
}

func ParseOffer(retailer, productID string, body []byte) (Offer, error) {
	lower := strings.ToLower(string(body))
	if strings.Contains(lower, "captcha") || strings.Contains(lower, "robot check") || strings.Contains(lower, "access denied") {
		return Offer{}, fetch.ErrBlocked
	}
	doc, err := goquery.NewDocumentFromReader(bytes.NewReader(body))
	if err != nil {
		return Offer{}, err
	}
	switch retailer {
	case "amazon_eg":
		return parseAmazon(doc)
	case "noon_eg":
		return parseNoon(doc, productID)
	case "jumia_eg":
		return parseJumia(doc)
	default:
		return Offer{}, errors.New("unsupported retailer")
	}
}

func firstText(doc *goquery.Document, selectors ...string) string {
	for _, s := range selectors {
		v := strings.Join(strings.Fields(doc.Find(s).First().Text()), " ")
		if v != "" {
			return v
		}
	}
	return ""
}

func parseAmazon(doc *goquery.Document) (Offer, error) {
	if firstText(doc, "#outOfStock", "#outOfStockBuyBox_feature_div") != "" && doc.Find("#add-to-cart-button,#buy-now-button").Length() == 0 {
		return Offer{}, ErrUnavailable
	}
	title := firstText(doc, "#productTitle", "h1")
	priceText := firstText(doc,
		"#corePrice_feature_div .priceToPay .a-offscreen",
		"#corePriceDisplay_desktop_feature_div .priceToPay .a-offscreen",
		"#apex_desktop .priceToPay .a-offscreen",
		"#corePrice_feature_div .a-price:not(.a-text-price) .a-offscreen",
		"#priceblock_dealprice", "#priceblock_ourprice", "#price_inside_buybox")
	if priceText == "" {
		return Offer{}, ErrPrice
	}
	if !strings.Contains(strings.ToUpper(priceText), "EGP") && !strings.Contains(priceText, "جنيه") {
		currency := doc.Find(`meta[property="product:price:currency"]`).AttrOr("content", "")
		if currency != "EGP" {
			return Offer{}, ErrCurrency
		}
	}
	price, err := ParsePrice(priceText)
	if err != nil {
		return Offer{}, err
	}
	seller := firstText(doc, "#sellerProfileTriggerId", `.offer-display-feature-text[offer-display-feature-name="desktop-merchant-info"]`)
	if seller == "" {
		doc.Find("#tabular-buybox .a-row,#merchant-info").EachWithBreak(func(_ int, row *goquery.Selection) bool {
			v := strings.Join(strings.Fields(row.Text()), " ")
			if strings.Contains(strings.ToLower(v), "sold by") || strings.Contains(v, "البائع") {
				seller = strings.TrimSpace(strings.TrimPrefix(v, "Sold by"))
				return false
			}
			return true
		})
	}
	if seller == "" {
		return Offer{}, ErrSeller
	}
	image := doc.Find("#landingImage").AttrOr("data-old-hires", "")
	if image == "" {
		image = doc.Find("#landingImage,#imgTagWrapperId img").First().AttrOr("src", "")
	}
	if image == "" {
		image = doc.Find(`meta[property="og:image"]`).AttrOr("content", "")
	}
	return Offer{Title: title, PriceCents: price, Currency: "EGP", Seller: seller, Stock: "in_stock", ImageURL: image}, nil
}

func parseNoon(doc *goquery.Document, expectedID string) (Offer, error) {
	var found Offer
	var parseErr error
	doc.Find(`script[type="application/ld+json"]`).EachWithBreak(func(_ int, s *goquery.Selection) bool {
		var node any
		if json.Unmarshal([]byte(s.Text()), &node) != nil {
			return true
		}
		for _, product := range productNodes(node) {
			id := asString(product["sku"])
			if expectedID != "" && id != "" && !strings.EqualFold(id, expectedID) {
				continue
			}
			offers, ok := product["offers"].(map[string]any)
			if !ok {
				continue
			}
			if strings.Contains(strings.ToLower(asString(offers["availability"])), "outofstock") {
				parseErr = ErrUnavailable
				return false
			}
			if asString(offers["priceCurrency"]) != "EGP" {
				parseErr = ErrCurrency
				return false
			}
			price, err := ParsePrice(asString(offers["price"]))
			if err != nil {
				parseErr = err
				return false
			}
			seller := ""
			if m, ok := offers["seller"].(map[string]any); ok {
				seller = asString(m["name"])
			}
			if seller == "" {
				parseErr = ErrSeller
				return false
			}
			found = Offer{Title: asString(product["name"]), PriceCents: price, Currency: "EGP", Seller: seller, Stock: "in_stock"}
			return false
		}
		return true
	})
	if parseErr != nil {
		return Offer{}, parseErr
	}
	if found.PriceCents == 0 {
		return Offer{}, ErrPrice
	}
	return found, nil
}

func productNodes(root any) []map[string]any {
	var out []map[string]any
	var walk func(any)
	walk = func(v any) {
		switch node := v.(type) {
		case []any:
			for _, child := range node {
				walk(child)
			}
		case map[string]any:
			if strings.EqualFold(asString(node["@type"]), "Product") {
				out = append(out, node)
			}
			for _, key := range []string{"@graph", "mainEntity"} {
				if child, ok := node[key]; ok {
					walk(child)
				}
			}
		}
	}
	walk(root)
	return out
}

func asString(v any) string {
	switch x := v.(type) {
	case string:
		return strings.TrimSpace(x)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return ""
	}
}

func parseJumia(doc *goquery.Document) (Offer, error) {
	title := firstText(doc, "h1")
	priceText := firstText(doc, "#jm .-fs24", ".-prxs .-fs24", ".-fs24.-b", `[data-testid="pdp-price"]`)
	if title == "" || priceText == "" {
		return Offer{}, ErrPrice
	}
	if doc.Find("#add-to-cart,button.add").Length() == 0 {
		return Offer{}, ErrUnavailable
	}
	price, err := ParsePrice(priceText)
	if err != nil {
		return Offer{}, err
	}
	seller := firstText(doc, "#seller .-m", "#seller .-b", ".seller-name", `[data-testid="seller-name"]`)
	if seller == "" {
		return Offer{}, ErrSeller
	}
	return Offer{Title: title, PriceCents: price, Currency: "EGP", Seller: seller, Stock: "in_stock"}, nil
}

func FetchOffer(ctx context.Context, client *fetch.Client, retailer, productID, rawURL string) (Offer, error) {
	domains := map[string]string{"amazon_eg": "amazon.eg", "noon_eg": "noon.com", "jumia_eg": "jumia.com.eg"}
	domain := domains[retailer]
	if domain == "" {
		return Offer{}, errors.New("unsupported retailer")
	}
	body, err := client.GetHTML(ctx, rawURL, domain)
	if err != nil {
		return Offer{}, err
	}
	return ParseOffer(retailer, productID, body)
}
