package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"ahsanly/go_platform/internal/admin"
	"ahsanly/go_platform/internal/catalog"
	"github.com/jackc/pgx/v5"
)

type API struct{ Catalog *catalog.Store }

var storeCode = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)
var categoryCode = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func contextWithTimeout(r *http.Request, d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), d)
}

func (a API) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", a.health)
	mux.HandleFunc("GET /api/v1/meta", a.meta)
	// Legacy unscoped endpoints: default to Egypt while it is the only launched country.
	mux.HandleFunc("GET /api/v1/products", a.productsLegacy)
	mux.HandleFunc("GET /api/v1/products/{id}", a.productLegacy)
	// Country-scoped endpoints.
	mux.HandleFunc("GET /api/v1/countries/{code}/meta", a.countryMeta)
	mux.HandleFunc("GET /api/v1/countries/{code}/categories", a.countryCategories)
	mux.HandleFunc("GET /api/v1/countries/{code}/products", a.countryProducts)
	mux.HandleFunc("GET /api/v1/countries/{code}/products/{id}", a.countryProduct)
	mux.HandleFunc("GET /api/v1/countries/{code}/deals", a.countryDeals)
	mux.HandleFunc("GET /api/v1/countries/{code}/deals/{id}", a.countryDeal)
	mux.HandleFunc("GET /api/v1/choose-country", a.chooseCountry)
	// Apex geo entry point.
	mux.HandleFunc("GET /", a.apexRoot)
	(&admin.Service{DB: a.Catalog.DB, Origin: os.Getenv("DEAL_GO_ADMIN_ORIGIN"),
		Enabled: os.Getenv("DEAL_GO_ADMIN_ENABLED") == "1"}).Register(mux)
	return http.TimeoutHandler(mux, 12*time.Second, "request timed out")
}

func write(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func (a API) health(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 2*time.Second)
	defer cancel()
	if err := a.Catalog.DB.Ping(ctx); err != nil {
		write(w, 503, map[string]string{"status": "unavailable"})
		return
	}
	write(w, 200, map[string]string{"status": "ok"})
}

func (a API) meta(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	meta, err := a.Catalog.Metadata(ctx)
	if err != nil {
		write(w, 503, map[string]string{"error": "catalog temporarily unavailable"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	write(w, 200, meta)
}

func parseFilter(r *http.Request, defaultCountry string) (catalog.Filter, error) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	if limit < 1 || limit > 48 {
		limit = 24
	}
	f := catalog.Filter{
		Query:    strings.TrimSpace(q.Get("q")),
		Store:    q.Get("store"),
		Country:  defaultCountry,
		Category: q.Get("category"),
		Sort:     q.Get("sort"),
		Cursor:   q.Get("cursor"),
		Limit:    limit,
		Verified: q.Get("verified") == "1",
	}
	if f.Store != "" && !storeCode.MatchString(f.Store) {
		return f, errors.New("invalid store")
	}
	if f.Country != "" && !countryCode.MatchString(f.Country) {
		return f, errors.New("invalid country")
	}
	if f.Category != "" && !categoryCode.MatchString(f.Category) {
		return f, errors.New("invalid category")
	}
	if minp := q.Get("min_price"); minp != "" {
		v, err := strconv.ParseFloat(minp, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return f, errors.New("invalid min_price")
		}
		f.MinPrice = &v
	}
	if maxp := q.Get("max_price"); maxp != "" {
		v, err := strconv.ParseFloat(maxp, 64)
		if err != nil || math.IsNaN(v) || math.IsInf(v, 0) || v < 0 {
			return f, errors.New("invalid max_price")
		}
		f.MaxPrice = &v
	}
	if len([]rune(f.Query)) > 120 {
		return f, errors.New("query too long")
	}
	if f.MinPrice != nil && f.MaxPrice != nil && *f.MinPrice > *f.MaxPrice {
		return f, errors.New("min_price exceeds max_price")
	}
	if f.Sort != "" && f.Sort != "recent" && f.Sort != "drop" && f.Sort != "price_asc" && f.Sort != "price_desc" {
		return f, errors.New("invalid sort")
	}
	return f, nil
}

func (a API) products(w http.ResponseWriter, r *http.Request, country string) {
	f, err := parseFilter(r, country)
	if err != nil {
		write(w, 400, map[string]string{"error": err.Error()})
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	result, err := a.Catalog.List(ctx, f)
	if err != nil {
		if err.Error() == "invalid cursor" || strings.HasPrefix(err.Error(), "cursor ") || strings.HasPrefix(err.Error(), "invalid ") {
			write(w, 400, map[string]string{"error": err.Error()})
			return
		}
		write(w, 503, map[string]string{"error": "catalog temporarily unavailable"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=20")
	write(w, 200, result)
}

func (a API) productsLegacy(w http.ResponseWriter, r *http.Request) {
	a.products(w, r, "EG")
}

func (a API) countryProducts(w http.ResponseWriter, r *http.Request) {
	a.products(w, r, r.PathValue("code"))
}

func (a API) product(w http.ResponseWriter, r *http.Request, id int64, country string) {
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	detail, err := a.Catalog.Get(ctx, id, country)
	if errors.Is(err, pgx.ErrNoRows) {
		write(w, 404, map[string]string{"error": "product not found"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "catalog temporarily unavailable"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=20")
	write(w, 200, detail)
}

func (a API) productLegacy(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		write(w, 400, map[string]string{"error": "invalid product id"})
		return
	}
	a.product(w, r, id, "EG")
}

func (a API) countryProduct(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		write(w, 400, map[string]string{"error": "invalid product id"})
		return
	}
	a.product(w, r, id, r.PathValue("code"))
}

func (a API) countryDeals(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, _ := strconv.Atoi(q.Get("limit"))
	f := catalog.DealFilter{
		Country:      r.PathValue("code"),
		Store:        q.Get("store"),
		Category:     q.Get("category"),
		Cursor:       q.Get("cursor"),
		Limit:        limit,
		FeaturedOnly: q.Get("featured") == "1",
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	result, err := a.Catalog.ListDeals(ctx, f)
	if err != nil {
		if strings.HasPrefix(err.Error(), "invalid ") || strings.HasPrefix(err.Error(), "cursor ") {
			write(w, 400, map[string]string{"error": err.Error()})
			return
		}
		write(w, 503, map[string]string{"error": "deals temporarily unavailable"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=20")
	write(w, 200, result)
}

func (a API) countryDeal(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id < 1 {
		write(w, 400, map[string]string{"error": "invalid deal id"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 10*time.Second)
	defer cancel()
	deal, err := a.Catalog.GetDeal(ctx, id, r.PathValue("code"))
	if errors.Is(err, pgx.ErrNoRows) {
		write(w, 404, map[string]string{"error": "deal not found"})
		return
	}
	if err != nil {
		write(w, 503, map[string]string{"error": "deal temporarily unavailable"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=20")
	write(w, 200, map[string]any{"deal": deal})
}

func (a API) countryMeta(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	m, err := a.Catalog.CountryMetadata(ctx, code)
	if err != nil {
		write(w, 503, map[string]string{"error": "catalog temporarily unavailable"})
		return
	}
	if m == nil {
		write(w, 404, map[string]string{"error": "country not found"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	write(w, 200, m)
}

func (a API) countryCategories(w http.ResponseWriter, r *http.Request) {
	code := r.PathValue("code")
	ctx, cancel := contextWithTimeout(r, 5*time.Second)
	defer cancel()
	m, err := a.Catalog.CountryMetadata(ctx, code)
	if err != nil {
		write(w, 503, map[string]string{"error": "catalog temporarily unavailable"})
		return
	}
	if m == nil {
		write(w, 404, map[string]string{"error": "country not found"})
		return
	}
	w.Header().Set("Cache-Control", "public, max-age=300")
	write(w, 200, map[string]any{"categories": m.Categories})
}

func (a API) apexRoot(w http.ResponseWriter, r *http.Request) {
	// Only the apex domain should geo-redirect; country subdomains hit Next directly.
	host := strings.ToLower(r.Host)
	if host != "ahsanli.com" && host != "www.ahsanli.com" {
		write(w, 404, map[string]string{"error": "not found"})
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	if host == "www.ahsanli.com" {
		http.Redirect(w, r, "https://ahsanli.com/", http.StatusMovedPermanently)
		return
	}
	// Respect saved preference cookie.
	if pref, err := r.Cookie("country_pref"); err == nil && pref.Value != "" {
		ctx, cancel := contextWithTimeout(r, 3*time.Second)
		defer cancel()
		m, err := a.Catalog.CountryMetadata(ctx, pref.Value)
		if err == nil && m != nil {
			http.Redirect(w, r, "https://"+m.Hostname+"/"+m.DefaultLocale+"/", http.StatusFound)
			return
		}
	}
	clientIP := r.Header.Get("X-Real-IP")
	if clientIP == "" {
		clientIP = r.RemoteAddr
	}
	ctx, cancel := contextWithTimeout(r, 3*time.Second)
	defer cancel()
	code, _ := a.Catalog.DetectCountryByIP(ctx, clientIP)
	if code != "" {
		m, err := a.Catalog.CountryMetadata(ctx, code)
		if err == nil && m != nil {
			http.Redirect(w, r, "https://"+m.Hostname+"/"+m.DefaultLocale+"/", http.StatusFound)
			return
		}
	}
	// Unknown/VPN/failed lookup: redirect to country selector.
	http.Redirect(w, r, "https://ahsanli.com/countries", http.StatusFound)
}

func (a API) chooseCountry(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.URL.Query().Get("code"))
	if !countryCode.MatchString(code) {
		write(w, 400, map[string]string{"error": "invalid country"})
		return
	}
	ctx, cancel := contextWithTimeout(r, 3*time.Second)
	defer cancel()
	m, err := a.Catalog.CountryMetadata(ctx, code)
	if err != nil {
		write(w, 503, map[string]string{"error": "catalog temporarily unavailable"})
		return
	}
	if m == nil || m.Hostname == "" {
		write(w, 404, map[string]string{"error": "country not found"})
		return
	}
	locale := r.URL.Query().Get("locale")
	if locale != "ar" && locale != "en" {
		locale = m.DefaultLocale
	}
	w.Header().Set("Cache-Control", "private, no-store")
	http.SetCookie(w, &http.Cookie{Name: "country_pref", Value: code, Path: "/", Domain: "ahsanli.com", MaxAge: 31536000, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode})
	http.Redirect(w, r, "https://"+m.Hostname+"/"+locale+"/", http.StatusSeeOther)
}
