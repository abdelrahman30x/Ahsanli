package capture

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ahsanly/go_platform/internal/fetch"
	"ahsanly/go_platform/internal/sources"
	"github.com/chromedp/chromedp"
)

type Box struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type regions struct {
	Cookie       bool   `json:"cookie"`
	Center       *Box   `json:"center"`
	Buy          *Box   `json:"buy"`
	Seller       *Box   `json:"seller"`
	Image        *Box   `json:"image"`
	Title        *Box   `json:"title"`
	Area         *Box   `json:"area"`
	Stock        *Box   `json:"stock"`
	StockText    string `json:"stockText"`
	DiscountText string `json:"discountText"`
	Obstructed   bool   `json:"obstructed"`
}

type Evidence struct {
	PriceCents        int64
	Seller            string
	DisplayedDiscount string
	LimitedStock      int
	Path              string
}

var productPath = regexp.MustCompile(`(?i)/dp/[A-Z0-9]{10}(?:/|$)`)
var stockPattern = regexp.MustCompile(`(?i)\bOnly\s+([1-9]|10)\s+left\s+in\s+stock\b`)
var arabicStockPattern = regexp.MustCompile(`(?:تبقى|تبقي|متبقي)\s+([0-9٠-٩]{1,2})\s+فقط`)
var discountPattern = regexp.MustCompile(`^\s*[-−]?\s*(\d{1,2}(?:[.,]\d+)?|100)\s*%\s*$`)

const regionScript = `(() => {
 const pick=(sels)=>sels.map(s=>document.querySelector(s)).find(e=>e && e.getBoundingClientRect().width>0 && e.getBoundingClientRect().height>0);
 const rect=(e)=>{if(!e)return null;const r=e.getBoundingClientRect();return {x:r.x,y:r.y,width:r.width,height:r.height}};
 const unobstructed=(e)=>{if(!e)return true;const r=e.getBoundingClientRect();const x=Math.max(0,Math.min(innerWidth-1,r.x+r.width/2));const y=Math.max(0,Math.min(innerHeight-1,r.y+r.height/2));const front=document.elementFromPoint(x,y);return front===e||e.contains(front)};
 const center=pick(['#corePriceDisplay_desktop_feature_div .priceToPay','#corePriceDisplay_desktop_feature_div .a-price:not(.a-text-price)','#corePriceDisplay_desktop_feature_div']);
 const buy=pick(['#corePrice_feature_div','#price_inside_buybox','#priceblock_ourprice','#priceblock_dealprice']);
 const seller=pick(['#desktop_buybox .offer-display-features-container','#tabular-buybox','#merchant-info']);
 const image=pick(['#imageBlock','#landingImage']);const title=pick(['#productTitle']);const area=pick(['#desktop_buybox','#rightCol']);
 const stock=pick(['#desktop_buybox #availability']);const discount=pick(['#corePriceDisplay_desktop_feature_div .savingsPercentage','#corePrice_feature_div .savingsPercentage']);
 const cookie=pick(['#sp-cc','#sp-cc-banner','[data-testid="cookie-banner"]']);
 return {cookie:!!cookie,center:rect(center),buy:rect(buy),seller:rect(seller),image:rect(image),title:rect(title),area:rect(area),stock:rect(stock),stockText:stock?.innerText||'',discountText:discount?.innerText||'',obstructed:![center,buy,seller].filter(Boolean).every(unobstructed)};
})()`

func normalizeSeller(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

func limitedStockCount(text string) int {
	match := stockPattern.FindStringSubmatch(text)
	if len(match) < 2 {
		match = arabicStockPattern.FindStringSubmatch(text)
	}
	if len(match) < 2 {
		return 0
	}
	var digits strings.Builder
	for _, r := range match[1] {
		if r >= '٠' && r <= '٩' {
			digits.WriteRune('0' + r - '٠')
		} else {
			digits.WriteRune(r)
		}
	}
	n, err := strconv.Atoi(digits.String())
	if err != nil || n < 1 || n > 10 {
		return 0
	}
	return n
}

func validProductURL(raw string) bool {
	if fetch.ValidateURL(raw, "amazon.eg") != nil {
		return false
	}
	u, err := url.Parse(raw)
	return err == nil && productPath.MatchString(u.Path)
}

// CaptureAmazon opens Chromium only after an HTTP observation already
// qualifies as a candidate. The screenshot is rejected on any mismatch.
func CaptureAmazon(parent context.Context, browserPath, rawURL, output string, expected sources.Offer) (Evidence, error) {
	if !validProductURL(rawURL) {
		return Evidence{}, errors.New("not an Amazon product URL")
	}
	if browserPath == "" || expected.PriceCents <= 0 || strings.TrimSpace(expected.Seller) == "" {
		return Evidence{}, errors.New("missing browser or expected offer")
	}
	options := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	options = append(options, chromedp.ExecPath(browserPath), chromedp.Flag("no-sandbox", true), chromedp.Flag("disable-dev-shm-usage", true), chromedp.Flag("disable-gpu", true), chromedp.Flag("disable-extensions", true))
	alloc, stopAlloc := chromedp.NewExecAllocator(parent, options...)
	defer stopAlloc()
	ctx, stopBrowser := chromedp.NewContext(alloc)
	defer stopBrowser()
	ctx, cancel := context.WithTimeout(ctx, 50*time.Second)
	defer cancel()
	var html string
	var shot []byte
	var layout regions
	var cookieAfter bool
	err := chromedp.Run(ctx,
		chromedp.EmulateViewport(1440, 900),
		chromedp.Navigate(rawURL),
		chromedp.WaitVisible(`#landingImage`, chromedp.ByQuery),
		chromedp.WaitVisible(`#productTitle`, chromedp.ByQuery),
		chromedp.Evaluate(`(() => { const n=document.querySelector('#sp-cc-rejectall-link'); if(n){n.click();return true} return false })()`, nil),
		chromedp.Sleep(500*time.Millisecond),
		chromedp.OuterHTML("html", &html),
		chromedp.Evaluate(regionScript, &layout),
		chromedp.CaptureScreenshot(&shot),
		chromedp.Evaluate(`(() => { const e=document.querySelector('#sp-cc,#sp-cc-banner,[data-testid="cookie-banner"]'); return !!e && e.getBoundingClientRect().height>0 && e.getBoundingClientRect().width>0 })()`, &cookieAfter),
	)
	if err != nil {
		return Evidence{}, fmt.Errorf("browser capture failed: %w", err)
	}
	if layout.Cookie || cookieAfter || layout.Obstructed {
		return Evidence{}, errors.New("cookie notice or overlay obscures evidence")
	}
	if (layout.Center == nil && layout.Buy == nil) || layout.Seller == nil || layout.Image == nil || layout.Title == nil || layout.Area == nil {
		return Evidence{}, errors.New("price, seller, or product region missing")
	}
	parsed, err := sources.ParseOffer("amazon_eg", "", []byte(html))
	if err != nil {
		return Evidence{}, err
	}
	if parsed.PriceCents != expected.PriceCents || normalizeSeller(parsed.Seller) != normalizeSeller(expected.Seller) {
		return Evidence{}, errors.New("screenshot price or seller differs from observation")
	}
	stock := limitedStockCount(layout.StockText)
	discount := ""
	if match := discountPattern.FindStringSubmatch(layout.DiscountText); len(match) > 1 {
		if n, e := strconv.ParseFloat(strings.ReplaceAll(match[1], ",", "."), 64); e == nil && n > 0 && n <= 100 {
			discount = fmt.Sprintf("%g", n)
		}
	}
	boxes := []*Box{layout.Image, layout.Title, layout.Area, layout.Center, layout.Buy, layout.Seller}
	if stock > 0 {
		boxes = append(boxes, layout.Stock)
	}
	cropped, err := annotateAndCrop(shot, boxes, []*Box{layout.Center, layout.Buy, layout.Seller}, func() *Box {
		if stock > 0 {
			return layout.Stock
		}
		return nil
	}())
	if err != nil {
		return Evidence{}, err
	}
	if err := os.MkdirAll(filepath.Dir(output), 0700); err != nil {
		return Evidence{}, err
	}
	temp := output + ".tmp"
	if err := os.WriteFile(temp, cropped, 0600); err != nil {
		return Evidence{}, err
	}
	if err := os.Rename(temp, output); err != nil {
		os.Remove(temp)
		return Evidence{}, err
	}
	return Evidence{PriceCents: parsed.PriceCents, Seller: parsed.Seller, DisplayedDiscount: discount, LimitedStock: stock, Path: output}, nil
}

func annotateAndCrop(raw []byte, bounds []*Box, outlines []*Box, stock *Box) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	viewport := src.Bounds()
	left, top, right, bottom := viewport.Max.X, viewport.Max.Y, 0, 0
	for _, b := range bounds {
		if b == nil {
			continue
		}
		if b.Width <= 0 || b.Height <= 0 {
			return nil, errors.New("invalid evidence region")
		}
		left = min(left, int(math.Floor(b.X))-18)
		top = min(top, int(math.Floor(b.Y))-12)
		right = max(right, int(math.Ceil(b.X+b.Width))+18)
		bottom = max(bottom, int(math.Ceil(b.Y+b.Height))+16)
	}
	crop := image.Rect(max(0, left), max(0, top), min(viewport.Max.X, right), min(viewport.Max.Y, bottom))
	if crop.Empty() {
		return nil, errors.New("invalid product crop")
	}
	for _, b := range outlines {
		if b != nil && (b.X < 0 || b.Y < 0 || b.X+b.Width > float64(viewport.Dx()) || b.Y+b.Height > float64(viewport.Dy())) {
			return nil, errors.New("outlined evidence outside viewport")
		}
	}
	if stock != nil && (stock.X < 0 || stock.Y < 0 || stock.X+stock.Width > float64(viewport.Dx()) || stock.Y+stock.Height > float64(viewport.Dy())) {
		return nil, errors.New("stock evidence outside viewport")
	}
	dst := image.NewRGBA(image.Rect(0, 0, crop.Dx(), crop.Dy()))
	draw.Draw(dst, dst.Bounds(), src, crop.Min, draw.Src)
	for _, b := range append(outlines, stock) {
		if b == nil {
			continue
		}
		r := image.Rect(int(b.X)-crop.Min.X-4, int(b.Y)-crop.Min.Y-4, int(math.Ceil(b.X+b.Width))-crop.Min.X+4, int(math.Ceil(b.Y+b.Height))-crop.Min.Y+4)
		drawRedBox(dst, r)
	}
	var output bytes.Buffer
	if err := png.Encode(&output, dst); err != nil {
		return nil, err
	}
	return output.Bytes(), nil
}

func drawRedBox(dst *image.RGBA, r image.Rectangle) {
	red := color.RGBA{R: 220, A: 255}
	r = r.Intersect(dst.Bounds())
	for y := r.Min.Y; y < r.Max.Y; y++ {
		for x := r.Min.X; x < r.Max.X; x++ {
			if x-r.Min.X < 3 || r.Max.X-1-x < 3 || y-r.Min.Y < 3 || r.Max.Y-1-y < 3 {
				dst.Set(x, y, red)
			}
		}
	}
}
