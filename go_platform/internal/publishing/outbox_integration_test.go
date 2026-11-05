package publishing

import (
	"context"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"ahsanly/go_platform/internal/catalog"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in database test; no Telegram HTTP request is made.
func TestPostgresPrivateRouteAndUncertainDelivery(t *testing.T) {
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
	cfg.MaxConns = 2
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal("isolated PostgreSQL pool unavailable")
	}
	defer pool.Close()
	var name string
	if err = pool.QueryRow(ctx, `SELECT current_database()`).Scan(&name); err != nil || name != "botafflite_go_test" {
		t.Fatal("refusing integration test outside isolated database")
	}
	if _, err = pool.Exec(ctx, `TRUNCATE core_country RESTART IDENTITY CASCADE`); err != nil {
		t.Fatalf("fixture reset: %v", err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_, _ = pool.Exec(cleanup, `TRUNCATE core_country RESTART IDENTITY CASCADE`)
	}()
	dir := t.TempDir()
	photoPath := filepath.Join(dir, "candidate.png")
	photo, err := os.OpenFile(photoPath, os.O_CREATE|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = png.Encode(photo, image.NewRGBA(image.Rect(0, 0, 2, 2))); err != nil {
		photo.Close()
		t.Fatal(err)
	}
	if err = photo.Close(); err != nil {
		t.Fatal(err)
	}
	var countryID, retailerID, listingID, observationID, candidateID, privateID, liveID int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_country(code,name_en,name_ar,currency,currency_label_ar,locale,enabled,enabled_public_site)
VALUES('ZX','Integration','اختبار','EGP','جنيه','ar',true,true) RETURNING id`).Scan(&countryID); err != nil {
		t.Fatalf("country fixture: %v", err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_retailer(code,country_id,domain,display_name_ar,currency,enabled,publish_enabled,
permission_reference,max_requests_per_hour,offer_source,health,last_error_class)
VALUES('amazon_eg',$1,'amazon.eg','أمازون','EGP',true,true,'',30,'server_http','healthy','') RETURNING id`, countryID).Scan(&retailerID); err != nil {
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
	if err = pool.QueryRow(ctx, `INSERT INTO core_offerobservation(listing_id,price,currency,seller,stock_status,source,
observed_at,imported,legacy_source)
VALUES($1,89,'EGP','Amazon.eg','in_stock','go_http',now(),false,'') RETURNING id`, listingID).Scan(&observationID); err != nil {
		t.Fatalf("observation fixture: %v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO core_currentoffer(listing_id,observation_id,updated_at) VALUES($1,$2,now())`, listingID, observationID)
	if err != nil {
		t.Fatalf("current offer fixture: %v", err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_dealcandidate(winning_observation_id,compared_observations,historical_baseline,
rule_version,reason,percent,state,evidence_path,evidence_price,evidence_seller,capture_retries,created_at)
VALUES($1,'[]'::jsonb,100,1,'historical',11,'review',$2,89,'Amazon.eg',0,now()) RETURNING id`, observationID, photoPath).Scan(&candidateID); err != nil {
		t.Fatalf("candidate fixture: %v", err)
	}
	for _, channel := range []struct {
		name, destination string
		private           bool
		id                *int64
	}{
		{"Private integration", "-100000001", true, &privateID},
		{"Live integration", "-100000002", false, &liveID},
	} {
		if err = pool.QueryRow(ctx, `INSERT INTO core_channel(name,country_id,platform,destination_id,telegram_chat_id,bot_token_env,enabled,private_test)
VALUES($1,$2,'telegram',$3,$3,'DEAL_TEST_TOKEN',true,$4) RETURNING id`, channel.name, countryID, channel.destination, channel.private).Scan(channel.id); err != nil {
			t.Fatalf("channel fixture: %v", err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO core_publicationroute(country_id,retailer_id,channel_id,enabled) VALUES($1,$2,$3,true)`, countryID, retailerID, *channel.id)
		if err != nil {
			t.Fatalf("route fixture: %v", err)
		}
		_, err = pool.Exec(ctx, `INSERT INTO core_affiliatelink(listing_id,channel_id,tracking_url,verified_at)
VALUES($1,$2,'https://www.amazon.eg/dp/B0TEST1234?tag=integration',now())`, listingID, *channel.id)
		if err != nil {
			t.Fatalf("affiliate fixture: %v", err)
		}
	}
	box := &Outbox{DB: pool, CaptureDir: dir, Enabled: true}
	if worked, err := box.PrepareOne(ctx); err != nil || worked {
		t.Fatalf("unowned listing prepared for publication: worked=%v err=%v", worked, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE core_listing SET extracted_attributes='{"go_owned":true}'::jsonb WHERE id=$1`, listingID); err != nil {
		t.Fatalf("mark Go ownership: %v", err)
	}
	if worked, err := box.PrepareOne(ctx); err != nil || worked {
		t.Fatalf("unreviewed candidate prepared for publication: worked=%v err=%v", worked, err)
	}
	publicCatalog := &catalog.Store{DB: pool}
	hidden, err := publicCatalog.ListDeals(ctx, catalog.DealFilter{Country: "ZX"})
	if err != nil || len(hidden.Items) != 0 {
		t.Fatalf("unreviewed candidate appeared publicly: %+v, %v", hidden, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE core_dealcandidate SET state='approved',reviewed_at=now(),evidence_discount=20 WHERE id=$1`, candidateID); err != nil {
		t.Fatal(err)
	}
	visible, err := publicCatalog.ListDeals(ctx, catalog.DealFilter{Country: "ZX"})
	if err != nil || len(visible.Items) != 1 || visible.Items[0].DisplayedDiscount == nil || *visible.Items[0].DisplayedDiscount != "20.00" {
		t.Fatalf("approved deal or store discount missing: %+v, %v", visible, err)
	}
	wrongCountry, err := publicCatalog.ListDeals(ctx, catalog.DealFilter{Country: "EG"})
	if err != nil || len(wrongCountry.Items) != 0 {
		t.Fatalf("deal leaked to another country: %+v, %v", wrongCountry, err)
	}
	if detail, detailErr := publicCatalog.GetDeal(ctx, listingID, "ZX"); detailErr != nil || detail.DealID != candidateID {
		t.Fatalf("approved deal detail unavailable: %+v, %v", detail, detailErr)
	}
	if _, detailErr := publicCatalog.GetDeal(ctx, listingID, "EG"); detailErr == nil {
		t.Fatal("deal detail leaked to another country")
	}
	if _, err = pool.Exec(ctx, `UPDATE core_offerobservation SET observed_at=now()-interval '3 hours' WHERE id=$1`, observationID); err != nil {
		t.Fatal(err)
	}
	stale, err := publicCatalog.ListDeals(ctx, catalog.DealFilter{Country: "ZX"})
	if err != nil || len(stale.Items) != 0 {
		t.Fatalf("stale approved deal remained public: %+v, %v", stale, err)
	}
	if _, err = pool.Exec(ctx, `UPDATE core_offerobservation SET observed_at=now() WHERE id=$1`, observationID); err != nil {
		t.Fatal(err)
	}
	worked, err := box.PrepareOne(ctx)
	if err != nil || !worked {
		t.Fatalf("prepare private photo: worked=%v err=%v", worked, err)
	}
	var publications, privatePublications, livePublications int
	if err = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER (WHERE channel_id=$1),count(*) FILTER (WHERE channel_id=$2)
FROM core_publication`, privateID, liveID).Scan(&publications, &privatePublications, &livePublications); err != nil {
		t.Fatal(err)
	}
	if publications != 1 || privatePublications != 1 || livePublications != 0 {
		t.Fatalf("private routing leaked to live channel: total=%d private=%d live=%d", publications, privatePublications, livePublications)
	}
	// Repeated preparation cannot duplicate a channel/observation publication.
	_, err = pool.Exec(ctx, `UPDATE core_dealcandidate SET state='approved' WHERE id=$1`, candidateID)
	if err != nil {
		t.Fatal(err)
	}
	worked, err = box.PrepareOne(ctx)
	if err != nil || !worked {
		t.Fatalf("idempotent prepare: worked=%v err=%v", worked, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_publication`).Scan(&publications); err != nil || publications != 1 {
		t.Fatalf("duplicate publication: %d, %v", publications, err)
	}
	// Claim the photo without contacting Telegram; a lost response must become
	// uncertain rather than an automatic second upload.
	t.Setenv("DEAL_TEST_TOKEN", "integration-placeholder")
	pending, err := box.claimPending(ctx)
	if err != nil || pending == nil || pending.Destination != "-100000001" {
		t.Fatalf("private publication claim: %v, %v", pending, err)
	}
	_, err = pool.Exec(ctx, `UPDATE core_publication SET delivery_lease_until=now()-interval '1 minute' WHERE id=$1`, pending.ID)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := box.RecoverUncertain(ctx)
	if err != nil || changed != 1 {
		t.Fatalf("uncertain delivery recovery: changed=%d err=%v", changed, err)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM core_publication WHERE id=$1`, pending.ID).Scan(&status); err != nil || status != "uncertain" {
		t.Fatalf("uncertain publication status: %q, %v", status, err)
	}
	box.AllowLiveChannels = true
	_, err = pool.Exec(ctx, `UPDATE core_dealcandidate SET state='approved' WHERE id=$1`, candidateID)
	if err != nil {
		t.Fatal(err)
	}
	worked, err = box.PrepareOne(ctx)
	if err != nil || !worked {
		t.Fatalf("explicit live route preparation: worked=%v err=%v", worked, err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM core_publication WHERE channel_id=$1`, liveID).Scan(&livePublications); err != nil || livePublications != 1 {
		t.Fatalf("explicit live route not prepared: %d, %v", livePublications, err)
	}

	var secondListing, secondObservation int64
	if err = pool.QueryRow(ctx, `INSERT INTO core_listing(retailer_id,retailer_product_id,original_url,title,extracted_attributes,tracking_state,cadence,created_at)
 VALUES($1,'B0TEST5678','https://www.amazon.eg/dp/B0TEST5678','Second integration deal','{"go_owned":true}'::jsonb,'watch','hot',now()) RETURNING id`, retailerID).Scan(&secondListing); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `INSERT INTO core_offerobservation(listing_id,price,currency,seller,stock_status,source,observed_at,imported,legacy_source)
 VALUES($1,75,'EGP','Amazon.eg','in_stock','go_http',now(),false,'') RETURNING id`, secondListing).Scan(&secondObservation); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO core_currentoffer(listing_id,observation_id,updated_at) VALUES($1,$2,now())`, secondListing, secondObservation); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO core_dealcandidate(winning_observation_id,compared_observations,historical_baseline,rule_version,reason,percent,state,evidence_path,evidence_price,evidence_seller,evidence_discount,capture_retries,created_at,reviewed_at,featured)
 VALUES($1,'[]'::jsonb,100,1,'historical',25,'approved',$2,75,'Amazon.eg',25,0,now(),now(),true)`, secondObservation, photoPath); err != nil {
		t.Fatal(err)
	}
	firstPage, err := publicCatalog.ListDeals(ctx, catalog.DealFilter{Country: "ZX", Limit: 1})
	if err != nil || len(firstPage.Items) != 1 || !firstPage.HasMore || firstPage.NextCursor == "" || !firstPage.Items[0].Featured {
		t.Fatalf("first curated page: %+v, %v", firstPage, err)
	}
	secondPage, err := publicCatalog.ListDeals(ctx, catalog.DealFilter{Country: "ZX", Limit: 1, Cursor: firstPage.NextCursor})
	if err != nil || len(secondPage.Items) != 1 || secondPage.Items[0].ID == firstPage.Items[0].ID || secondPage.HasMore {
		t.Fatalf("cursor duplicated or omitted a deal: first=%+v second=%+v err=%v", firstPage, secondPage, err)
	}
}
