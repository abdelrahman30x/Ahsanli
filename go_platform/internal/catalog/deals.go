package catalog

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"
)

type Deal struct {
	Product
	DealID            int64     `json:"deal_id"`
	DisplayedDiscount *string   `json:"displayed_discount"`
	Featured          bool      `json:"featured"`
	EditorialNoteAR   string    `json:"editorial_note_ar"`
	EditorialNoteEN   string    `json:"editorial_note_en"`
	SelectedAt        time.Time `json:"selected_at"`
}

type DealPage struct {
	Items      []Deal `json:"items"`
	NextCursor string `json:"next_cursor,omitempty"`
	HasMore    bool   `json:"has_more"`
}

type DealFilter struct {
	Country      string
	Store        string
	Category     string
	Cursor       string
	Limit        int
	FeaturedOnly bool
}

type dealCursor struct {
	Featured bool   `json:"f"`
	Selected string `json:"s"`
	DealID   int64  `json:"i"`
}

type dealScanner interface {
	Scan(dest ...any) error
}

func scanDeal(row dealScanner) (Deal, error) {
	var d Deal
	p := &d.Product
	err := row.Scan(&p.ID, &p.Title, &p.ProductID, &p.Store, &p.StoreName, &p.Country, &p.CountryName,
		&p.Currency, &p.Price, &p.Seller, &p.ObservedAt, &p.Imported, &p.Fresh, &p.DropPct, &p.ImageURL,
		&p.URL, &p.AffiliateURL, &d.DealID, &d.DisplayedDiscount, &d.Featured, &d.EditorialNoteAR,
		&d.EditorialNoteEN, &d.SelectedAt)
	if p.AffiliateURL == "" {
		p.URL = ""
	}
	return d, err
}

const dealSelect = selectProductFields + `,d.id,d.evidence_discount::text,d.featured,d.editorial_note_ar,d.editorial_note_en,
 COALESCE(d.reviewed_at,d.created_at)` + selectProductFrom + `
 JOIN core_dealcandidate d ON d.winning_observation_id=o.id AND d.reason='historical'
 JOIN core_dealrule rule ON rule.country_id=c.id
 LEFT JOIN core_category cat ON cat.id=l.category_id `

const eligiblePublicDeal = `d.state IN ('approved','queued') AND d.evidence_path<>''
 AND d.evidence_price=o.price AND d.evidence_seller=o.seller
 AND d.id=(SELECT max(d2.id) FROM core_dealcandidate d2 WHERE d2.winning_observation_id=o.id AND d2.reason='historical')
 AND co.listing_id=l.id AND co.observation_id=o.id
 AND NOT o.imported AND o.stock_status='in_stock'
 AND o.observed_at>=now()-(rule.freshness_hours*interval '1 hour')
 AND d.rule_version=rule.version AND rule.enabled AND r.enabled AND r.publish_enabled
 AND l.extracted_attributes->>'go_owned'='true' AND c.enabled AND c.enabled_public_site`

func decodeDealCursor(raw string) (dealCursor, error) {
	var cursor dealCursor
	if raw == "" {
		return cursor, nil
	}
	if len(raw) > 512 {
		return cursor, errors.New("cursor too long")
	}
	data, err := base64.URLEncoding.DecodeString(raw)
	if err != nil {
		return cursor, errors.New("invalid cursor")
	}
	if err = json.Unmarshal(data, &cursor); err != nil || cursor.DealID < 1 {
		return cursor, errors.New("invalid cursor")
	}
	if _, err = time.Parse(time.RFC3339Nano, cursor.Selected); err != nil {
		return cursor, errors.New("invalid cursor date")
	}
	return cursor, nil
}

func encodeDealCursor(d Deal) string {
	data, _ := json.Marshal(dealCursor{Featured: d.Featured, Selected: d.SelectedAt.Format(time.RFC3339Nano), DealID: d.DealID})
	return base64.URLEncoding.EncodeToString(data)
}

func (s *Store) ListDeals(ctx context.Context, f DealFilter) (DealPage, error) {
	if !countryCode.MatchString(f.Country) {
		return DealPage{}, errors.New("invalid country")
	}
	if f.Limit < 1 || f.Limit > 48 {
		f.Limit = 24
	}
	cursor, err := decodeDealCursor(f.Cursor)
	if err != nil {
		return DealPage{}, err
	}
	args := []any{f.Country}
	where := []string{eligiblePublicDeal, "c.code=$1"}
	if f.Store != "" {
		if !storeCode.MatchString(f.Store) {
			return DealPage{}, errors.New("invalid store")
		}
		args = append(args, f.Store)
		where = append(where, "r.code=$"+strconv.Itoa(len(args)))
	}
	if f.Category != "" {
		if !categoryCode.MatchString(f.Category) {
			return DealPage{}, errors.New("invalid category")
		}
		args = append(args, f.Category)
		where = append(where, "cat.code=$"+strconv.Itoa(len(args)))
	}
	if f.FeaturedOnly {
		where = append(where, "d.featured")
	}
	if f.Cursor != "" {
		args = append(args, cursor.Featured, cursor.Selected, cursor.DealID)
		n := len(args)
		where = append(where, fmt.Sprintf("(d.featured,COALESCE(d.reviewed_at,d.created_at),d.id)<($%d::boolean,$%d::timestamptz,$%d)", n-2, n-1, n))
	}
	args = append(args, f.Limit+1)
	query := dealSelect + " WHERE " + joinConditions(where) +
		" ORDER BY d.featured DESC,COALESCE(d.reviewed_at,d.created_at) DESC,d.id DESC LIMIT $" + strconv.Itoa(len(args))
	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return DealPage{}, err
	}
	defer rows.Close()
	items := make([]Deal, 0, f.Limit)
	for rows.Next() {
		d, scanErr := scanDeal(rows)
		if scanErr != nil {
			return DealPage{}, scanErr
		}
		items = append(items, d)
	}
	if err = rows.Err(); err != nil {
		return DealPage{}, err
	}
	page := DealPage{Items: items, HasMore: len(items) > f.Limit}
	if page.HasMore {
		items = items[:f.Limit]
		page.Items = items
		page.NextCursor = encodeDealCursor(items[len(items)-1])
	}
	return page, nil
}

func (s *Store) GetDeal(ctx context.Context, listingID int64, country string) (Deal, error) {
	if listingID < 1 || !countryCode.MatchString(country) {
		return Deal{}, errors.New("invalid deal")
	}
	query := dealSelect + " WHERE " + eligiblePublicDeal + " AND l.id=$1 AND c.code=$2"
	return scanDeal(s.DB.QueryRow(ctx, query, listingID, country))
}

func joinConditions(where []string) string {
	if len(where) == 0 {
		return ""
	}
	result := where[0]
	for _, condition := range where[1:] {
		result += " AND " + condition
	}
	return result
}
