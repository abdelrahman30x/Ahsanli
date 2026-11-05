package sources

import "testing"

func TestAmazonCategoryIdentityAndCursor(t *testing.T) {
	html := `<div data-component-type="s-search-result" data-asin="B0CPT6RBQY"><h2>Headphones</h2><img class="s-image" src="https://m.media-amazon.com/test.jpg"></div>
<div data-component-type="s-search-result" data-asin="B0CPT6RBQY"><h2>Duplicate</h2></div>
<div data-component-type="s-search-result" data-asin="B0DN13WQHF"><span class="puis-sponsored-label-text">Sponsored</span><h2>Ad</h2></div>
<a class="s-pagination-next" href="/s?page=2">Next</a>`
	p, err := ParseAmazonCategory([]byte(html), "https://www.amazon.eg/s?page=1")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Listings) != 1 || p.Listings[0].ProductID != "B0CPT6RBQY" || p.NextURL != "https://www.amazon.eg/s?page=2" {
		t.Fatalf("unexpected page: %+v", p)
	}
}

func TestAmazonCategoryRejectsCrossHostNextPage(t *testing.T) {
	html := `<div data-component-type="s-search-result" data-asin="B0CPT6RBQY"><h2>Headphones</h2></div><a class="s-pagination-next" href="https://example.com/s?page=2">Next</a>`
	if _, err := ParseAmazonCategory([]byte(html), "https://www.amazon.eg/s?page=1"); err == nil {
		t.Fatal("accepted foreign next-page URL")
	}
}
