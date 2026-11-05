package capture

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func TestAnnotateAndCropDrawsPriceAndSellerBoxes(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 200, 120))
	for y := 0; y < 120; y++ {
		for x := 0; x < 200; x++ {
			src.Set(x, y, color.RGBA{R: 255, G: 255, B: 255, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatal(err)
	}
	imageBox := &Box{X: 10, Y: 10, Width: 60, Height: 80}
	title := &Box{X: 80, Y: 10, Width: 70, Height: 20}
	price := &Box{X: 80, Y: 40, Width: 40, Height: 20}
	seller := &Box{X: 145, Y: 40, Width: 40, Height: 30}
	out, err := annotateAndCrop(raw.Bytes(), []*Box{imageBox, title, price, seller}, []*Box{price, seller}, nil)
	if err != nil {
		t.Fatal(err)
	}
	result, err := png.Decode(bytes.NewReader(out))
	if err != nil {
		t.Fatal(err)
	}
	if result.Bounds().Dx() != 200 || result.Bounds().Dy() != 106 {
		t.Fatalf("unexpected crop: %v", result.Bounds())
	}
	red := result.At(76, 36)
	r, g, b, _ := red.RGBA()
	if r <= g || r <= b {
		t.Fatalf("price box border missing: %v", red)
	}
}

func TestAnnotateRejectsOffscreenEvidence(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 100, 100))
	var raw bytes.Buffer
	png.Encode(&raw, src)
	box := &Box{X: 95, Y: 10, Width: 20, Height: 20}
	if _, err := annotateAndCrop(raw.Bytes(), []*Box{box}, []*Box{box}, nil); err == nil {
		t.Fatal("accepted offscreen seller")
	}
}

func TestProductURLAllowsAffiliateQueryButRejectsOtherHost(t *testing.T) {
	if !validProductURL("https://www.amazon.eg/dp/B0CPT6RBQY?tag=gweliy-21") {
		t.Fatal("rejected valid product URL")
	}
	if validProductURL("https://example.com/dp/B0CPT6RBQY") {
		t.Fatal("accepted foreign host")
	}
}

func TestLimitedStockEnglishAndArabic(t *testing.T) {
	for _, tc := range []struct {
		text string
		want int
	}{
		{"Only 3 left in stock - order soon.", 3},
		{"تبقى 1 فقط - اطلبه الآن.", 1},
		{"متبقي ٣ فقط", 3},
		{"Only 25 left in stock", 0},
	} {
		if got := limitedStockCount(tc.text); got != tc.want {
			t.Fatalf("%q: got %d want %d", tc.text, got, tc.want)
		}
	}
}
