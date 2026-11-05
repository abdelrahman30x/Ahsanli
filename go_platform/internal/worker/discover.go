package worker

import (
	"context"
	"errors"
	"fmt"
	"time"

	"ahsanly/go_platform/internal/fetch"
	"ahsanly/go_platform/internal/sources"
	"github.com/jackc/pgx/v5"
)

type DiscoveryJob struct {
	ID, SeedID, RetailerID int64
	URL, Cursor            string
	MaxPerHour, RetryCount int
}

func (r *Runner) claimDiscovery(ctx context.Context) (*DiscoveryJob, error) {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	j := &DiscoveryJob{}
	err = tx.QueryRow(ctx, `SELECT j.id,s.id,r.id,s.collection_url,s.pagination_cursor,r.max_requests_per_hour,j.retry_count
 FROM core_fetchjob j JOIN core_categoryseed s ON s.id=j.seed_id JOIN core_retailer r ON r.id=s.retailer_id
 WHERE j.kind='discover' AND (j.status='due' AND j.due_at<=now() OR j.status='leased' AND j.lease_until<=now())
 AND s.enabled AND r.code='amazon_eg' AND r.enabled AND r.offer_source='server_http'
 AND (r.health<>'blocked' OR r.last_failure_at<=now()-interval '6 hours')
 AND (SELECT count(*) FROM core_fetchattempt a JOIN core_fetchjob fj ON fj.id=a.job_id
 LEFT JOIN core_listing ll ON ll.id=fj.listing_id LEFT JOIN core_categoryseed ss ON ss.id=fj.seed_id
 WHERE a.started_at>now()-interval '1 hour' AND COALESCE(ll.retailer_id,ss.retailer_id)=r.id)<r.max_requests_per_hour
 ORDER BY j.priority,j.due_at,j.id LIMIT 1 FOR UPDATE OF j SKIP LOCKED`).Scan(&j.ID, &j.SeedID, &j.RetailerID, &j.URL, &j.Cursor, &j.MaxPerHour, &j.RetryCount)
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

// RunDiscoverOnce processes at most one explicitly enabled Amazon category
// page. New identities start paused and never generate prices or deal posts.
func (r *Runner) RunDiscoverOnce(ctx context.Context) (bool, error) {
	j, err := r.claimDiscovery(ctx)
	if err != nil || j == nil {
		return false, err
	}
	client := r.Clients["amazon_eg"]
	if client == nil {
		interval := time.Hour / time.Duration(max(j.MaxPerHour, 1))
		client = fetch.NewClient(fetch.Policy{MaxPerHour: j.MaxPerHour, MinInterval: interval, BlockLimit: 3, BlockCooldown: time.Hour})
		r.Clients["amazon_eg"] = client
	}
	target := j.Cursor
	if target == "" {
		target = j.URL
	}
	started := time.Now()
	body, err := client.GetHTML(ctx, target, "amazon.eg")
	if err != nil {
		return true, r.recordDiscoveryFailure(ctx, j, started, err)
	}
	page, err := sources.ParseAmazonCategory(body, target)
	if err != nil {
		return true, r.recordDiscoveryFailure(ctx, j, started, err)
	}
	return true, r.recordDiscoveryPage(ctx, j, target, started, len(body), page)
}

func (r *Runner) recordDiscoveryPage(ctx context.Context, j *DiscoveryJob, target string, started time.Time, size int, page sources.ListingPage) error {
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var cursor string
	var enabled bool
	err = tx.QueryRow(ctx, `SELECT pagination_cursor,enabled FROM core_categoryseed WHERE id=$1 FOR UPDATE`, j.SeedID).Scan(&cursor, &enabled)
	if err != nil {
		return err
	}
	if !enabled || (cursor != "" && cursor != target) || (cursor == "" && j.URL != target) {
		return errors.New("category cursor changed during fetch")
	}
	for _, p := range page.Listings {
		image := ""
		if validAmazonImage(p.ImageURL) {
			image = p.ImageURL
		}
		_, err = tx.Exec(ctx, `INSERT INTO core_listing(retailer_id,retailer_product_id,original_url,title,extracted_attributes,tracking_state,cadence,created_at)
  VALUES($1,$2,$3,$4,jsonb_build_object('image_url',$5::text),'paused','cold',now())
  ON CONFLICT(retailer_id,retailer_product_id) DO NOTHING`, j.RetailerID, p.ProductID, p.URL, p.Title, image)
		if err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE core_categoryseed SET pagination_cursor=$2,last_success_at=now() WHERE id=$1`, j.SeedID, page.NextURL)
	if err != nil {
		return err
	}
	status, lastResult, due := "due", "page", time.Now().Add(time.Hour/time.Duration(max(j.MaxPerHour, 1)))
	if page.NextURL == "" {
		status = "paused"
		lastResult = "completed"
	}
	_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status=$2,last_result=$3,last_error_class='',lease_until=NULL,retry_count=0,due_at=$4 WHERE id=$1`, j.ID, status, lastResult, due)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_fetchattempt(job_id,started_at,finished_at,result,error_class,response_status,response_bytes,elapsed_ms,metadata)
 VALUES($1,$2,now(),'success','',200,$3,$4,jsonb_build_object('discovered',$5::int))`, j.ID, started, size, time.Since(started).Milliseconds(), len(page.Listings))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE core_retailer SET health='healthy',last_success_at=now(),last_error_class='' WHERE id=$1`, j.RetailerID)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Runner) recordDiscoveryFailure(ctx context.Context, j *DiscoveryJob, started time.Time, reason error) error {
	class := errorClass(reason)
	tx, err := r.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `UPDATE core_fetchjob SET status='due',lease_until=NULL,retry_count=retry_count+1,
 due_at=now()+($2::bigint*interval '1 second'),last_result='failure',last_error_class=$3 WHERE id=$1`, j.ID, int64(backoff(j.RetryCount+1).Seconds()), class)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO core_fetchattempt(job_id,started_at,finished_at,result,error_class,elapsed_ms,metadata)
 VALUES($1,$2,now(),'failure',$3,$4,'{}'::jsonb)`, j.ID, started, class, time.Since(started).Milliseconds())
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE core_retailer SET health=$2,last_failure_at=now(),last_error_class=$3 WHERE id=$1`, j.RetailerID, map[bool]string{true: "blocked", false: "degraded"}[class == "blocked"], class)
	if err != nil {
		return err
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit discovery failure: %w", err)
	}
	return nil
}
