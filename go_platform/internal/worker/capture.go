package worker

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"ahsanly/go_platform/internal/capture"
	"ahsanly/go_platform/internal/sources"
	"github.com/jackc/pgx/v5"
)

const browserAdvisoryLock int64 = 8274131

type captureJob struct {
	ID, ObservationID, ListingID            int64
	URL, Price, Seller, Baseline, Threshold string
	FreshnessHours                          int
}

func (r *Runner) claimCapture(ctx context.Context, conn *pgx.Conn) (*captureJob, error) {
	tx, err := conn.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	j := &captureJob{}
	err = tx.QueryRow(ctx, `SELECT d.id,o.id,l.id,l.original_url,o.price::text,o.seller,
 d.historical_baseline::text,rule.historical_percent::text,rule.freshness_hours
 FROM core_dealcandidate d JOIN core_offerobservation o ON o.id=d.winning_observation_id
 JOIN core_listing l ON l.id=o.listing_id JOIN core_retailer r ON r.id=l.retailer_id
 JOIN core_country c ON c.id=r.country_id JOIN core_dealrule rule ON rule.country_id=c.id
 JOIN core_currentoffer co ON co.listing_id=l.id AND co.observation_id=o.id
 WHERE d.reason='historical' AND d.historical_baseline IS NOT NULL AND d.capture_retries<3
 AND (d.state IN ('new','capture_retry') OR (d.state='capturing' AND d.capture_lease_until<now()))
 AND o.imported=false AND o.stock_status='in_stock' AND o.observed_at>=now()-(rule.freshness_hours*interval '1 hour')
 AND r.code='amazon_eg' AND r.enabled AND r.publish_enabled AND l.extracted_attributes->>'go_owned'='true'
 AND c.enabled AND rule.enabled
 AND d.rule_version=rule.version
 ORDER BY d.created_at,d.id LIMIT 1 FOR UPDATE OF d SKIP LOCKED`).Scan(
		&j.ID, &j.ObservationID, &j.ListingID, &j.URL, &j.Price, &j.Seller, &j.Baseline, &j.Threshold, &j.FreshnessHours)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	current, err := sources.ParsePrice(j.Price)
	if err != nil {
		return nil, err
	}
	baseline, err := sources.ParsePrice(j.Baseline)
	if err != nil {
		return nil, err
	}
	threshold, err := sources.ParsePrice(j.Threshold)
	if err != nil {
		return nil, err
	}
	if !Qualifies(current, baseline, threshold) {
		_, err = tx.Exec(ctx, `UPDATE core_dealcandidate SET state='held',capture_lease_until=NULL WHERE id=$1`, j.ID)
		if err != nil {
			return nil, err
		}
		return nil, tx.Commit(ctx)
	}
	_, err = tx.Exec(ctx, `UPDATE core_dealcandidate SET state='capturing',capture_lease_until=now()+interval '2 minutes' WHERE id=$1`, j.ID)
	if err != nil {
		return nil, err
	}
	return j, tx.Commit(ctx)
}

// RunCaptureOnce opens one browser after a verified candidate has qualified.
// The global advisory lock limits Chromium to one process on this VPS.
func (r *Runner) RunCaptureOnce(ctx context.Context, browserPath, outputDir string) (bool, error) {
	if browserPath == "" || outputDir == "" {
		return false, errors.New("browser path and capture directory required")
	}
	pooled, err := r.DB.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer pooled.Release()
	conn := pooled.Conn()
	var locked bool
	if err = conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, browserAdvisoryLock).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, nil
	}
	defer func() {
		unlockCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		_ = conn.QueryRow(unlockCtx, `SELECT pg_advisory_unlock($1)`, browserAdvisoryLock).Scan(&unlocked)
	}()
	j, err := r.claimCapture(ctx, conn)
	if err != nil || j == nil {
		return false, err
	}
	cents, err := sources.ParsePrice(j.Price)
	if err != nil {
		return true, r.failCapture(ctx, j.ID, err)
	}
	output := filepath.Join(outputDir, fmt.Sprintf("candidate-%d.png", j.ID))
	evidence, err := capture.CaptureAmazon(ctx, browserPath, j.URL, output, sources.Offer{PriceCents: cents, Currency: "EGP", Seller: j.Seller})
	if err != nil {
		return true, r.failCapture(ctx, j.ID, err)
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return true, err
	}
	defer tx.Rollback(ctx)
	var stillCurrent bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM core_dealcandidate d
 JOIN core_offerobservation o ON o.id=d.winning_observation_id
 JOIN core_currentoffer co ON co.listing_id=o.listing_id AND co.observation_id=o.id
 JOIN core_listing l ON l.id=o.listing_id JOIN core_retailer r ON r.id=l.retailer_id
 JOIN core_dealrule rule ON rule.country_id=r.country_id
 WHERE d.id=$1 AND d.state='capturing' AND d.capture_lease_until>now()
 AND o.observed_at>=now()-(rule.freshness_hours*interval '1 hour')
 AND o.imported=false AND o.stock_status='in_stock'
 AND d.historical_baseline>o.price AND (d.historical_baseline-o.price)*100>=d.historical_baseline*rule.historical_percent
 AND r.enabled AND r.publish_enabled AND l.extracted_attributes->>'go_owned'='true'
 AND rule.enabled AND d.rule_version=rule.version)`, j.ID).Scan(&stillCurrent)
	if err != nil {
		return true, err
	}
	if !stillCurrent {
		_, err = tx.Exec(ctx, `UPDATE core_dealcandidate SET state='held',capture_lease_until=NULL WHERE id=$1`, j.ID)
		if err != nil {
			return true, err
		}
		return true, tx.Commit(ctx)
	}
	var discount any
	if evidence.DisplayedDiscount != "" {
		discount = evidence.DisplayedDiscount
	}
	var stock any
	if evidence.LimitedStock > 0 {
		stock = evidence.LimitedStock
	}
	tag, err := tx.Exec(ctx, `UPDATE core_dealcandidate SET state='captured',capture_lease_until=NULL,
 evidence_path=$2,evidence_price=$3::numeric,evidence_seller=$4,evidence_discount=$5::numeric,limited_stock=$6
 WHERE id=$1 AND state='capturing' AND capture_lease_until>now()`, j.ID, output, j.Price, evidence.Seller, discount, stock)
	if err != nil {
		return true, err
	}
	if tag.RowsAffected() != 1 {
		return true, errors.New("capture lease expired")
	}
	return true, tx.Commit(ctx)
}

func (r *Runner) failCapture(ctx context.Context, id int64, reason error) error {
	var state string
	err := r.DB.QueryRow(ctx, `UPDATE core_dealcandidate SET capture_retries=capture_retries+1,
 state=CASE WHEN capture_retries+1>=3 THEN 'held' ELSE 'capture_retry' END,
 capture_lease_until=NULL WHERE id=$1 AND state='capturing'
 RETURNING state`, id).Scan(&state)
	if err != nil {
		return err
	}
	return fmt.Errorf("capture %d %s: %w", id, strings.ToLower(state), reason)
}
