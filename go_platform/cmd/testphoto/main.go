// Command testphoto sends one clearly labelled Telegram photo test. It never
// claims a price drop and does not enable the production deal publisher.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"ahsanly/go_platform/internal/capture"
	"ahsanly/go_platform/internal/catalog"
	"ahsanly/go_platform/internal/publishing"
	"ahsanly/go_platform/internal/sources"
)

func main() {
	listingID := flag.Int64("listing", 0, "watched Amazon listing ID")
	channelID := flag.Int64("channel", 0, "Telegram channel ID")
	browser := flag.String("browser", "", "headless Chromium executable")
	output := flag.String("output", "", "new PNG inside the evidence directory")
	send := flag.Bool("send", false, "send one clearly labelled test photo")
	allowPublic := flag.Bool("allow-public-channel", false, "explicitly permit the current public channel for this test")
	flag.Parse()
	if *listingID <= 0 || *channelID <= 0 || *browser == "" || *output == "" ||
		(*send && !*allowPublic) {
		fmt.Fprintln(os.Stderr, "listing, channel, browser and output are required; sending also requires --allow-public-channel")
		os.Exit(2)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 75*time.Second)
	defer cancel()
	store, err := catalog.New(ctx, catalog.DatabaseURL())
	if err != nil {
		fail(errors.New("database unavailable"))
	}
	defer store.DB.Close()
	var title, rawURL, price, seller, destination, tokenEnv, affiliateURL string
	var observedAt time.Time
	var private bool
	err = store.DB.QueryRow(ctx, `SELECT l.title,l.original_url,o.price::text,o.seller,o.observed_at,
COALESCE(NULLIF(ch.destination_id,''),ch.telegram_chat_id,''),ch.bot_token_env,a.tracking_url,ch.private_test
FROM core_listing l JOIN core_retailer rt ON rt.id=l.retailer_id
JOIN core_currentoffer co ON co.listing_id=l.id
JOIN core_offerobservation o ON o.id=co.observation_id
JOIN core_channel ch ON ch.id=$2 AND ch.country_id=rt.country_id
JOIN core_affiliatelink a ON a.listing_id=l.id AND a.channel_id=ch.id
WHERE l.id=$1 AND l.tracking_state='watch' AND rt.code='amazon_eg' AND rt.enabled
AND o.imported=false AND o.stock_status='in_stock' AND o.observed_at>=now()-interval '2 hours'
AND ch.enabled AND ch.platform='telegram' AND a.verified_at IS NOT NULL
AND EXISTS (SELECT 1 FROM core_publicationroute pr WHERE pr.channel_id=ch.id AND pr.enabled
AND pr.country_id=rt.country_id AND (pr.retailer_id IS NULL OR pr.retailer_id=rt.id))`,
		*listingID, *channelID).Scan(&title, &rawURL, &price, &seller, &observedAt,
		&destination, &tokenEnv, &affiliateURL, &private)
	if err != nil || destination == "" || seller == "" || (!private && !*allowPublic) {
		fail(errors.New("fresh routed and verified offer unavailable"))
	}
	cents, err := sources.ParsePrice(price)
	if err != nil {
		fail(errors.New("invalid observed price"))
	}
	evidence, err := capture.CaptureAmazon(ctx, *browser, rawURL, *output,
		sources.Offer{PriceCents: cents, Currency: "EGP", Seller: seller})
	if err != nil {
		fail(fmt.Errorf("screenshot verification failed: %w", err))
	}
	file, err := os.Open(evidence.Path)
	if err != nil {
		fail(err)
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 || info.Size() > 10*1024*1024 {
		file.Close()
		fail(errors.New("invalid screenshot file"))
	}
	if _, err = png.DecodeConfig(file); err != nil {
		file.Close()
		fail(errors.New("invalid screenshot PNG"))
	}
	file.Close()
	if evidence.PriceCents != cents || !strings.EqualFold(evidence.Seller, seller) {
		fail(errors.New("screenshot differs from direct observation"))
	}
	fmt.Printf("verified screenshot=%s price=%s seller=%s observed=%s\n",
		filepath.Base(evidence.Path), price, seller, observedAt.UTC().Format(time.RFC3339))
	if !*send {
		return
	}
	token := os.Getenv(tokenEnv)
	sender, err := publishing.NewTelegramSender(token)
	if err != nil {
		fail(errors.New("Telegram token environment reference unavailable"))
	}
	runes := []rune(title)
	if len(runes) > 180 {
		runes = append(runes[:179], '…')
	}
	caption := fmt.Sprintf("🧪 اختبار صورة المنتج من أحسنلي | AHSANLY\n\nهذه رسالة تجربة للصورة والبوكسات، وليست إعلانًا عن انخفاض سعر.\n\n%s\n\n💰 السعر الحالي المؤكد: %s جنيه\n🏪 البائع: %s\n🛒 %s",
		string(runes), price, seller, affiliateURL)
	if evidence.LimitedStock > 0 {
		caption += fmt.Sprintf("\n📦 المتبقي: %d", evidence.LimitedStock)
	}
	if len([]rune(caption)) > publishing.PhotoCaptionLimit {
		fail(errors.New("test caption exceeds Telegram limit"))
	}
	tx, err := store.DB.Begin(ctx)
	if err != nil {
		fail(errors.New("test audit unavailable"))
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, *listingID*1000000+*channelID)
	if err != nil {
		fail(errors.New("test lock unavailable"))
	}
	var previous int
	err = tx.QueryRow(ctx, `SELECT count(*) FROM core_auditevent WHERE action='go_test_photo'
AND subject_id=$1 AND details->>'listing_id'=$2 AND created_at>=now()-interval '24 hours'`,
		strconv.FormatInt(*channelID, 10), strconv.FormatInt(*listingID, 10)).Scan(&previous)
	if err != nil || previous > 0 {
		fail(errors.New("test photo already attempted in the past 24 hours"))
	}
	var auditID int64
	err = tx.QueryRow(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
VALUES(NULL,'go_test_photo','channel',$1,jsonb_build_object('listing_id',$2::text,
'price',$3::text,'seller',$4::text,'status','attempting','screenshot',$5::text),now()) RETURNING id`,
		strconv.FormatInt(*channelID, 10), strconv.FormatInt(*listingID, 10), price, seller, evidence.Path).Scan(&auditID)
	if err != nil || tx.Commit(ctx) != nil {
		fail(errors.New("test audit unavailable"))
	}
	messageID, sendErr := sender.SendPhoto(ctx, destination, evidence.Path, caption)
	status := "sent"
	if sendErr != nil {
		status = "failed"
		var delivery *publishing.DeliveryError
		if errors.As(sendErr, &delivery) && delivery.Uncertain {
			status = "uncertain"
		}
	}
	_, err = store.DB.Exec(context.Background(), `UPDATE core_auditevent SET details=details ||
jsonb_build_object('status',$2::text,'message_id',$3::bigint) WHERE id=$1`, auditID, status, messageID)
	if err != nil {
		fail(errors.New("test delivery result could not be recorded; review Telegram before retry"))
	}
	if sendErr != nil {
		fail(fmt.Errorf("Telegram photo %s: %w", status, sendErr))
	}
	fmt.Printf("test photo sent message_id=%d\n", messageID)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
