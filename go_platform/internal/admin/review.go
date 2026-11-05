package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

func validEditorialNote(value string) bool {
	return utf8.RuneCountInString(value) <= 600
}

func (s *Service) reviewCandidate(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid candidate"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 8192)
	var input struct {
		Decision        string  `json:"decision"`
		Featured        *bool   `json:"featured"`
		EditorialNoteAR *string `json:"editorial_note_ar"`
		EditorialNoteEN *string `json:"editorial_note_en"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF ||
		(input.Decision != "" && input.Decision != "approved" && input.Decision != "rejected") {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid editorial review"})
		return
	}
	if input.EditorialNoteAR != nil {
		value := strings.TrimSpace(*input.EditorialNoteAR)
		if !validEditorialNote(value) {
			adminJSON(w, http.StatusBadRequest, map[string]string{"error": "Arabic editorial note is too long"})
			return
		}
		input.EditorialNoteAR = &value
	}
	if input.EditorialNoteEN != nil {
		value := strings.TrimSpace(*input.EditorialNoteEN)
		if !validEditorialNote(value) {
			adminJSON(w, http.StatusBadRequest, map[string]string{"error": "English editorial note is too long"})
			return
		}
		input.EditorialNoteEN = &value
	}

	ctx, cancel := requestContext(r)
	defer cancel()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "editorial review unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var state string
	var eligible bool
	err = tx.QueryRow(ctx, `SELECT d.state, EXISTS(
 SELECT 1 FROM core_dealcandidate candidate
 JOIN core_offerobservation o ON o.id=candidate.winning_observation_id
 JOIN core_currentoffer co ON co.listing_id=o.listing_id AND co.observation_id=o.id
 JOIN core_listing l ON l.id=o.listing_id JOIN core_retailer r ON r.id=l.retailer_id
 JOIN core_country c ON c.id=r.country_id JOIN core_dealrule rule ON rule.country_id=c.id
 WHERE candidate.id=d.id AND candidate.historical_baseline>o.price
 AND (candidate.historical_baseline-o.price)*100>=candidate.historical_baseline*rule.historical_percent
 AND candidate.evidence_path<>'' AND candidate.evidence_price=o.price AND candidate.evidence_seller=o.seller
 AND NOT o.imported AND o.stock_status='in_stock'
 AND o.observed_at>=now()-(rule.freshness_hours*interval '1 hour')
 AND candidate.rule_version=rule.version AND rule.enabled AND c.enabled
 AND r.enabled AND r.publish_enabled AND l.extracted_attributes->>'go_owned'='true')
 FROM core_dealcandidate d WHERE d.id=$1 FOR UPDATE OF d`, id).Scan(&state, &eligible)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "candidate not found"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "editorial review unavailable"})
		return
	}
	if input.Decision != "" && state != "review" {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "candidate is not awaiting editorial review"})
		return
	}
	if input.Decision == "" && state != "approved" && state != "queued" {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "only an approved deal can be edited"})
		return
	}
	if input.Decision == "approved" && !eligible {
		adminJSON(w, http.StatusConflict, map[string]string{"error": "candidate no longer has a fresh matching verified offer"})
		return
	}
	_, err = tx.Exec(ctx, `UPDATE core_dealcandidate SET
 state=CASE WHEN $2='' THEN state ELSE $2 END,
 reviewer_id=CASE WHEN $2='' THEN reviewer_id ELSE $3 END,
 reviewed_at=CASE WHEN $2='' THEN reviewed_at ELSE now() END,
 featured=CASE WHEN $2='rejected' THEN false ELSE COALESCE($4,featured) END,
 editorial_note_ar=COALESCE($5::text,editorial_note_ar),
 editorial_note_en=COALESCE($6::text,editorial_note_en)
 WHERE id=$1`, id, input.Decision, staff.UserID, input.Featured, input.EditorialNoteAR, input.EditorialNoteEN)
	if err == nil {
		_, err = tx.Exec(ctx, `INSERT INTO core_auditevent(actor_id,action,subject_type,subject_id,details,created_at)
 VALUES($1,'go_review_deal_candidate','deal_candidate',$2,
 jsonb_build_object('decision',$3::text,'featured',$4::boolean),now())`,
			staff.UserID, strconv.FormatInt(id, 10), input.Decision, input.Featured)
	}
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "editorial review unavailable"})
		return
	}
	if input.Decision != "" {
		state = input.Decision
	}
	adminJSON(w, http.StatusOK, map[string]any{"id": id, "state": state})
}

func (s *Service) candidateEvidence(w http.ResponseWriter, r *http.Request) {
	if s.requireSession(w, r, false) == nil {
		return
	}
	id, ok := parseID(r)
	if !ok {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid candidate"})
		return
	}
	ctx, cancel := requestContext(r)
	defer cancel()
	var path string
	if err := s.DB.QueryRow(ctx, `SELECT evidence_path FROM core_dealcandidate WHERE id=$1`, id).Scan(&path); err != nil || path == "" {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "evidence not found"})
		return
	}
	root := os.Getenv("AHSANLY_CAPTURE_DIR")
	if root == "" {
		root = "/var/lib/ahsanly/captures"
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "evidence not found"})
		return
	}
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "evidence not found"})
		return
	}
	rel, err := filepath.Rel(resolvedRoot, resolvedPath)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "evidence not found"})
		return
	}
	info, err := os.Stat(resolvedPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		adminJSON(w, http.StatusNotFound, map[string]string{"error": "evidence not found"})
		return
	}
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeFile(w, r, resolvedPath)
}
