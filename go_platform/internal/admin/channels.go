package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var telegramDestination = regexp.MustCompile(`^(?:@[A-Za-z0-9_]{5,32}|-?[0-9]{5,20})$`)
var tokenEnvName = regexp.MustCompile(`^DEAL_TELEGRAM_TOKEN_[A-Z0-9_]{1,80}$`)

func (s *Service) countries(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	rows, err := s.DB.Query(ctx, `SELECT id,code,name_en,currency,enabled FROM core_country ORDER BY code`)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "countries unavailable"})
		return
	}
	defer rows.Close()
	items := []map[string]any{}
	for rows.Next() {
		var id int64
		var code, name, currency string
		var enabled bool
		if err = rows.Scan(&id, &code, &name, &currency, &enabled); err != nil {
			break
		}
		items = append(items, map[string]any{"id": id, "code": code, "name": name,
			"currency": currency, "enabled": enabled})
	}
	if err != nil || rows.Err() != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "countries unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"items": items})
}

// A newly configured channel starts disabled. The token itself stays only in
// the protected process environment; this stores its variable name.
func (s *Service) createChannel(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2048)
	var input struct {
		Name          string `json:"name"`
		CountryID     int64  `json:"country_id"`
		DestinationID string `json:"destination_id"`
		BotTokenEnv   string `json:"bot_token_env"`
		PrivateTest   bool   `json:"private_test"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid channel request"})
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.DestinationID = strings.TrimSpace(input.DestinationID)
	input.BotTokenEnv = strings.TrimSpace(input.BotTokenEnv)
	if input.CountryID <= 0 || input.Name == "" || len([]rune(input.Name)) > 100 ||
		!telegramDestination.MatchString(input.DestinationID) || !tokenEnvName.MatchString(input.BotTokenEnv) {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid Telegram channel settings"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channel unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var countryEnabled bool
	err = tx.QueryRow(ctx, `SELECT enabled FROM core_country WHERE id=$1`, input.CountryID).Scan(&countryEnabled)
	if errors.Is(err, pgx.ErrNoRows) || !countryEnabled {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "country is unavailable"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channel unavailable"})
		return
	}
	var id int64
	err = tx.QueryRow(ctx, `INSERT INTO core_channel(name,country_id,platform,destination_id,telegram_chat_id,
bot_token_env,enabled,private_test) VALUES($1,$2,'telegram',$3,NULL,$4,false,$5) RETURNING id`,
		input.Name, input.CountryID, input.DestinationID, input.BotTokenEnv, input.PrivateTest).Scan(&id)
	var dbError *pgconn.PgError
	if errors.As(err, &dbError) && dbError.Code == "23505" {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "destination already exists"})
		return
	}
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_create_channel','channel',$2,jsonb_build_object('country_id',$3::bigint,
'destination_id',$4::text,'private_test',$5::boolean,'enabled',false),now())`,
			staff.UserID, strconv.FormatInt(id, 10), input.CountryID, input.DestinationID, input.PrivateTest)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channel unavailable"})
		return
	}
	adminJSON(w, http.StatusCreated, map[string]any{"id": id, "enabled": false})
}

func (s *Service) updateChannel(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid channel"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 1024)
	var input struct {
		Enabled *bool `json:"enabled"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF || input.Enabled == nil {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid channel state"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channel unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var old, countryEnabled bool
	var platform, destination, envName string
	err = tx.QueryRow(ctx, `SELECT ch.enabled,COALESCE(c.enabled,false),ch.platform,
COALESCE(NULLIF(ch.destination_id,''),ch.telegram_chat_id,''),ch.bot_token_env
FROM core_channel ch LEFT JOIN core_country c ON c.id=ch.country_id WHERE ch.id=$1 FOR UPDATE OF ch`, id).Scan(
		&old, &countryEnabled, &platform, &destination, &envName)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "channel not found"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channel unavailable"})
		return
	}
	if *input.Enabled && (!countryEnabled || platform != "telegram" ||
		!telegramDestination.MatchString(destination) || !tokenEnvName.MatchString(envName) || os.Getenv(envName) == "") {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "country, destination or server token reference is not ready"})
		return
	}
	if old == *input.Enabled {
		adminJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": old})
		return
	}
	if _, err = tx.Exec(ctx, `UPDATE core_channel SET enabled=$2 WHERE id=$1`, id, *input.Enabled); err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES($1,'go_update_channel','channel',$2,
jsonb_build_object('old_enabled',$3::boolean,'new_enabled',$4::boolean),now())`,
			staff.UserID, strconv.FormatInt(id, 10), old, *input.Enabled)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "channel unavailable"})
		return
	}
	adminJSON(w, http.StatusOK, map[string]any{"id": id, "enabled": *input.Enabled})
}
