package worker

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"ahsanly/go_platform/internal/fetch"
	"ahsanly/go_platform/internal/sources"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Job struct {
	ID, ListingID, RetailerID                   int64
	ProductID, URL, Retailer, Currency, Cadence string
	MaxPerHour                                  int
	RetryCount                                  int
}

type Runner struct {
	DB             *pgxpool.Pool
	Clients        map[string]*fetch.Client
	CandidateState string
}

func New(db *pgxpool.Pool) *Runner {
	return &Runner{DB: db, Clients: map[string]*fetch.Client{}, CandidateState: "shadow"}
}

// Claim only the selected Amazon shadow jobs for now. Noon and Jumia require
// a reliable permitted server-side source before they can enter this queue.
func (r *Runner) claim(ctx context.Context) (*Job, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	j := &Job{}
	err = tx.QueryRow(ctx, `SELECT j.id,l.id,r.id,l.retailer_product_id,l.original_url,r.code,r.currency,l.cadence,r.max_requests_per_hour,j.retry_count
FROM core_fetchjob j JOIN core_listing l ON l.id=j.listing_id JOIN core_retailer r ON r.id=l.retailer_id
WHERE j.kind='offer' AND (j.status='due' AND j.due_at<=now() OR j.status='leased' AND j.lease_until<=now())
AND r.code='amazon_eg' AND r.enabled AND r.offer_source='server_http' AND l.tracking_state='watch'
AND ($1::boolean=false OR r.publish_enabled)
AND ($1::boolean=false OR l.extracted_attributes->>'go_owned'='true')
AND (r.health<>'blocked' OR r.last_failure_at<=now()-interval '6 hours')
AND (SELECT count(*) FROM core_fetchattempt a JOIN core_fetchjob fj ON fj.id=a.job_id JOIN core_listing ll ON ll.id=fj.listing_id
 WHERE ll.retailer_id=r.id AND a.started_at>now()-interval '1 hour')<r.max_requests_per_hour
ORDER BY j.priority,j.due_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`, r.CandidateState == "new").Scan(&j.ID, &j.ListingID, &j.RetailerID, &j.ProductID, &j.URL, &j.Retailer, &j.Currency, &j.Cadence, &j.MaxPerHour, &j.RetryCount)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status='leased',lease_until=now()+interval '3 minutes' WHERE id=$1`, j.ID)
	if err != nil {
		return nil, err
	}
	return j, tx.Commit(ctx)
}

func (r *Runner) RunOnce(ctx context.Context) (bool, error) {
	j, err := r.claim(ctx)
	if err != nil || j == nil {
		return false, err
	}
	client := r.Clients[j.Retailer]
	if client == nil {
		interval := time.Hour / time.Duration(max(j.MaxPerHour, 1))
		client = fetch.NewClient(fetch.Policy{MaxPerHour: j.MaxPerHour, MinInterval: interval, BlockLimit: 3, BlockCooldown: time.Hour})
		r.Clients[j.Retailer] = client
	}
	started := time.Now()
	offer, fetchErr := sources.FetchOffer(ctx, client, j.Retailer, j.ProductID, j.URL)
	if fetchErr != nil {
		if errors.Is(fetchErr, sources.ErrUnavailable) {
			return true, r.recordUnavailable(ctx, j, started)
		}
		return true, r.recordFailure(ctx, j, started, fetchErr)
	}
	if offer.Currency != j.Currency || offer.PriceCents <= 0 || strings.TrimSpace(offer.Seller) == "" {
		return true, r.recordFailure(ctx, j, started, errors.New("invalid offer"))
	}
	if err = r.recordOffer(ctx, j, offer, started); err != nil {
		_ = r.recordFailure(ctx, j, started, err)
		return true, err
	}
	return true, nil
}

func (r *Runner) recordUnavailable(ctx context.Context, j *Job, started time.Time) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status='due',lease_until=NULL,retry_count=0,
due_at=now()+($2::bigint * interval '1 second'),last_result='unavailable',last_error_class='' WHERE id=$1`, j.ID, int64(cadenceDuration(j.Cadence).Seconds()))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_fetchattempt(job_id,started_at,finished_at,result,error_class,elapsed_ms,metadata)
VALUES ($1,$2,now(),'unavailable','',$3,'{}'::jsonb)`, j.ID, started, time.Since(started).Milliseconds())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func errorClass(err error) string {
	switch {
	case errors.Is(err, fetch.ErrBlocked):
		return "blocked"
	case errors.Is(err, fetch.ErrPaused):
		return "blocked"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, sources.ErrUnavailable):
		return "unavailable"
	case errors.Is(err, sources.ErrSeller):
		return "missing_seller"
	case errors.Is(err, sources.ErrPrice):
		return "parser_error"
	default:
		return "network_or_parser_error"
	}
}

func backoff(retries int) time.Duration {
	if retries < 1 {
		retries = 1
	}
	if retries > 6 {
		retries = 6
	}
	d := time.Duration(5<<retries) * time.Minute
	if d > 6*time.Hour {
		return 6 * time.Hour
	}
	return d
}

func (r *Runner) recordFailure(ctx context.Context, j *Job, started time.Time, reason error) error {
	class := errorClass(reason)
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status='due',lease_until=NULL,retry_count=retry_count+1,
 due_at=now()+($2::bigint * interval '1 second'),last_result='failure',last_error_class=$3 WHERE id=$1`, j.ID, int64(backoff(j.RetryCount+1).Seconds()), class)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE core_retailer SET health=$2,last_failure_at=now(),last_error_class=$3 WHERE id=$1`, j.RetailerID, map[bool]string{true: "blocked", false: "degraded"}[class == "blocked"], class)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_fetchattempt(job_id,started_at,finished_at,result,error_class,elapsed_ms,metadata)
VALUES ($1,$2,now(),'failure',$3,$4,'{}'::jsonb)`, j.ID, started, class, time.Since(started).Milliseconds())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func cadenceDuration(c string) time.Duration {
	switch c {
	case "hot":
		return time.Hour
	case "cold":
		return 24 * time.Hour
	default:
		return 6 * time.Hour
	}
}

// Qualifies uses cents and percentage basis points, so no float rounding can
// turn a 9.99% change into a 10% deal.
func Qualifies(current, previous, thresholdBasisPoints int64) bool {
	return current > 0 && previous > current && thresholdBasisPoints > 0 &&
		(previous-current)*10_000 >= previous*thresholdBasisPoints
}

func percentBasisPoints(current, previous int64) int64 {
	if previous <= 0 || current >= previous {
		return 0
	}
	return ((previous-current)*10_000 + previous/2) / previous
}

func validAmazonImage(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && u.Scheme == "https" && u.Hostname() == "m.media-amazon.com" && u.User == nil && len(raw) < 1000
}

func (r *Runner) recordOffer(ctx context.Context, j *Job, o sources.Offer, started time.Time) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var tracking, existingCurrency string
	var retailerEnabled, retailerPublishEnabled bool
	var goOwned bool
	err = tx.QueryRow(ctx, `SELECT l.tracking_state,r.currency,r.enabled,r.publish_enabled,
 COALESCE(l.extracted_attributes->>'go_owned'='true',false)
 FROM core_listing l JOIN core_retailer r ON r.id=l.retailer_id WHERE l.id=$1 FOR UPDATE OF l`, j.ListingID).Scan(&tracking, &existingCurrency, &retailerEnabled, &retailerPublishEnabled, &goOwned)
	if err != nil {
		return err
	}
	if tracking != "watch" || !retailerEnabled || existingCurrency != o.Currency {
		return errors.New("listing no longer eligible")
	}
	if r.CandidateState == "new" && (!retailerPublishEnabled || !goOwned) {
		return errors.New("retailer publication or Go listing ownership is disabled")
	}
	if r.CandidateState != "new" && r.CandidateState != "shadow" {
		return errors.New("invalid candidate state configuration")
	}
	var previous string
	err = tx.QueryRow(ctx, `SELECT price::text FROM core_offerobservation WHERE listing_id=$1 AND imported=false AND stock_status='in_stock' ORDER BY observed_at DESC,id DESC LIMIT 1`, j.ListingID).Scan(&previous)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	var obsID int64
	err = tx.QueryRow(ctx, `INSERT INTO core_offerobservation(listing_id,price,currency,seller,stock_status,source,observed_at,imported,legacy_source)
VALUES($1,$2::numeric,$3,$4,'in_stock','go_http',now(),false,'') RETURNING id`, j.ListingID, o.PriceString(), o.Currency, o.Seller).Scan(&obsID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_currentoffer(listing_id,observation_id,updated_at) VALUES($1,$2,now())
ON CONFLICT(listing_id) DO UPDATE SET observation_id=EXCLUDED.observation_id,updated_at=EXCLUDED.updated_at`, j.ListingID, obsID)
	if err != nil {
		return err
	}
	image := ""
	if validAmazonImage(o.ImageURL) {
		image = o.ImageURL
	}
	_, err = tx.Exec(ctx, `UPDATE core_listing SET title=CASE WHEN $2<>'' THEN $2 ELSE title END,
extracted_attributes=CASE WHEN $3<>'' AND COALESCE(extracted_attributes->>'image_url','')='' THEN jsonb_set(extracted_attributes,'{image_url}',to_jsonb($3::text)) ELSE extracted_attributes END,
last_checked_at=now() WHERE id=$1`, j.ListingID, o.Title, image)
	if err != nil {
		return err
	}
	if previous != "" {
		prevCents, parseErr := sources.ParsePrice(previous)
		if parseErr == nil {
			var threshold string
			var version int
			err = tx.QueryRow(ctx, `SELECT rule.historical_percent::text,rule.version FROM core_dealrule rule
JOIN core_retailer r ON r.country_id=rule.country_id WHERE r.id=$1 AND rule.enabled=true
ORDER BY rule.id LIMIT 1`, j.RetailerID).Scan(&threshold, &version)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if err == nil {
				thresholdBP, parseErr := sources.ParsePrice(threshold)
				if parseErr == nil && Qualifies(o.PriceCents, prevCents, thresholdBP) {
					pct := percentBasisPoints(o.PriceCents, prevCents)
					_, err = tx.Exec(ctx, `INSERT INTO core_dealcandidate(winning_observation_id,compared_observations,historical_baseline,rule_version,reason,percent,state,evidence_path,evidence_seller,capture_retries,created_at)
VALUES($1,'[]'::jsonb,$2::numeric,$3,'historical',$4::numeric,$5,'','',0,now())
ON CONFLICT(winning_observation_id,reason,rule_version) DO NOTHING`, obsID, previous, version, fmt.Sprintf("%d.%02d", pct/100, pct%100), r.CandidateState)
					if err != nil {
						return err
					}
				}
			}
		}
	}
	_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status='due',lease_until=NULL,retry_count=0,
due_at=now()+($2::bigint * interval '1 second'),last_result='success',last_error_class='' WHERE id=$1`, j.ID, int64(cadenceDuration(j.Cadence).Seconds()))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE core_retailer SET health='healthy',last_success_at=now(),last_error_class='' WHERE id=$1`, j.RetailerID)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_fetchattempt(job_id,started_at,finished_at,result,error_class,response_status,elapsed_ms,metadata)
VALUES($1,$2,now(),'success','',200,$3,'{}'::jsonb)`, j.ID, started, time.Since(started).Milliseconds())
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
