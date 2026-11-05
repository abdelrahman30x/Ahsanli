package sources

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"ahsanly/go_platform/internal/fetch"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "..", "tests", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestPrices(t *testing.T) {
	for input, want := range map[string]int64{"EGP 1,299.00": 129900, "١٬٢٩٩٫٥٠ جنيه": 129950, "1.299,50": 129950, "315": 31500} {
		got, err := ParsePrice(input)
		if err != nil || got != want {
			t.Errorf("%q: %d %v, want %d", input, got, err, want)
		}
	}
}

func TestAmazonCurrentPriceAndSeller(t *testing.T) {
	o, err := ParseOffer("amazon_eg", "B000000001", fixture(t, "product_visible.html"))
	if err != nil {
		t.Fatal(err)
	}
	if o.PriceCents != 129900 || o.Seller != "ShopOne" || o.Title != "Sample Headphones" {
		t.Fatalf("wrong offer: %+v", o)
	}
}

func TestChallengeRejected(t *testing.T) {
	_, err := ParseOffer("amazon_eg", "", fixture(t, "robot_check.html"))
	if !errors.Is(err, fetch.ErrBlocked) {
		t.Fatalf("challenge: %v", err)
	}
}

func TestNoonSKU(t *testing.T) {
	body := fixture(t, "noon_product_jsonld.html")
	o, err := ParseOffer("noon_eg", "N39519778A", body)
	if err != nil || o.PriceCents != 31500 || o.Seller != "X-Tech" {
		t.Fatalf("offer: %+v %v", o, err)
	}
	_, err = ParseOffer("noon_eg", "N00000000A", body)
	if !errors.Is(err, ErrPrice) {
		t.Fatalf("mismatched SKU accepted: %v", err)
	}
}
