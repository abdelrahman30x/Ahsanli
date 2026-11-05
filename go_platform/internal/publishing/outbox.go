package publishing

import (
	"context"
	"errors"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Outbox struct {
	DB                *pgxpool.Pool
	CaptureDir        string
	Enabled           bool
	AllowLiveChannels bool
}

func evidenceFile(root, path string) error {
	if root == "" || path == "" {
		return errors.New("missing evidence location")
	}
	dir, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	file, err := filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(dir, file)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return errors.New("evidence path outside capture directory")
	}
	info, err := os.Stat(file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() == 0 {
		return errors.New("evidence image missing or empty")
	}
	if info.Size() > 10*1024*1024 {
		return errors.New("evidence PNG exceeds Telegram photo size")
	}
	photo, err := os.Open(file)
	if err != nil {
		return err
	}
	defer photo.Close()
	config, err := png.DecodeConfig(photo)
	if err != nil || config.Width < 1 || config.Height < 1 {
		return errors.New("evidence is not a valid PNG screenshot")
	}
	return nil
}

// PrepareOne creates one idempotent publication per eligible destination.
// The candidate and route checks are repeated at send time.
func (o *Outbox) PrepareOne(ctx context.Context) (bool, error) {
	if !o.Enabled {
		return false, errors.New("Go publisher disabled")
	}
	tx, err := o.DB.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var candidateID, observationID, listingID, retailerID, countryID int64
	var evidencePath string
	err = tx.QueryRow(ctx, `SELECT d.id,obs.id,l.id,r.id,c.id,d.evidence_path
 FROM core_dealcandidate d JOIN core_offerobservation obs ON obs.id=d.winning_observation_id
 JOIN core_listing l ON l.id=obs.listing_id JOIN core_retailer r ON r.id=l.retailer_id
 JOIN core_country c ON c.id=r.country_id JOIN core_dealrule rule ON rule.country_id=c.id
 JOIN core_currentoffer co ON co.listing_id=l.id AND co.observation_id=obs.id
 WHERE d.state='approved' AND d.reason='historical' AND d.historical_baseline IS NOT NULL
 AND d.historical_baseline>obs.price AND (d.historical_baseline-obs.price)*100>=d.historical_baseline*rule.historical_percent
 AND obs.imported=false AND obs.stock_status='in_stock' AND obs.observed_at>=now()-(rule.freshness_hours*interval '1 hour')
 AND d.evidence_path<>'' AND d.evidence_price=obs.price AND d.evidence_seller=obs.seller
 AND r.code='amazon_eg' AND r.enabled AND r.publish_enabled AND l.extracted_attributes->>'go_owned'='true'
 AND c.enabled AND rule.enabled
 AND d.rule_version=rule.version
 ORDER BY d.created_at,d.id LIMIT 1 FOR UPDATE OF d SKIP LOCKED`).Scan(&candidateID, &observationID, &listingID, &retailerID, &countryID, &evidencePath)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err = evidenceFile(o.CaptureDir, evidencePath); err != nil {
		_, updateErr := tx.Exec(ctx, `UPDATE core_dealcandidate SET state='held' WHERE id=$1`, candidateID)
		if updateErr != nil {
			return true, updateErr
		}
		return true, tx.Commit(ctx)
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT ch.id FROM core_publicationroute pr
 JOIN core_channel ch ON ch.id=pr.channel_id
 JOIN core_affiliatelink a ON a.channel_id=ch.id AND a.listing_id=$1 AND a.verified_at IS NOT NULL
 WHERE pr.country_id=$2 AND pr.enabled AND (pr.retailer_id IS NULL OR pr.retailer_id=$3)
 AND ch.enabled AND ch.country_id=$2 AND ch.platform='telegram'
 AND (ch.private_test OR $4::boolean)`, listingID, countryID, retailerID, o.AllowLiveChannels)
	if err != nil {
		return true, err
	}
	var channels []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return true, err
		}
		channels = append(channels, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return true, err
	}
	if len(channels) == 0 {
		_, err = tx.Exec(ctx, `UPDATE core_dealcandidate SET state='held' WHERE id=$1`, candidateID)
		if err != nil {
			return true, err
		}
		return true, tx.Commit(ctx)
	}
	for _, channelID := range channels {
		key := fmt.Sprintf("offer:%d:%d", channelID, observationID)
		_, err = tx.Exec(ctx, `INSERT INTO core_publication(candidate_id,channel_id,canonical_product_id,publication_key,status,screenshot_path,attempt_count,last_error,provider_message_id,created_at)
   SELECT d.id,$2,d.canonical_product_id,$3,'pending',$4,0,'','',now() FROM core_dealcandidate d WHERE d.id=$1
   ON CONFLICT(publication_key) DO NOTHING`, candidateID, channelID, key, evidencePath)
		if err != nil {
			return true, err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE core_dealcandidate SET state='queued' WHERE id=$1`, candidateID)
	if err != nil {
		return true, err
	}
	return true, tx.Commit(ctx)
}

type pendingPhoto struct {
	ID                                                                                      int64
	Screenshot, TokenEnv, Destination, Store, Title, Price, Currency, Seller, Link, Percent string
	Discount                                                                                *string
	Stock                                                                                   *int
}

func (o *Outbox) hold(ctx context.Context, tx pgx.Tx, id int64, why string) error {
	_, err := tx.Exec(ctx, `UPDATE core_publication SET status='held',last_error=$2,delivery_lease_until=NULL WHERE id=$1`, id, why)
	return err
}

// RecoverUncertain never resends an expired in-flight upload automatically.
func (o *Outbox) RecoverUncertain(ctx context.Context) (int64, error) {
	if !o.Enabled {
		return 0, errors.New("Go publisher disabled")
	}
	tag, err := o.DB.Exec(ctx, `UPDATE core_publication SET status='uncertain',last_error='Delivery lease expired; manual review required',delivery_lease_until=NULL
 WHERE status='sending' AND (delivery_lease_until IS NULL OR delivery_lease_until<now())`)
	return tag.RowsAffected(), err
}

func (o *Outbox) claimPending(ctx context.Context) (*pendingPhoto, error) {
	tx, err := o.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	p := &pendingPhoto{}
	err = tx.QueryRow(ctx, `SELECT p.id FROM core_publication p JOIN core_channel ch ON ch.id=p.channel_id
 WHERE p.status='pending' AND (p.next_attempt_at IS NULL OR p.next_attempt_at<=now())
 AND (ch.private_test OR $1::boolean)
 ORDER BY p.id LIMIT 1 FOR UPDATE OF p SKIP LOCKED`, o.AllowLiveChannels).Scan(&p.ID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	err = tx.QueryRow(ctx, `SELECT p.screenshot_path,ch.bot_token_env,COALESCE(NULLIF(ch.destination_id,''),ch.telegram_chat_id,''),
 COALESCE(NULLIF(r.display_name_ar,''),r.code),COALESCE(NULLIF(l.title,''),l.retailer_product_id),
 obs.price::text,c.currency_label_ar,obs.seller,a.tracking_url,d.percent::text,d.evidence_discount::text,d.limited_stock
 FROM core_publication p JOIN core_dealcandidate d ON d.id=p.candidate_id
 JOIN core_offerobservation obs ON obs.id=d.winning_observation_id
 JOIN core_listing l ON l.id=obs.listing_id JOIN core_retailer r ON r.id=l.retailer_id
 JOIN core_country c ON c.id=r.country_id JOIN core_dealrule rule ON rule.country_id=c.id
 JOIN core_channel ch ON ch.id=p.channel_id
 JOIN core_affiliatelink a ON a.listing_id=l.id AND a.channel_id=ch.id AND a.verified_at IS NOT NULL
 JOIN core_currentoffer co ON co.listing_id=l.id AND co.observation_id=obs.id
 WHERE p.id=$1 AND p.status='pending' AND d.state='queued' AND d.reason='historical'
 AND d.historical_baseline>obs.price AND (d.historical_baseline-obs.price)*100>=d.historical_baseline*rule.historical_percent
 AND obs.imported=false AND obs.stock_status='in_stock' AND obs.observed_at>=now()-(rule.freshness_hours*interval '1 hour')
 AND d.evidence_path=p.screenshot_path AND d.evidence_price=obs.price AND d.evidence_seller=obs.seller
 AND r.code='amazon_eg' AND r.enabled AND r.publish_enabled AND l.extracted_attributes->>'go_owned'='true'
 AND c.enabled AND rule.enabled
 AND d.rule_version=rule.version
 AND ch.enabled AND ch.country_id=c.id AND ch.platform='telegram'
 AND (ch.private_test OR $2::boolean)
 AND EXISTS(SELECT 1 FROM core_publicationroute pr WHERE pr.channel_id=ch.id AND pr.country_id=c.id
 AND pr.enabled AND (pr.retailer_id IS NULL OR pr.retailer_id=r.id))`, p.ID, o.AllowLiveChannels).Scan(
		&p.Screenshot, &p.TokenEnv, &p.Destination, &p.Store, &p.Title, &p.Price, &p.Currency, &p.Seller, &p.Link, &p.Percent, &p.Discount, &p.Stock)
	if errors.Is(err, pgx.ErrNoRows) {
		if err = o.hold(ctx, tx, p.ID, "Offer, route, or evidence is no longer valid"); err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	if err != nil {
		return nil, err
	}
	if err = evidenceFile(o.CaptureDir, p.Screenshot); err != nil {
		if err = o.hold(ctx, tx, p.ID, "Screenshot missing or outside evidence directory"); err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	if os.Getenv(p.TokenEnv) == "" || p.TokenEnv == "" || p.Destination == "" {
		if err = o.hold(ctx, tx, p.ID, "Telegram destination or token unavailable"); err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `UPDATE core_publication SET status='sending',attempt_count=attempt_count+1,
 delivery_lease_until=now()+interval '2 minutes',next_attempt_at=NULL WHERE id=$1`, p.ID)
	if err != nil {
		return nil, err
	}
	return p, tx.Commit(ctx)
}

func formatPrice(raw string) string {
	parts := strings.SplitN(raw, ".", 2)
	whole := parts[0]
	if len(whole) > 3 {
		var b strings.Builder
		for i, r := range whole {
			if i > 0 && (len(whole)-i)%3 == 0 {
				b.WriteByte(',')
			}
			b.WriteRune(r)
		}
		whole = b.String()
	}
	if len(parts) > 1 {
		return whole + "." + parts[1]
	}
	return whole
}

// SendOne performs a single verified photo upload. A transport timeout becomes
// uncertain and is never retried by the worker.
func (o *Outbox) SendOne(ctx context.Context) (bool, error) {
	if !o.Enabled {
		return false, errors.New("Go publisher disabled")
	}
	p, err := o.claimPending(ctx)
	if err != nil || p == nil {
		return false, err
	}
	discount := ""
	if p.Discount != nil {
		discount = *p.Discount
	}
	stock := 0
	if p.Stock != nil {
		stock = *p.Stock
	}
	caption, err := FormatCaption(CaptionInput{StoreName: p.Store, Title: p.Title, Price: formatPrice(p.Price), CurrencyLabel: p.Currency, DropPercent: p.Percent, DisplayedDiscount: discount, LimitedStock: stock, Seller: p.Seller, ProductURL: p.Link})
	if err != nil {
		_, updateErr := o.DB.Exec(ctx, `UPDATE core_publication SET status='held',last_error='Caption validation failed',delivery_lease_until=NULL WHERE id=$1 AND status='sending'`, p.ID)
		if updateErr != nil {
			return true, updateErr
		}
		return true, err
	}
	sender, err := NewTelegramSender(os.Getenv(p.TokenEnv))
	if err != nil {
		return true, o.finishError(ctx, p.ID, &DeliveryError{Description: "Telegram token invalid"})
	}
	messageID, err := sender.SendPhoto(ctx, p.Destination, p.Screenshot, caption)
	if err != nil {
		return true, o.finishError(ctx, p.ID, err)
	}
	tag, err := o.DB.Exec(ctx, `UPDATE core_publication SET status='sent',telegram_message_id=$2,provider_message_id=$3,
 sent_at=now(),last_error='',delivery_lease_until=NULL WHERE id=$1 AND status='sending' AND delivery_lease_until>now()`, p.ID, messageID, strconv.FormatInt(messageID, 10))
	if err != nil {
		return true, err
	}
	if tag.RowsAffected() != 1 {
		return true, errors.New("Telegram sent but publication lease expired; review manually")
	}
	return true, nil
}

func (o *Outbox) finishError(ctx context.Context, id int64, reason error) error {
	state, why, delay := "held", "Telegram rejected photo", false
	var delivery *DeliveryError
	if errors.As(reason, &delivery) {
		if delivery.Uncertain {
			state = "uncertain"
			why = "Telegram delivery uncertain; manual review required"
		}
		if delivery.Retryable {
			state = "pending"
			why = "Telegram rate limit; retry scheduled"
			delay = true
		}
	}
	_, err := o.DB.Exec(ctx, `UPDATE core_publication SET status=CASE WHEN $2='pending' AND attempt_count>=3 THEN 'held' ELSE $2 END,
 last_error=$3,delivery_lease_until=NULL,next_attempt_at=CASE WHEN $4::boolean THEN now()+interval '15 minutes' ELSE NULL END
	WHERE id=$1 AND status='sending'`, id, state, why, delay)
	return err
}
