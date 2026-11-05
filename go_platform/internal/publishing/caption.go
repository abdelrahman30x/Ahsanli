package publishing

import (
	"errors"
	"fmt"
	"net/url"
	"strings"
	"unicode/utf8"
)

const PhotoCaptionLimit = 1024

type CaptionInput struct {
	StoreName         string
	Title             string
	Price             string
	CurrencyLabel     string
	DropPercent       string
	DisplayedDiscount string
	LimitedStock      int
	Seller            string
	ProductURL        string
}

// FormatCaption only shortens the title. Price, evidence-based percentages,
// seller and the destination URL must remain intact for every photo post.
func FormatCaption(in CaptionInput) (string, error) {
	u, err := url.Parse(in.ProductURL)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil {
		return "", errors.New("invalid product URL")
	}
	if strings.TrimSpace(in.Price) == "" || strings.TrimSpace(in.Seller) == "" || strings.TrimSpace(in.StoreName) == "" {
		return "", errors.New("incomplete verified offer")
	}
	title := strings.TrimSpace(in.Title)
	if title == "" {
		return "", errors.New("missing title")
	}
	lines := []string{fmt.Sprintf("🔥 عرض على %s!", in.StoreName), title, "", fmt.Sprintf("💰 سعر العرض: %s %s", in.Price, in.CurrencyLabel), "", "أحسنلي | أحسن عرض.. أحسن سعر"}
	if in.DropPercent != "" {
		lines = append(lines, fmt.Sprintf("📉 أقل من آخر سعر رصدناه بنسبة %s%%", in.DropPercent))
	}
	if in.DisplayedDiscount != "" {
		lines = append(lines, fmt.Sprintf("🏷️ خصم المتجر المعروض: %s%%", in.DisplayedDiscount))
	}
	if in.LimitedStock > 0 {
		lines = append(lines, fmt.Sprintf("⚠️ آخر %d قطع", in.LimitedStock))
	}
	lines = append(lines, "", fmt.Sprintf("🏪 البائع: %s", in.Seller), fmt.Sprintf("🛒 %s", in.ProductURL))
	caption := strings.Join(lines, "\n")
	if utf8.RuneCountInString(caption) <= PhotoCaptionLimit {
		return caption, nil
	}
	over := utf8.RuneCountInString(caption) - PhotoCaptionLimit
	runes := []rune(title)
	keep := len(runes) - over - 1
	if keep < 16 {
		return "", errors.New("caption cannot fit verified offer")
	}
	lines[1] = strings.TrimSpace(string(runes[:keep])) + "…"
	caption = strings.Join(lines, "\n")
	if utf8.RuneCountInString(caption) > PhotoCaptionLimit {
		return "", errors.New("caption exceeds photo limit")
	}
	return caption, nil
}
