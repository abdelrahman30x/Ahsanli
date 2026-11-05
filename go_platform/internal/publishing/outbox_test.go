package publishing

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestEvidenceMustStayInsideCaptureDirectory(t *testing.T) {
	dir := t.TempDir()
	inside := filepath.Join(dir, "candidate-1.png")
	photo, err := os.OpenFile(inside, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(photo, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		photo.Close()
		t.Fatal(err)
	}
	if err = photo.Close(); err != nil {
		t.Fatal(err)
	}
	if err := evidenceFile(dir, inside); err != nil {
		t.Fatalf("valid evidence rejected: %v", err)
	}
	invalid := filepath.Join(dir, "invalid.png")
	if err := os.WriteFile(invalid, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := evidenceFile(dir, invalid); err == nil {
		t.Fatal("accepted malformed evidence")
	}
	outside := filepath.Join(t.TempDir(), "other.png")
	if err := os.WriteFile(outside, []byte("image"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := evidenceFile(dir, outside); err == nil {
		t.Fatal("accepted evidence outside capture dir")
	}
}

func TestFormatPriceForArabicCaption(t *testing.T) {
	if got := formatPrice("3485.00"); got != "3,485.00" {
		t.Fatalf("got %q", got)
	}
}
