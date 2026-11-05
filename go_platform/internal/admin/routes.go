package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Routes are separate from channels: one destination can receive country-wide
// deals or only offers from a selected retailer. A disabled route never sends.
func (s *Service) routes(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT pr.id,c.code,ch.name,COALESCE(rt.code,''),pr.enabled,ch.enabled
FROM core_publicationroute pr JOIN core_country c ON c.id=pr.country_id
JOIN core_channel ch ON ch.id=pr.channel_id
LEFT JOIN core_retailer rt ON rt.id=pr.retailer_id
ORDER BY c.code,ch.name,rt.code NULLS FIRST,pr.id`)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "routes unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var country, channel, retailer string
		var enabled, channelEnabled bool
		if err = rows.Scan(&id, &country, &channel, &retailer, &enabled, &channelEnabled); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "country": country, "channel": channel,
			"retailer": retailer, "enabled": enabled, "channel_enabled": channelEnabled})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "routes unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items})
}

// Creating a route never activates it. Staff can inspect it and explicitly
// enable it later; the global publication gates remain independent.
func (s *Service) createRoute(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		ChannelID  int64  `json:"channel_id"`
		RetailerID *int64 `json:"retailer_id"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF || input.ChannelID <= 0 ||
		(input.RetailerID != nil && *input.RetailerID <= 0) {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route selection"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var countryID *int64
	err = tx.QueryRow(ctx, `SELECT country_id FROM core_channel WHERE id=$1`, input.ChannelID).Scan(&countryID)
	if errors.Is(err, pgx.ErrNoRows) || countryID == nil {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "channel needs a country"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
		return
	}
	if input.RetailerID != nil {
		var retailerCountryID *int64
		err = tx.QueryRow(ctx, `SELECT country_id FROM core_retailer WHERE id=$1`, *input.RetailerID).Scan(&retailerCountryID)
		if errors.Is(err, pgx.ErrNoRows) || retailerCountryID == nil || *retailerCountryID != *countryID {
			adminJSON(w, http.StatusBadRequest, map[string]string{"error": "retailer belongs to another country"})
			return
		}
		if err != nil {
			adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
			return
		}
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO core_publicationroute(country_id,channel_id,retailer_id,enabled)
VALUES($1,$2,$3,false) RETURNING id`, *countryID, input.ChannelID, input.RetailerID).Scan(&id)
	var dbError *pgconn.PgError
	if errors.As(err, &dbError) && dbError.Code == "23505" {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "route already exists"})
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_create_publication_route','publication_route',$2,
jsonb_build_object('country_id',$3::bigint,'channel_id',$4::bigint,'retailer_id',$5::bigint,'enabled',false),now())`,
			staff.UserID, strconv.FormatInt(id, 10), *countryID, input.ChannelID, input.RetailerID)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
		return
	}
	adminJSON(w, http.StatusCreated, map[string]any{"id": id, "enabled": false})
}

func (s *Service) updateRoute(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil || input.Enabled == nil {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid route state"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var old, channelEnabled, countryEnabled, retailerEnabled bool
	var routeCountryID int64
	var channelCountryID, retailerCountryID *int64
	err = tx.QueryRow(ctx, `SELECT pr.enabled,ch.enabled,c.enabled,COALESCE(rt.enabled,false),
ch.country_id,pr.country_id,rt.country_id
FROM core_publicationroute pr JOIN core_channel ch ON ch.id=pr.channel_id
JOIN core_country c ON c.id=pr.country_id
LEFT JOIN core_retailer rt ON rt.id=pr.retailer_id WHERE pr.id=$1 FOR UPDATE OF pr`, id).Scan(
		&old, &channelEnabled, &countryEnabled, &retailerEnabled,
		&channelCountryID, &routeCountryID, &retailerCountryID)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "route not found"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
		return
	}
	if *input.Enabled && (!channelEnabled || !countryEnabled || channelCountryID == nil || *channelCountryID != routeCountryID ||
		(retailerCountryID != nil && (!retailerEnabled || *retailerCountryID != routeCountryID))) {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "route destination or retailer is not enabled for this country"})
		return
	}
	if old == *input.Enabled {
		adminJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": old})
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE core_publicationroute SET enabled=$2 WHERE id=$1`, id, *input.Enabled); err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_update_publication_route','publication_route',$2,
jsonb_build_object('old_enabled',$3::boolean,'new_enabled',$4::boolean),now())`,
			staff.UserID, strconv.FormatInt(id, 10), old, *input.Enabled)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "route unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": *input.Enabled})
}
