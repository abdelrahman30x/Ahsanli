package admin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"ahsanly/go_platform/internal/sources"
	"github.com/jackc/pgx/v5"
)

func requestContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(r.Context(), 8*time.Second)
}

func pageLimit(r *http.Request) (int, int) {
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 1000 {
		page = 1000
	}
	return page, (page - 1) * 50
}

func (s *Service) overview(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	var listings, watched, current, fresh, due, held, publications int64
	err := s.DB.QueryRow(ctx, `SELECT
 (SELECT count(*) FROM core_listing),
 (SELECT count(*) FROM core_listing WHERE tracking_state='watch'),
 (SELECT count(*) FROM core_currentoffer),
 (SELECT count(*) FROM core_currentoffer co JOIN core_offerobservation o ON o.id=co.observation_id
   JOIN core_listing l ON l.id=co.listing_id JOIN core_retailer r ON r.id=l.retailer_id
   WHERE NOT o.imported AND o.stock_status='in_stock' AND r.enabled AND o.observed_at>=now()-interval '2 hours'),
 (SELECT count(*) FROM core_fetchjob WHERE status='due' AND due_at<=now()),
 (SELECT count(*) FROM core_dealcandidate WHERE state IN ('held','review')),
 (SELECT count(*) FROM core_publication)`).Scan(&listings, &watched, &current, &fresh, &due, &held, &publications)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "overview unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"listings": listings, "watched": watched,
		"current_offers": current, "fresh_offers": fresh, "due_jobs": due,
		"held_candidates": held, "publications": publications})
}

func (s *Service) retailers(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT r.id,r.code,COALESCE(NULLIF(r.display_name_ar,''),r.code),
COALESCE(c.code,''),r.enabled,r.publish_enabled,r.health,r.offer_source,r.max_requests_per_hour,
r.last_success_at,r.last_failure_at,r.last_error_class,
(SELECT count(*) FROM core_listing l WHERE l.retailer_id=r.id AND l.tracking_state='watch')
FROM core_retailer r LEFT JOIN core_country c ON c.id=r.country_id ORDER BY c.code,r.code`)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retailer status unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, watched int64
		var code, name, country, health, source, lastError string
		var enabled, publish bool
		var rate int
		var success, failure *time.Time
		if err = rows.Scan(&id, &code, &name, &country, &enabled, &publish, &health, &source,
			&rate, &success, &failure, &lastError, &watched); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "code": code, "name": name, "country": country,
			"enabled": enabled, "publish_enabled": publish, "health": health, "offer_source": source,
			"max_requests_per_hour": rate, "last_success_at": success, "last_failure_at": failure,
			"last_error_class": lastError, "watched": watched})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "retailer status unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Service) listings(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	page, offset := pageLimit(r)
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if len([]rune(query)) > 100 {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "query too long"})
		return
	}
	state := r.URL.Query().Get("state")
	if state != "" && state != "watch" && state != "paused" {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid state"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT l.id,l.title,l.retailer_product_id,l.original_url,l.tracking_state,l.cadence,
r.code,l.last_checked_at,o.price::text,o.observed_at,COALESCE(o.imported,true)
FROM core_listing l JOIN core_retailer r ON r.id=l.retailer_id
LEFT JOIN core_currentoffer co ON co.listing_id=l.id
LEFT JOIN core_offerobservation o ON o.id=co.observation_id
WHERE ($1::text='' OR l.title ILIKE '%'||$1||'%' OR l.retailer_product_id ILIKE '%'||$1||'%')
AND ($2::text='' OR l.tracking_state=$2) ORDER BY l.id DESC LIMIT 51 OFFSET $3`, query, state, offset)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "listings unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var title, productID, url, tracking, cadence, retailer string
		var lastChecked, observed *time.Time
		var price *string
		var imported bool
		if err = rows.Scan(&id, &title, &productID, &url, &tracking, &cadence, &retailer,
			&lastChecked, &price, &observed, &imported); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "title": title, "product_id": productID,
			"url": url, "tracking_state": tracking, "cadence": cadence, "store": retailer,
			"last_checked_at": lastChecked, "price": price, "observed_at": observed, "imported": imported})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "listings unavailable"})
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "has_more": more})
}

func parseID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil && id > 0
}

func (s *Service) updateListing(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid listing"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var input struct {
		TrackingState string `json:"tracking_state"`
		Cadence       string `json:"cadence"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || (input.TrackingState != "watch" && input.TrackingState != "paused") ||
		(input.Cadence != "hot" && input.Cadence != "normal" && input.Cadence != "cold") {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid watchlist settings"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "watchlist unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var oldState, oldCadence, retailer, source string
	var enabled, permitted bool
	err = tx.QueryRow(ctx, `SELECT l.tracking_state,l.cadence,r.code,r.offer_source,r.enabled,
(r.permission_verified_at IS NOT NULL AND r.permission_reference<>'')
FROM core_listing l JOIN core_retailer r ON r.id=l.retailer_id WHERE l.id=$1 FOR UPDATE OF l`, id).Scan(
		&oldState, &oldCadence, &retailer, &source, &enabled, &permitted)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "listing not found"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "watchlist unavailable"})
		return
	}
	if input.TrackingState == "watch" && (!enabled || source != "server_http" || (retailer != "amazon_eg" && !permitted)) {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "retailer source is not ready for direct monitoring"})
		return
	}
	if oldState == input.TrackingState && oldCadence == input.Cadence {
		adminJSON(w, http.StatusOK, map[string]any{"id": id, "tracking_state": oldState, "cadence": oldCadence})
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE core_listing SET tracking_state=$2,cadence=$3 WHERE id=$1`, id, input.TrackingState, input.Cadence); err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "watchlist unavailable"})
		return
	}
	if input.TrackingState == "paused" {
		_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status='paused',lease_until=NULL,last_result='staff_paused' WHERE listing_id=$1 AND kind='offer'`, id)
	} else {
		priority := 50
		if input.Cadence == "hot" {
			priority = 10
		}
		_, err = tx.Exec(ctx, `INSERT INTO core_fetchjob(listing_id,kind,status,priority,due_at,retry_count,last_result,last_error_class,created_at)
VALUES($1,'offer','due',$2,now(),0,'staff_watch','',now())
ON CONFLICT(listing_id,kind) WHERE kind='offer' DO UPDATE SET
status='due',priority=EXCLUDED.priority,due_at=now(),lease_until=NULL,last_result='staff_watch'`, id, priority)
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "watchlist unavailable"})
		return
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_update_watchlist','listing',$2,jsonb_build_object('old_state',$3::text,'new_state',$4::text,
'old_cadence',$5::text,'new_cadence',$6::text),now())`, staff.UserID, strconv.FormatInt(id, 10), oldState, input.TrackingState, oldCadence, input.Cadence)
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "watchlist unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"id": id, "tracking_state": input.TrackingState, "cadence": input.Cadence})
}

func (s *Service) rules(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT rule.id,COALESCE(c.code,''),rule.name,rule.historical_percent::text,
rule.freshness_hours,rule.version,rule.enabled FROM core_dealrule rule
LEFT JOIN core_country c ON c.id=rule.country_id ORDER BY c.code,rule.id`)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rules unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var country, name, percent string
		var freshness, version int
		var enabled bool
		if err = rows.Scan(&id, &country, &name, &percent, &freshness, &version, &enabled); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "country": country, "name": name,
			"historical_percent": percent, "freshness_hours": freshness, "version": version, "enabled": enabled})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rules unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Service) updateRule(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid rule"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		HistoricalPercent string `json:"historical_percent"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid percentage"})
		return
	}
	basisPoints, err := sources.ParsePrice(input.HistoricalPercent)
	if err != nil || basisPoints < 100 || basisPoints > 9000 {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "percentage must be 1–90"})
		return
	}
	percent := fmt.Sprintf("%d.%02d", basisPoints/100, basisPoints%100)
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rules unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var old string
	err = tx.QueryRow(ctx, `SELECT historical_percent::text FROM core_dealrule WHERE id=$1 AND country_id IS NOT NULL FOR UPDATE`, id).Scan(&old)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "country rule not found"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rules unavailable"})
		return
	}
	if old == percent {
		adminJSON(w, http.StatusOK, map[string]any{"id": id, "historical_percent": old})
		return
	}
	_, err = tx.Exec(ctx, `UPDATE core_dealrule SET historical_percent=$2::numeric,version=version+1 WHERE id=$1`, id, percent)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_update_deal_rule','deal_rule',$2,jsonb_build_object('old_percent',$3::text,'new_percent',$4::text),now())`,
			staff.UserID, strconv.FormatInt(id, 10), old, percent)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "rules unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"id": id, "historical_percent": percent})
}

func (s *Service) channels(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT ch.id,ch.name,COALESCE(c.code,''),ch.platform,ch.destination_id,
ch.enabled,ch.private_test,(SELECT count(*) FROM core_publicationroute pr WHERE pr.channel_id=ch.id AND pr.enabled)
FROM core_channel ch LEFT JOIN core_country c ON c.id=ch.country_id ORDER BY c.code,ch.name`)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channels unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, routes int64
		var name, country, platform, destination string
		var enabled, private bool
		if err = rows.Scan(&id, &name, &country, &platform, &destination, &enabled, &private, &routes); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "name": name, "country": country,
			"platform": platform, "destination_id": destination, "enabled": enabled,
			"private_test": private, "active_routes": routes})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channels unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Service) candidates(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	page, offset := pageLimit(r)
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT d.id,l.id,r.code,COALESCE(NULLIF(r.display_name_ar,''),r.code),c.code,
l.title,o.price::text,o.currency,o.seller,o.observed_at,d.percent::text,d.state,d.reason,
d.created_at,d.evidence_path<>'',d.evidence_discount::text,d.featured,d.editorial_note_ar,d.editorial_note_en,
(NOT o.imported AND o.stock_status='in_stock' AND o.observed_at>=now()-interval '2 hours')
 FROM core_dealcandidate d
JOIN core_offerobservation o ON o.id=d.winning_observation_id
JOIN core_listing l ON l.id=o.listing_id JOIN core_retailer r ON r.id=l.retailer_id
JOIN core_country c ON c.id=r.country_id
ORDER BY CASE WHEN d.state='review' THEN 0 ELSE 1 END,d.id DESC LIMIT 51 OFFSET $1`, offset)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "candidates unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, listingID int64
		var store, storeName, country, title, price, currency, seller, percent, state, reason string
		var created, observed time.Time
		var evidence, featured, eligible bool
		var discount *string
		var noteAR, noteEN string
		if err = rows.Scan(&id, &listingID, &store, &storeName, &country, &title, &price, &currency, &seller,
			&observed, &percent, &state, &reason, &created, &evidence, &discount, &featured, &noteAR, &noteEN, &eligible); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "listing_id": listingID, "store": store, "store_name": storeName,
			"country": country, "title": title, "price": price, "currency": currency, "seller": seller,
			"observed_at": observed, "percent": percent, "state": state, "reason": reason, "created_at": created,
			"has_evidence": evidence, "evidence_url": fmt.Sprintf("/api/v1/admin/candidates/%d/evidence", id),
			"displayed_discount": discount, "featured": featured, "editorial_note_ar": noteAR,
			"editorial_note_en": noteEN, "eligible": eligible})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "candidates unavailable"})
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "has_more": more})
}

func (s *Service) publications(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	page, offset := pageLimit(r)
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT p.id,ch.name,p.status,p.attempt_count,p.provider_message_id,
p.created_at,p.sent_at,p.last_error FROM core_publication p
JOIN core_channel ch ON ch.id=p.channel_id ORDER BY p.id DESC LIMIT 51 OFFSET $1`, offset)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "publications unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var channel, status, messageID, lastError string
		var attempts int
		var created time.Time
		var sent *time.Time
		if err = rows.Scan(&id, &channel, &status, &attempts, &messageID, &created, &sent, &lastError); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "channel": channel, "status": status,
			"attempt_count": attempts, "message_id": messageID, "created_at": created,
			"sent_at": sent, "last_error": lastError})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "publications unavailable"})
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "has_more": more})
}

func (s *Service) matches(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	state := r.URL.Query().Get("state")
	if state == "" {
		state = "proposed"
	}
	if state != "proposed" && state != "approved" && state != "rejected" {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid match state"})
		return
	}
	page, offset := pageLimit(r)
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT d.id,l.id,l.title,r.code,p.id,p.brand,p.model_mpn,d.method,d.confidence::text,d.state
FROM core_matchdecision d JOIN core_listing l ON l.id=d.listing_id
JOIN core_retailer r ON r.id=l.retailer_id JOIN core_canonicalproduct p ON p.id=d.canonical_product_id
WHERE d.state=$1 ORDER BY d.id DESC LIMIT 51 OFFSET $2`, state, offset)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match reviews unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id, listingID, productID int64
		var title, store, brand, model, method, confidence, currentState string
		if err = rows.Scan(&id, &listingID, &title, &store, &productID, &brand, &model, &method, &confidence, &currentState); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "listing_id": listingID, "title": title,
			"store": store, "canonical_id": productID, "brand": brand, "model": model,
			"method": method, "confidence": confidence, "state": currentState})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match reviews unavailable"})
		return
	}
	more := len(items) > 50
	if more {
		items = items[:50]
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items, "page": page, "has_more": more})
}

func (s *Service) reviewMatch(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid match decision"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Decision string `json:"decision"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || (input.Decision != "approved" && input.Decision != "rejected") {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid review decision"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match review unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var listingID, canonicalID int64
	var state string
	err = tx.QueryRow(ctx, `SELECT listing_id,canonical_product_id,state FROM core_matchdecision WHERE id=$1 FOR UPDATE`, id).Scan(&listingID, &canonicalID, &state)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "match decision not found"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match review unavailable"})
		return
	}
	if state != "proposed" {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "match was already reviewed"})
		return
	}
	var existing *int64
	if err = tx.QueryRow(ctx, `SELECT canonical_product_id FROM core_listing WHERE id=$1 FOR UPDATE`, listingID).Scan(&existing); err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match review unavailable"})
		return
	}
	if input.Decision == "approved" && existing != nil && *existing != canonicalID {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "listing already belongs to another exact variant"})
		return
	}
	_, err = tx.Exec(ctx, `UPDATE core_matchdecision SET state=$2,reviewer_id=$3,decided_at=now() WHERE id=$1`, id, input.Decision, staff.UserID)
	if err == nil && input.Decision == "approved" {
		_, err = tx.Exec(ctx, `UPDATE core_listing SET canonical_product_id=$2 WHERE id=$1`, listingID, canonicalID)
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_review_match','match_decision',$2,jsonb_build_object('decision',$3::text,
'listing_id',$4::bigint,'canonical_id',$5::bigint),now())`, staff.UserID, strconv.FormatInt(id, 10), input.Decision, listingID, canonicalID)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "match review unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"id": id, "decision": input.Decision})
}
