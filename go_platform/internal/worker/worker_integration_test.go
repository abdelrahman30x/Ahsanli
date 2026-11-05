package worker

import (
	"context"
	"os"
	"testing"
	"time"

	"ahsanly/go_platform/internal/catalog"
	"ahsanly/go_platform/internal/sources"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This test is opt-in and refuses to use the production database. It exercises
// the actual PostgreSQL schema without contacting a retailer or Telegram.
func TestPostgresObservationAndShadowCandidate(t *testing.T) {
	if os.Getenv("DEAL_GO_INTEGRATION") != "1" {
		t.Skip("set DEAL_GO_INTEGRATION=1 with an isolated botafflite_go_test database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(catalog.DatabaseURL())
	if err != nil {
		t.Fatal("invalid test database configuration")
	}
	cfg.ConnConfig.Database = "botafflite_go_test"
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("isolated PostgreSQL pool unavailable")
	}
	defer pool.Close()
	var database string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&database); err != nil || database != "botafflite_go_test" {
		t.Fatal("refusing integration test outside isolated database")
	}
	if _, err = pool.Exec(ctx, `TRUNCATE core_country RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("test fixture reset: %v", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = pool.Exec(cleanup, `TRUNCATE core_country RESTART IDENTITY CASCADE`)
	}()
	var countryID, retailerID, listingID, jobID int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_country(code,name_en,name_ar,currency,currency_label_ar,locale,enabled)
VALUES('ZX','Integration','اختبار','EGP','جنيه','ar',true) RETURNING id`).Scan(&countryID); err != nil {
		t.Fatalf("country fixture: %v", err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_retailer(code,country_id,domain,display_name_ar,currency,enabled,publish_enabled,
permission_reference,max_requests_per_hour,offer_source,health,last_error_class)
VALUES('amazon_eg',$1,'amazon.eg','أمازون','EGP',true,false,'',30,'server_http','healthy','') RETURNING id`, countryID).Scan(&retailerID); err != nil {
		t.Fatalf("retailer fixture: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core_dealrule(name,country_id,historical_percent,cross_store_percent,cooldown_hours,
freshness_hours,cash_floor_under_500,cash_floor_under_2000,cash_floor_under_10000,cash_floor_10000_plus,version,enabled)
VALUES('integration',$1,10,5,24,2,10,50,150,300,1,true)`, countryID)
	if err != nil {
		t.Fatalf("rule fixture: %v", err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_listing(retailer_id,retailer_product_id,original_url,title,
extracted_attributes,tracking_state,cadence,created_at)
VALUES($1,'B0TEST1234','https://www.amazon.eg/dp/B0TEST1234','Integration product','{}'::jsonb,'watch','hot',now()) RETURNING id`, retailerID).Scan(&listingID); err != nil {
		t.Fatalf("listing fixture: %v", err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_fetchjob(listing_id,kind,status,priority,due_at,retry_count,
last_result,last_error_class,created_at)
VALUES($1,'offer','due',1,now(),0,'','',now()) RETURNING id`, listingID).Scan(&jobID); err != nil {
		t.Fatalf("job fixture: %v", err)
	}
	runner := New(pool)
	job := &Job{ID: jobID, ListingID: listingID, RetailerID: retailerID, ProductID: "B0TEST1234", Currency: "EGP", Cadence: "hot"}
	first := sources.Offer{PriceCents: 10000, Currency: "EGP", Seller: "Amazon.eg", Title: "Integration product"}
	if err = runner.recordOffer(ctx, job, first, time.Now()); err != nil {
		t.Fatalf("baseline observation: %v", err)
	}
	var observations, candidates int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_offerobservation WHERE listing_id=$1`, listingID).Scan(&observations); err != nil || observations != 1 {
		t.Fatalf("expected one baseline, got %d: %v", observations, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_dealcandidate`).Scan(&candidates); err != nil || candidates != 0 {
		t.Fatalf("baseline created a deal: %d, %v", candidates, err)
	}
	second := sources.Offer{PriceCents: 8900, Currency: "EGP", Seller: "Amazon.eg", Title: "Integration product"}
	if err = runner.recordOffer(ctx, job, second, time.Now()); err != nil {
		t.Fatalf("drop observation: %v", err)
	}
	var state, percent string
	if err = pool.QueryRow(ctx, `SELECT state,percent::text FROM core_dealcandidate LIMIT 1`).Scan(&state, &percent); err != nil {
		t.Fatalf("deal candidate: %v", err)
	}
	if state != "shadow" || percent != "11.00" {
		t.Fatalf("expected shadow 11%% candidate, got %q %q", state, percent)
	}
	runner.CandidateState = "new"
	if err = runner.recordOffer(ctx, job, sources.Offer{PriceCents: 7800, Currency: "EGP", Seller: "Amazon.eg"}, time.Now()); err == nil {
		t.Fatal("live candidate was accepted while retailer publication was disabled")
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_offerobservation WHERE listing_id=$1`, listingID).Scan(&observations); err != nil || observations != 2 {
		t.Fatalf("rejected live candidate changed history: %d, %v", observations, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE core_retailer SET publish_enabled=true WHERE id=$1`, retailerID); err != nil {
		t.Fatal(err)
	}
	if err = runner.recordOffer(ctx, job, sources.Offer{PriceCents: 7800, Currency: "EGP", Seller: "Amazon.eg"}, time.Now()); err == nil {
		t.Fatal("live candidate was accepted without Go listing ownership")
	}
	if _, err = pool.Exec(ctx, `UPDATE core_listing SET extracted_attributes='{"go_owned":true}'::jsonb WHERE id=$1`, listingID); err != nil {
		t.Fatal(err)
	}
	if err = runner.recordOffer(ctx, job, sources.Offer{PriceCents: 7800, Currency: "EGP", Seller: "Amazon.eg"}, time.Now()); err != nil {
		t.Fatalf("owned live candidate: %v", err)
	}
	if err = pool.QueryRow(ctx, `SELECT state FROM core_dealcandidate ORDER BY id DESC LIMIT 1`).Scan(&state); err != nil || state != "new" {
		t.Fatalf("expected owned live candidate, got %q: %v", state, err)
	}
}
