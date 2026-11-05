package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/oschwald/maxminddb-golang/v2"
)

type Product struct {
	ID           int64      `json:"id"`
	Title        string     `json:"title"`
	ProductID    string     `json:"product_id"`
	Store        string     `json:"store"`
	StoreName    string     `json:"store_name"`
	Country      string     `json:"country"`
	CountryName  string     `json:"country_name"`
	Currency     string     `json:"currency"`
	Price        *string    `json:"price"`
	Seller       *string    `json:"seller"`
	ObservedAt   *time.Time `json:"observed_at"`
	Imported     bool       `json:"imported"`
	Fresh        bool       `json:"fresh"`
	DropPct      *string    `json:"drop_percent"`
	ImageURL     *string    `json:"image_url"`
	URL          string     `json:"url"`
	AffiliateURL string     `json:"affiliate_url"`
}

type PricePoint struct {
	Price    string    `json:"price"`
	At       time.Time `json:"at"`
	Imported bool      `json:"imported"`
	Seller   string    `json:"seller"`
}

type Detail struct {
	Product Product      `json:"product"`
	History []PricePoint `json:"history"`
}

type Filter struct {
	Query    string
	Store    string
	Country  string
	Category string
	MinPrice *float64
	MaxPrice *float64
	Sort     string
	Page     int
	Limit    int
	Verified bool
	Cursor   string
	Locale   string
}

type Page struct {
	Items      []Product `json:"items"`
	Page       int       `json:"page,omitempty"`
	NextCursor string    `json:"next_cursor,omitempty"`
	HasMore    bool      `json:"has_more"`
}

type Cursor struct {
	SortKey string `json:"k"`
	LastID  int64  `json:"i"`
	Sort    string `json:"s"`
}

type Store struct {
	DB    *pgxpool.Pool
	GeoDB *maxminddb.Reader
}

type RetailerMeta struct {
	Code    string `json:"code"`
	Name    string `json:"name"`
	Enabled bool   `json:"enabled"`
	Health  string `json:"health"`
}

type CategoryMeta struct {
	Code   string `json:"code"`
	NameAR string `json:"name_ar"`
	NameEN string `json:"name_en"`
}

type CountryMeta struct {
	Code          string         `json:"code"`
	NameAR        string         `json:"name_ar"`
	NameEN        string         `json:"name_en"`
	Currency      string         `json:"currency"`
	CurrencyLabel string         `json:"currency_label"`
	Timezone      string         `json:"timezone"`
	DefaultLocale string         `json:"default_locale"`
	Hostname      string         `json:"hostname"`
	Retailers     []RetailerMeta `json:"retailers"`
	Categories    []CategoryMeta `json:"categories"`
	TelegramURL   string         `json:"telegram_url,omitempty"`
}

type Meta struct {
	Countries []CountryMeta `json:"countries"`
}

var storeCode = regexp.MustCompile(`^[a-z0-9_]{1,32}$`)
var countryCode = regexp.MustCompile(`^[A-Z]{2}$`)
var categoryCode = regexp.MustCompile(`^[a-z0-9_]{1,64}$`)

func DatabaseURL() string {
	if v := strings.TrimSpace(os.Getenv("DATABASE_URL")); v != "" {
		return v
	}
	u := &url.URL{Scheme: "postgres", User: url.UserPassword(getenvDefault("DEAL_DB_USER", "ahsanly_app"), os.Getenv("DEAL_DB_PASSWORD")), Host: getenvDefault("DEAL_DB_HOST", "127.0.0.1") + ":" + getenvDefault("DEAL_DB_PORT", "5432"), Path: "/" + getenvDefault("DEAL_DB_NAME", "ahsanly")}
	u.RawQuery = "sslmode=disable"
	return u.String()
}

func getenvDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func New(ctx context.Context, dsn string) (*Store, error) {
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		return nil, err
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, err
	}
	if err = db.Ping(ctx); err != nil {
		db.Close()
		return nil, err
	}
	store := &Store{DB: db}
	geoPath := strings.TrimSpace(os.Getenv("AHSANLY_GEOIP_DB"))
	if geoPath == "" {
		geoPath = "/etc/ahsanly/dbip-country-lite.mmdb"
	}
	if _, statErr := os.Stat(geoPath); statErr == nil {
		store.GeoDB, err = maxminddb.Open(geoPath)
		if err == nil {
			err = store.GeoDB.Verify()
		}
		if err != nil {
			store.Close()
			return nil, fmt.Errorf("geoip database: %w", err)
		}
	} else if !errors.Is(statErr, os.ErrNotExist) || os.Getenv("AHSANLY_GEOIP_DB") != "" {
		store.Close()
		return nil, fmt.Errorf("geoip database: %w", statErr)
	}
	return store, nil
}

func (s *Store) Close() {
	if s.GeoDB != nil {
		_ = s.GeoDB.Close()
	}
	if s.DB != nil {
		s.DB.Close()
	}
}

const selectProduct = `SELECT l.id, COALESCE(NULLIF(l.title,''),l.retailer_product_id), l.retailer_product_id,
 r.code, COALESCE(NULLIF(r.display_name_ar,''),r.code), COALESCE(c.code,''), COALESCE(c.name_ar,''), r.currency,
 o.price::text, o.seller, o.observed_at, COALESCE(o.imported,true),
 (o.id IS NOT NULL AND NOT o.imported AND o.stock_status='in_stock' AND r.enabled AND COALESCE(c.enabled,false) AND o.observed_at>=now()-interval '2 hours'),
 dc.percent::text, NULLIF(COALESCE(l.extracted_attributes->>'image_url',''),''), l.original_url,
 CASE WHEN r.code<>'amazon_eg' AND (o.id IS NULL OR o.imported) AND EXISTS
 (SELECT 1 FROM core_kanbkamproduct kp WHERE kp.retailer_listing_id=l.id AND kp.mapping_status<>'linked')
 THEN '' ELSE COALESCE((SELECT a.tracking_url FROM core_affiliatelink a JOIN core_channel ch ON ch.id=a.channel_id
 WHERE a.listing_id=l.id AND a.verified_at IS NOT NULL AND ch.enabled AND NOT ch.private_test
 AND ch.country_id=r.country_id AND EXISTS
 (SELECT 1 FROM core_publicationroute pr WHERE pr.channel_id=ch.id AND pr.enabled
 AND pr.country_id=r.country_id AND (pr.retailer_id IS NULL OR pr.retailer_id=r.id))
 ORDER BY a.id LIMIT 1), l.original_url) END
 FROM core_listing l JOIN core_retailer r ON r.id=l.retailer_id LEFT JOIN core_country c ON c.id=r.country_id
 LEFT JOIN core_currentoffer co ON co.listing_id=l.id
 LEFT JOIN core_offerobservation o ON o.id=co.observation_id
 LEFT JOIN LATERAL (SELECT d.percent FROM core_dealcandidate d WHERE d.winning_observation_id=o.id AND d.reason='historical'
 AND d.state IN ('captured','queued') AND d.evidence_path<>'' AND d.evidence_price=o.price AND d.evidence_seller=o.seller
 ORDER BY d.id DESC LIMIT 1) dc ON true`

func scanProduct(rows pgx.Rows) (Product, error) {
	var p Product
	err := rows.Scan(&p.ID, &p.Title, &p.ProductID, &p.Store, &p.StoreName, &p.Country, &p.CountryName, &p.Currency, &p.Price, &p.Seller, &p.ObservedAt, &p.Imported, &p.Fresh, &p.DropPct, &p.ImageURL, &p.URL, &p.AffiliateURL)
	if p.AffiliateURL == "" {
		p.URL = ""
	}
	return p, err
}

func encodeCursor(c Cursor) string {
	b, _ := json.Marshal(c)
	return base64.URLEncoding.EncodeToString(b)
}

func decodeCursor(s string) (Cursor, error) {
	var c Cursor
	if s == "" {
		return c, nil
	}
	if len(s) > 512 {
		return c, errors.New("cursor too long")
	}
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return c, err
	}
	err = json.Unmarshal(b, &c)
	if err == nil && c.LastID < 1 {
		err = errors.New("invalid cursor id")
	}
	return c, err
}

func sortExpression(sort string) string {
	switch sort {
	case "price_asc":
		return "COALESCE(o.price, 100000000000000::numeric) ASC, l.id ASC"
	case "price_desc":
		return "COALESCE(o.price, -1::numeric) DESC, l.id DESC"
	case "drop":
		return "d.percent DESC, l.id DESC"
	case "recent":
		return "COALESCE(o.observed_at, '-infinity'::timestamptz) DESC, l.id DESC"
	default:
		return "l.id DESC"
	}
}

func cursorPredicate(sort string, cursor Cursor, args *[]any) (string, error) {
	if cursor.LastID == 0 {
		return "", nil
	}
	if cursor.Sort != sort {
		return "", errors.New("cursor sort mismatch")
	}
	if sort == "" {
		*args = append(*args, cursor.LastID)
		return "l.id<$" + strconv.Itoa(len(*args)), nil
	}
	if cursor.SortKey == "" {
		return "", errors.New("cursor sort key missing")
	}
	var expression, cast, comparison string
	switch sort {
	case "recent":
		if cursor.SortKey != "-infinity" {
			if _, err := time.Parse(time.RFC3339Nano, cursor.SortKey); err != nil {
				return "", errors.New("invalid recent cursor")
			}
		}
		expression, cast, comparison = "COALESCE(o.observed_at, '-infinity'::timestamptz)", "timestamptz", "<"
	case "price_asc", "price_desc", "drop":
		if _, err := strconv.ParseFloat(cursor.SortKey, 64); err != nil {
			return "", errors.New("invalid numeric cursor")
		}
		cast = "numeric"
		comparison = "<"
		if sort == "price_asc" {
			expression, comparison = "COALESCE(o.price, 100000000000000::numeric)", ">"
		}
		if sort == "price_desc" {
			expression = "COALESCE(o.price, -1::numeric)"
		}
		if sort == "drop" {
			expression = "d.percent"
		}
	default:
		return "", errors.New("invalid cursor sort")
	}
	*args = append(*args, cursor.SortKey, cursor.LastID)
	n := len(*args)
	return fmt.Sprintf("(%s,l.id)%s($%d::%s,$%d)", expression, comparison, n-1, cast, n), nil
}

func productCursorKey(p Product, sort string) string {
	switch sort {
	case "recent":
		if p.ObservedAt == nil {
			return "-infinity"
		}
		return p.ObservedAt.Format(time.RFC3339Nano)
	case "price_asc":
		if p.Price == nil {
			return "100000000000000"
		}
		return *p.Price
	case "price_desc":
		if p.Price == nil {
			return "-1"
		}
		return *p.Price
	default:
		return ""
	}
}

func (s *Store) List(ctx context.Context, f Filter) (Page, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.Limit < 1 || f.Limit > 48 {
		f.Limit = 24
	}
	if len([]rune(f.Query)) > 120 {
		return Page{}, errors.New("query too long")
	}
	if f.Sort == "drop" {
		return s.listDrops(ctx, f)
	}
	if f.Sort != "" && f.Sort != "recent" && f.Sort != "price_asc" && f.Sort != "price_desc" {
		return Page{}, errors.New("invalid sort")
	}
	cursor, err := decodeCursor(f.Cursor)
	if err != nil {
		return Page{}, errors.New("invalid cursor")
	}
	args := []any{}
	where := []string{"c.enabled AND c.enabled_public_site"}
	if q := strings.TrimSpace(f.Query); q != "" {
		// Search title, retailer product id, or a pasted product URL.
		args = append(args, "%"+q+"%", "%"+q+"%", "%"+q+"%")
		n := len(args) - 2
		where = append(where, fmt.Sprintf("(l.title ILIKE $%d OR l.retailer_product_id ILIKE $%d OR l.original_url ILIKE $%d)", n, n+1, n+2))
	}
	if f.Store != "" {
		if !storeCode.MatchString(f.Store) {
			return Page{}, errors.New("invalid store")
		}
		args = append(args, f.Store)
		where = append(where, "r.code=$"+strconv.Itoa(len(args)))
	}
	if f.Country != "" {
		if !countryCode.MatchString(f.Country) {
			return Page{}, errors.New("invalid country")
		}
		args = append(args, f.Country)
		where = append(where, "c.code=$"+strconv.Itoa(len(args)))
	}
	if f.Category != "" {
		if !categoryCode.MatchString(f.Category) {
			return Page{}, errors.New("invalid category")
		}
		args = append(args, f.Category)
		where = append(where, "cat.code=$"+strconv.Itoa(len(args)))
	}
	if f.MinPrice != nil {
		args = append(args, *f.MinPrice)
		where = append(where, "o.price>=$"+strconv.Itoa(len(args)))
	}
	if f.MaxPrice != nil {
		args = append(args, *f.MaxPrice)
		where = append(where, "o.price<=$"+strconv.Itoa(len(args)))
	}
	if f.Verified {
		where = append(where, "o.id IS NOT NULL AND NOT o.imported AND o.stock_status='in_stock' AND r.enabled AND COALESCE(c.enabled,false) AND o.observed_at>=now()-interval '2 hours'")
	}
	order := sortExpression(f.Sort)
	if predicate, e := cursorPredicate(f.Sort, cursor, &args); e != nil {
		return Page{}, e
	} else if predicate != "" {
		where = append(where, predicate)
	}
	args = append(args, f.Limit+1)
	query := selectProduct + " LEFT JOIN core_category cat ON cat.id=l.category_id WHERE " + strings.Join(where, " AND ") + " ORDER BY " + order + fmt.Sprintf(" LIMIT $%d", len(args))
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return Page{}, err
	}
	defer rows.Close()
	items := make([]Product, 0, f.Limit)
	for rows.Next() {
		p, e := scanProduct(rows)
		if e != nil {
			return Page{}, e
		}
		items = append(items, p)
	}
	if err = rows.Err(); err != nil {
		return Page{}, err
	}
	more := len(items) > f.Limit
	if more {
		items = items[:f.Limit]
	}
	page := Page{Items: items, Page: f.Page, HasMore: more}
	if more && len(items) > 0 {
		last := items[len(items)-1]
		page.NextCursor = encodeCursor(Cursor{LastID: last.ID, SortKey: productCursorKey(last, f.Sort), Sort: f.Sort})
	}
	return page, nil
}

// A drop is a recorded, directly verified candidate for the current offer.
func (s *Store) listDrops(ctx context.Context, f Filter) (Page, error) {
	cursor, err := decodeCursor(f.Cursor)
	if err != nil {
		return Page{}, errors.New("invalid cursor")
	}
	args := []any{}
	where := []string{"d.reason='historical'", "d.state IN ('captured','queued')", "d.evidence_path<>''", "d.evidence_price=o.price", "d.evidence_seller=o.seller", "NOT o.imported", "o.stock_status='in_stock'", "o.observed_at>=now()-interval '2 hours'", "r.enabled", "c.enabled", "c.enabled_public_site"}
	where = append(where, "d.id=(SELECT max(d2.id) FROM core_dealcandidate d2 WHERE d2.winning_observation_id=o.id AND d2.reason='historical')")
	if q := strings.TrimSpace(f.Query); q != "" {
		args = append(args, "%"+q+"%")
		where = append(where, "l.title ILIKE $"+strconv.Itoa(len(args)))
	}
	if f.Store != "" {
		args = append(args, f.Store)
		where = append(where, "r.code=$"+strconv.Itoa(len(args)))
	}
	if f.Country != "" {
		args = append(args, f.Country)
		where = append(where, "c.code=$"+strconv.Itoa(len(args)))
	}
	if f.Category != "" {
		args = append(args, f.Category)
		where = append(where, "cat.code=$"+strconv.Itoa(len(args)))
	}
	if predicate, e := cursorPredicate("drop", cursor, &args); e != nil {
		return Page{}, e
	} else if predicate != "" {
		where = append(where, predicate)
	}
	args = append(args, f.Limit+1)
	query := `SELECT l.id,d.percent::text FROM core_dealcandidate d
JOIN core_offerobservation o ON o.id=d.winning_observation_id
JOIN core_currentoffer co ON co.observation_id=o.id
JOIN core_listing l ON l.id=o.listing_id
JOIN core_retailer r ON r.id=l.retailer_id
JOIN core_country c ON c.id=r.country_id
LEFT JOIN core_category cat ON cat.id=l.category_id
WHERE ` + strings.Join(where, " AND ") + " ORDER BY d.percent DESC,l.id DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return Page{}, err
	}
	ids := []int64{}
	percentByID := map[int64]string{}
	for rows.Next() {
		var id int64
		var percent string
		if err = rows.Scan(&id, &percent); err != nil {
			rows.Close()
			return Page{}, err
		}
		ids = append(ids, id)
		percentByID[id] = percent
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return Page{}, err
	}
	more := len(ids) > f.Limit
	if more {
		ids = ids[:f.Limit]
	}
	page := Page{Items: []Product{}, Page: f.Page, HasMore: more}
	if more && len(ids) > 0 {
		lastID := ids[len(ids)-1]
		page.NextCursor = encodeCursor(Cursor{LastID: lastID, SortKey: percentByID[lastID], Sort: "drop"})
	}
	if len(ids) == 0 {
		return page, nil
	}
	products, err := s.DB.Query(ctx, selectProduct+" WHERE l.id=ANY($1) ORDER BY array_position($1::bigint[],l.id)", ids)
	if err != nil {
		return Page{}, err
	}
	defer products.Close()
	for products.Next() {
		p, e := scanProduct(products)
		if e != nil {
			return Page{}, e
		}
		page.Items = append(page.Items, p)
	}
	return page, products.Err()
}

func (s *Store) Metadata(ctx context.Context) (Meta, error) {
	rows, err := s.DB.Query(ctx, `SELECT c.code,c.name_ar,c.name_en,c.currency,c.currency_label_ar,c.timezone,c.default_locale,c.hostname,r.code,COALESCE(NULLIF(r.display_name_ar,''),r.code),COALESCE(r.enabled,false),COALESCE(r.health,'unknown')
FROM core_country c LEFT JOIN core_retailer r ON r.country_id=c.id
WHERE c.enabled=true AND c.enabled_public_site=true ORDER BY c.code,r.code`)
	if err != nil {
		return Meta{}, err
	}
	defer rows.Close()
	meta := Meta{Countries: []CountryMeta{}}
	for rows.Next() {
		var code, nameAR, nameEN, currency, currencyLabel, timezone, defaultLocale, hostname string
		var retailerCode, retailerName *string
		var enabled bool
		var health string
		if err = rows.Scan(&code, &nameAR, &nameEN, &currency, &currencyLabel, &timezone, &defaultLocale, &hostname, &retailerCode, &retailerName, &enabled, &health); err != nil {
			return Meta{}, err
		}
		if len(meta.Countries) == 0 || meta.Countries[len(meta.Countries)-1].Code != code {
			meta.Countries = append(meta.Countries, CountryMeta{Code: code, NameAR: nameAR, NameEN: nameEN, Currency: currency, CurrencyLabel: currencyLabel, Timezone: timezone, DefaultLocale: defaultLocale, Hostname: hostname, Retailers: []RetailerMeta{}})
		}
		if retailerCode != nil && retailerName != nil {
			last := &meta.Countries[len(meta.Countries)-1]
			last.Retailers = append(last.Retailers, RetailerMeta{Code: *retailerCode, Name: *retailerName, Enabled: enabled, Health: health})
		}
	}
	return meta, rows.Err()
}

func (s *Store) CountryMetadata(ctx context.Context, code string) (*CountryMeta, error) {
	if !countryCode.MatchString(code) {
		return nil, errors.New("invalid country")
	}
	row := s.DB.QueryRow(ctx, `SELECT c.code,c.name_ar,c.name_en,c.currency,c.currency_label_ar,c.timezone,c.default_locale,c.hostname
FROM core_country c WHERE c.code=$1 AND c.enabled=true AND c.enabled_public_site=true`, code)
	var m CountryMeta
	if err := row.Scan(&m.Code, &m.NameAR, &m.NameEN, &m.Currency, &m.CurrencyLabel, &m.Timezone, &m.DefaultLocale, &m.Hostname); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	retailers, err := s.DB.Query(ctx, `SELECT r.code,COALESCE(NULLIF(r.display_name_ar,''),r.code),COALESCE(r.enabled,false),COALESCE(r.health,'unknown')
FROM core_retailer r WHERE r.country_id=(SELECT id FROM core_country WHERE code=$1) ORDER BY r.code`, code)
	if err != nil {
		return nil, err
	}
	defer retailers.Close()
	m.Retailers = []RetailerMeta{}
	for retailers.Next() {
		var rm RetailerMeta
		if err = retailers.Scan(&rm.Code, &rm.Name, &rm.Enabled, &rm.Health); err != nil {
			return nil, err
		}
		m.Retailers = append(m.Retailers, rm)
	}
	if err = retailers.Err(); err != nil { return nil, err }
	cats, err := s.DB.Query(ctx, `SELECT cat.code,cat.name_ar,cat.name_en FROM core_category cat
WHERE EXISTS (SELECT 1 FROM core_listing l JOIN core_retailer r ON r.id=l.retailer_id JOIN core_country c ON c.id=r.country_id
WHERE l.category_id=cat.id AND c.code=$1) ORDER BY cat.position`, code)
	if err != nil {
		return nil, err
	}
	defer cats.Close()
	m.Categories = []CategoryMeta{}
	for cats.Next() {
		var cm CategoryMeta
		if err = cats.Scan(&cm.Code, &cm.NameAR, &cm.NameEN); err != nil {
			return nil, err
		}
		m.Categories = append(m.Categories, cm)
	}
	if err = cats.Err(); err != nil {
		return nil, err
	}
	// An active public destination may have no public handle. In that case, omit the link.
	_ = s.DB.QueryRow(ctx, `SELECT COALESCE(ch.public_url,'') FROM core_channel ch
JOIN core_publicationroute pr ON pr.channel_id=ch.id
JOIN core_country c ON c.id=pr.country_id
WHERE c.code=$1 AND ch.platform='telegram' AND ch.enabled AND NOT ch.private_test AND pr.enabled
AND ch.public_url LIKE 'https://t.me/%' ORDER BY pr.id LIMIT 1`, code).Scan(&m.TelegramURL)
	return &m, nil
}

func (s *Store) Get(ctx context.Context, id int64, countryCode string) (Detail, error) {
	var detail Detail
	args := []any{id}
	where := "l.id=$1"
	if countryCode != "" {
		args = append(args, countryCode)
		where += " AND c.code=$2"
	}
	where += " AND c.enabled AND c.enabled_public_site"
	rows, err := s.DB.Query(ctx, selectProduct+" WHERE "+where, args...)
	if err != nil {
		return Detail{}, err
	}
	if rows.Next() {
		detail.Product, err = scanProduct(rows)
	} else {
		err = pgx.ErrNoRows
	}
	rows.Close()
	if err != nil {
		return Detail{}, err
	}
	if err = rows.Err(); err != nil {
		return Detail{}, err
	}
	h, err := s.DB.Query(ctx, `SELECT price::text,observed_at,imported,seller FROM core_offerobservation WHERE listing_id=$1 ORDER BY observed_at DESC,id DESC LIMIT 90`, id)
	if err != nil {
		return Detail{}, err
	}
	defer h.Close()
	detail.History = []PricePoint{}
	for h.Next() {
		var point PricePoint
		if err = h.Scan(&point.Price, &point.At, &point.Imported, &point.Seller); err != nil {
			return Detail{}, err
		}
		detail.History = append(detail.History, point)
	}
	return detail, h.Err()
}

// DetectCountryByIP uses a locally installed MMDB. Unknown or private addresses
// deliberately fall through to the selector instead of guessing a country.
func (s *Store) DetectCountryByIP(ctx context.Context, clientAddress string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if s.GeoDB == nil {
		return "", nil
	}
	address := clientAddress
	if host, _, err := net.SplitHostPort(clientAddress); err == nil {
		address = host
	}
	ip, err := netip.ParseAddr(address)
	if err != nil || !ip.IsGlobalUnicast() || ip.IsPrivate() {
		return "", nil
	}
	var code string
	if err = s.GeoDB.Lookup(ip.Unmap()).DecodePath(&code, "country", "iso_code"); err != nil {
		return "", err
	}
	if !countryCode.MatchString(code) {
		return "", nil
	}
	return code, nil
}
