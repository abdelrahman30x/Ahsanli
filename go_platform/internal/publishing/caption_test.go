package publishing

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestCaptionKeepsVerifiedFieldsWhenTitleIsLong(t *testing.T) {
	in := CaptionInput{StoreName: "أمازون مصر", Title: strings.Repeat("هاتف ", 300), Price: "3,485.00", CurrencyLabel: "جنيه", DropPercent: "10.4", DisplayedDiscount: "12", LimitedStock: 3, Seller: "Amazon.eg", ProductURL: "https://www.amazon.eg/dp/B0DN13WQHF?tag=gweliy-21"}
	caption, err := FormatCaption(in)
	if err != nil {
		t.Fatal(err)
	}
	if utf8.RuneCountInString(caption) > PhotoCaptionLimit || !strings.Contains(caption, "3,485.00") || !strings.Contains(caption, "12%") || !strings.Contains(caption, "آخر 3 قطع") || !strings.Contains(caption, in.ProductURL) {
		t.Fatalf("lost evidence or exceeded limit: %s", caption)
	}
}

func TestCaptionRejectsInvalidDestination(t *testing.T) {
	_, err := FormatCaption(CaptionInput{StoreName: "نون", Title: "منتج", Price: "100", Seller: "Shop", ProductURL: "http://noon.com/item"})
	if err == nil {
		t.Fatal("accepted HTTP destination")
	}
}
