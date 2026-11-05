package admin

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"ahsanly/go_platform/internal/catalog"
	"ahsanly/go_platform/internal/schema"
	"github.com/jackc/pgx/v5/pgxpool"
)

func integrationCode(secret []byte, at time.Time) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(at.Unix()/30))
	mac := hmac.New(sha1.New, secret)
	_, _ = mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := int(sum[len(sum)-1] & 15)
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", value%1_000_000)
}

func TestIsolatedStaffSessionAndCSRF(t *testing.T) {
	if os.Getenv("DEAL_GO_INTEGRATION") != "1" {
		t.Skip("requires isolated botafflite_go_test PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(catalog.DatabaseURL())
	if err != nil {
		t.Fatal("invalid database configuration")
	}
	cfg.ConnConfig.Database = "botafflite_go_test"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("isolated PostgreSQL pool unavailable")
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "botafflite_go_test" {
		t.Fatal("refusing authentication test outside isolated database")
	}
	if _, err = schema.Apply(ctx, pool, true); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE auth_user RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `TRUNCATE core_country RESTART IDENTITY CASCADE`); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = pool.Exec(cleanup, `TRUNCATE core_country RESTART IDENTITY CASCADE`)
		_, _ = pool.Exec(cleanup, `TRUNCATE auth_user RESTART IDENTITY CASCADE`)
	}()
	var userID int64
	password := "pbkdf2_sha256$10000$integration-salt$1ojjG/dDl5wqb8r2csASu+JkeXhjXft504k0zW/lumo="
	err = pool.QueryRow(ctx, `INSERT INTO auth_user(password,is_superuser,username,first_name,last_name,email,
is_staff,is_active,date_joined) VALUES($1,false,'go-integration','','','',true,true,now()) RETURNING id`, password).Scan(&userID)
	if err != nil {
		t.Fatal(err)
	}
	var countryID, retailerID, listingID, ruleID int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_country(code,name_en,name_ar,currency,currency_label_ar,locale,enabled)
VALUES('ZX','Integration','اختبار','EGP','جنيه','ar',true) RETURNING id`).Scan(&countryID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_retailer(code,country_id,domain,display_name_ar,currency,enabled,publish_enabled,
permission_reference,max_requests_per_hour,offer_source,health,last_error_class)
VALUES('amazon_eg',$1,'amazon.eg','أمازون','EGP',true,false,'',30,'server_http','healthy','') RETURNING id`, countryID).Scan(&retailerID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_dealrule(name,country_id,historical_percent,cross_store_percent,cooldown_hours,
freshness_hours,cash_floor_under_500,cash_floor_under_2000,cash_floor_under_10000,cash_floor_10000_plus,version,enabled)
VALUES('integration',$1,10,5,24,2,10,50,150,300,1,true) RETURNING id`, countryID).Scan(&ruleID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_listing(retailer_id,retailer_product_id,original_url,title,
extracted_attributes,tracking_state,cadence,created_at)
VALUES($1,'B0TEST1234','https://www.amazon.eg/dp/B0TEST1234','Integration product','{}'::jsonb,'paused','hot',now()) RETURNING id`, retailerID).Scan(&listingID); err != nil {
		t.Fatal(err)
	}
	secret := []byte("12345678901234567890")
	_, err = pool.Exec(ctx, `INSERT INTO otp_totp_totpdevice(name,confirmed,key,step,t0,digits,tolerance,drift,
last_t,user_id,throttling_failure_count) VALUES('integration',true,$1,30,0,6,1,0,-1,$2,0)`, hex.EncodeToString(secret), userID)
	if err != nil {
		t.Fatal(err)
	}
	s := &Service{DB: pool, Origin: "https://134.122.89.95", Enabled: true}
	mux := http.NewServeMux()
	s.Register(mux)
	unauthenticated := httptest.NewRecorder()
	mux.ServeHTTP(unauthenticated, httptest.NewRequest("GET", "https://134.122.89.95/api/v1/admin/overview", nil))
	if unauthenticated.Code != http.StatusUnauthorized {
		t.Fatalf("overview exposed to guest: %d", unauthenticated.Code)
	}
	code := integrationCode(secret, time.Now())
	payload, _ := json.Marshal(map[string]string{"username": "go-integration", "password": "correct horse", "totp": code})
	login := httptest.NewRequest("POST", "https://134.122.89.95/api/v1/admin/session", bytes.NewReader(payload))
	login.Header.Set("Origin", s.Origin)
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, login)
	if response.Code != http.StatusOK {
		t.Fatalf("staff login failed: %d %s", response.Code, response.Body.String())
	}
	var result struct {
		CSRF string `json:"csrf_token"`
	}
	if err = json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.CSRF) != 64 {
		t.Fatal("staff session omitted CSRF token")
	}
	cookies := response.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].Secure || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		t.Fatal("staff session cookie is not protected")
	}
	request := httptest.NewRequest("GET", "https://134.122.89.95/api/v1/admin/overview", nil)
	request.AddCookie(cookies[0])
	overview := httptest.NewRecorder()
	mux.ServeHTTP(overview, request)
	if overview.Code != http.StatusOK {
		t.Fatalf("authenticated overview failed: %d %s", overview.Code, overview.Body.String())
	}
	unauthorizedRule := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/rules/%d", ruleID), bytes.NewBufferString(`{"historical_percent":"12.50"}`))
	unauthorizedRule.AddCookie(cookies[0])
	unauthorizedRule.Header.Set("Origin", s.Origin)
	blockedRule := httptest.NewRecorder()
	mux.ServeHTTP(blockedRule, unauthorizedRule)
	if blockedRule.Code != http.StatusForbidden {
		t.Fatalf("rule edit without CSRF accepted: %d", blockedRule.Code)
	}
	editRule := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/rules/%d", ruleID), bytes.NewBufferString(`{"historical_percent":"12.50"}`))
	editRule.AddCookie(cookies[0])
	editRule.Header.Set("Origin", s.Origin)
	editRule.Header.Set("X-CSRF-Token", result.CSRF)
	ruleResponse := httptest.NewRecorder()
	mux.ServeHTTP(ruleResponse, editRule)
	if ruleResponse.Code != http.StatusOK {
		t.Fatalf("rule edit failed: %d %s", ruleResponse.Code, ruleResponse.Body.String())
	}
	var version int
	if err = pool.QueryRow(ctx, `SELECT version FROM core_dealrule WHERE id=$1`, ruleID).Scan(&version); err != nil || version != 2 {
		t.Fatalf("rule version not advanced: %d, %v", version, err)
	}
	editListing := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/listings/%d", listingID), bytes.NewBufferString(`{"tracking_state":"watch","cadence":"hot"}`))
	editListing.AddCookie(cookies[0])
	editListing.Header.Set("Origin", s.Origin)
	editListing.Header.Set("X-CSRF-Token", result.CSRF)
	listingResponse := httptest.NewRecorder()
	mux.ServeHTTP(listingResponse, editListing)
	if listingResponse.Code != http.StatusOK {
		t.Fatalf("watchlist edit failed: %d %s", listingResponse.Code, listingResponse.Body.String())
	}
	var jobStatus string
	if err = pool.QueryRow(ctx, `SELECT status FROM core_fetchjob WHERE listing_id=$1 AND kind='offer'`, listingID).Scan(&jobStatus); err != nil || jobStatus != "due" {
		t.Fatalf("watch job missing: %q, %v", jobStatus, err)
	}
	var auditCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_auditevent WHERE actor_id=$1`, userID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("admin edits not audited: %d, %v", auditCount, err)
	}
	var canonicalID, matchID int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_canonicalproduct(brand,model_mpn,gtin,category,variant_attributes,
review_state,paused,created_at) VALUES('Test','Model','','phones','{}'::jsonb,'pending',false,now()) RETURNING id`).Scan(&canonicalID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_matchdecision(listing_id,canonical_product_id,method,confidence,rule_version,state,created_at)
VALUES($1,$2,'variant_review',0.5,'1','proposed',now()) RETURNING id`, listingID, canonicalID).Scan(&matchID); err != nil {
		t.Fatal(err)
	}
	review := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/matches/%d", matchID), bytes.NewBufferString(`{"decision":"approved"}`))
	review.AddCookie(cookies[0])
	review.Header.Set("Origin", s.Origin)
	review.Header.Set("X-CSRF-Token", result.CSRF)
	reviewResponse := httptest.NewRecorder()
	mux.ServeHTTP(reviewResponse, review)
	if reviewResponse.Code != http.StatusOK {
		t.Fatalf("match review failed: %d %s", reviewResponse.Code, reviewResponse.Body.String())
	}
	var assigned *int64
	if err = pool.QueryRow(ctx, `SELECT canonical_product_id FROM core_listing WHERE id=$1`, listingID).Scan(&assigned); err != nil || assigned == nil || *assigned != canonicalID {
		t.Fatalf("approved exact variant not assigned: %v, %v", assigned, err)
	}
	var channelID, routeID int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_channel(name,country_id,platform,destination_id,telegram_chat_id,bot_token_env,enabled,private_test)
VALUES('Integration route',$1,'telegram','-1001234567','-1001234567','TEST_TOKEN',false,true) RETURNING id`, countryID).Scan(&channelID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_publicationroute(country_id,channel_id,retailer_id,enabled)
VALUES($1,$2,$3,false) RETURNING id`, countryID, channelID, retailerID).Scan(&routeID); err != nil {
		t.Fatal(err)
	}
	blockedRoute := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/routes/%d", routeID), bytes.NewBufferString(`{"enabled":true}`))
	blockedRoute.AddCookie(cookies[0])
	blockedRoute.Header.Set("Origin", s.Origin)
	blockedRoute.Header.Set("X-CSRF-Token", result.CSRF)
	blockedRouteResponse := httptest.NewRecorder()
	mux.ServeHTTP(blockedRouteResponse, blockedRoute)
	if blockedRouteResponse.Code != http.StatusConflict {
		t.Fatalf("route enabled with disabled destination: %d", blockedRouteResponse.Code)
	}
	if _, err = pool.Exec(ctx, `UPDATE core_channel SET enabled=true WHERE id=$1`, channelID); err != nil {
		t.Fatal(err)
	}
	routeRequest := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/routes/%d", routeID), bytes.NewBufferString(`{"enabled":true}`))
	routeRequest.AddCookie(cookies[0])
	routeRequest.Header.Set("Origin", s.Origin)
	routeRequest.Header.Set("X-CSRF-Token", result.CSRF)
	routeResponse := httptest.NewRecorder()
	mux.ServeHTTP(routeResponse, routeRequest)
	if routeResponse.Code != http.StatusOK {
		t.Fatalf("route edit failed: %d %s", routeResponse.Code, routeResponse.Body.String())
	}
	var routeEnabled bool
	if err = pool.QueryRow(ctx, `SELECT enabled FROM core_publicationroute WHERE id=$1`, routeID).Scan(&routeEnabled); err != nil || !routeEnabled {
		t.Fatalf("route update missing: %v, %v", routeEnabled, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_auditevent WHERE action='go_update_publication_route' AND actor_id=$1`, userID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("route edit not audited: %d, %v", auditCount, err)
	}
	createCountryRoute := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest("POST", "https://134.122.89.95/api/v1/admin/routes",
			bytes.NewBufferString(fmt.Sprintf(`{"channel_id":%d,"retailer_id":null}`, channelID)))
		request.AddCookie(cookies[0])
		request.Header.Set("Origin", s.Origin)
		request.Header.Set("X-CSRF-Token", result.CSRF)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		return response
	}
	created := createCountryRoute()
	if created.Code != http.StatusCreated {
		t.Fatalf("country route creation failed: %d %s", created.Code, created.Body.String())
	}
	if duplicate := createCountryRoute(); duplicate.Code != http.StatusConflict {
		t.Fatalf("duplicate country route accepted: %d", duplicate.Code)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_publicationroute WHERE channel_id=$1 AND retailer_id IS NULL AND NOT enabled`, channelID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("new country route was not disabled: %d, %v", auditCount, err)
	}
	newChannel := httptest.NewRequest("POST", "https://134.122.89.95/api/v1/admin/channels",
		bytes.NewBufferString(fmt.Sprintf(`{"name":"Second test channel","country_id":%d,"destination_id":"@test_channel_2","bot_token_env":"DEAL_TELEGRAM_TOKEN_TEST","private_test":true}`, countryID)))
	newChannel.AddCookie(cookies[0])
	newChannel.Header.Set("Origin", s.Origin)
	newChannel.Header.Set("X-CSRF-Token", result.CSRF)
	newChannelResponse := httptest.NewRecorder()
	mux.ServeHTTP(newChannelResponse, newChannel)
	if newChannelResponse.Code != http.StatusCreated {
		t.Fatalf("channel creation failed: %d %s", newChannelResponse.Code, newChannelResponse.Body.String())
	}
	var secondChannelID int64
	if err = pool.QueryRow(ctx, `SELECT id FROM core_channel WHERE destination_id='@test_channel_2' AND NOT enabled`).Scan(&secondChannelID); err != nil {
		t.Fatalf("new channel was not disabled: %v", err)
	}
	channelToggle := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/channels/%d", secondChannelID),
			bytes.NewBufferString(`{"enabled":true}`))
		request.AddCookie(cookies[0])
		request.Header.Set("Origin", s.Origin)
		request.Header.Set("X-CSRF-Token", result.CSRF)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		return response
	}
	if noToken := channelToggle(); noToken.Code != http.StatusConflict {
		t.Fatalf("channel enabled without configured token reference: %d", noToken.Code)
	}
	t.Setenv("DEAL_TELEGRAM_TOKEN_TEST", "integration-placeholder")
	if activated := channelToggle(); activated.Code != http.StatusOK {
		t.Fatalf("channel activation failed: %d %s", activated.Code, activated.Body.String())
	}
	var observationID, candidateID int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_offerobservation(listing_id,price,currency,seller,stock_status,source,observed_at,imported,legacy_source)
 VALUES($1,85,'EGP','Amazon.eg','in_stock','go_http',now(),false,'') RETURNING id`, listingID).Scan(&observationID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO core_currentoffer(listing_id,observation_id,updated_at) VALUES($1,$2,now())`, listingID, observationID); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_dealcandidate(winning_observation_id,compared_observations,historical_baseline,rule_version,reason,percent,state,evidence_path,evidence_price,evidence_seller,capture_retries,created_at)
 VALUES($1,'[]'::jsonb,100,2,'historical',15,'review','/tmp/integration-evidence.png',85,'Amazon.eg',0,now()) RETURNING id`, observationID).Scan(&candidateID); err != nil {
		t.Fatal(err)
	}
	candidateReview := func() *httptest.ResponseRecorder {
		request := httptest.NewRequest("PATCH", fmt.Sprintf("https://134.122.89.95/api/v1/admin/candidates/%d", candidateID),
			bytes.NewBufferString(`{"decision":"approved","featured":true,"editorial_note_ar":"اختيار اختباري","editorial_note_en":"Integration pick"}`))
		request.AddCookie(cookies[0])
		request.Header.Set("Origin", s.Origin)
		request.Header.Set("X-CSRF-Token", result.CSRF)
		response := httptest.NewRecorder()
		mux.ServeHTTP(response, request)
		return response
	}
	if ineligible := candidateReview(); ineligible.Code != http.StatusConflict {
		t.Fatalf("ineligible candidate was approved: %d %s", ineligible.Code, ineligible.Body.String())
	}
	if _, err = pool.Exec(ctx, `UPDATE core_retailer SET publish_enabled=true WHERE id=$1`, retailerID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `UPDATE core_listing SET extracted_attributes='{"go_owned":true}'::jsonb WHERE id=$1`, listingID); err != nil {
		t.Fatal(err)
	}
	if approved := candidateReview(); approved.Code != http.StatusOK {
		t.Fatalf("editorial approval failed: %d %s", approved.Code, approved.Body.String())
	}
	var reviewerID int64
	var featured bool
	var note string
	if err = pool.QueryRow(ctx, `SELECT reviewer_id,featured,editorial_note_ar FROM core_dealcandidate WHERE id=$1 AND state='approved' AND reviewed_at IS NOT NULL`, candidateID).Scan(&reviewerID, &featured, &note); err != nil || reviewerID != userID || !featured || note == "" {
		t.Fatalf("editorial review was not recorded: %v", err)
	}
	if replayed := candidateReview(); replayed.Code != http.StatusConflict {
		t.Fatalf("candidate approval replay accepted: %d", replayed.Code)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_auditevent WHERE action='go_review_deal_candidate' AND actor_id=$1`, userID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("candidate review not audited: %d, %v", auditCount, err)
	}
	replay := httptest.NewRequest("POST", "https://134.122.89.95/api/v1/admin/session", bytes.NewReader(payload))
	replay.Header.Set("Origin", s.Origin)
	replayResponse := httptest.NewRecorder()
	mux.ServeHTTP(replayResponse, replay)
	if replayResponse.Code != http.StatusForbidden {
		t.Fatalf("TOTP replay accepted: %d", replayResponse.Code)
	}
	logout := httptest.NewRequest("DELETE", "https://134.122.89.95/api/v1/admin/session", nil)
	logout.AddCookie(cookies[0])
	logout.Header.Set("Origin", s.Origin)
	blocked := httptest.NewRecorder()
	mux.ServeHTTP(blocked, logout)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF accepted: %d", blocked.Code)
	}
	logout.Header.Set("X-CSRF-Token", result.CSRF)
	loggedOut := httptest.NewRecorder()
	mux.ServeHTTP(loggedOut, logout)
	if loggedOut.Code != http.StatusOK {
		t.Fatalf("logout failed: %d", loggedOut.Code)
	}
	after := httptest.NewRecorder()
	mux.ServeHTTP(after, request)
	if after.Code != http.StatusUnauthorized {
		t.Fatalf("old session survived logout: %d", after.Code)
	}
}
