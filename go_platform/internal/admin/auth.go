package admin

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const sessionCookie = "__Host-ahsanly_admin"

type Service struct {
	DB      *pgxpool.Pool
	Origin  string
	Enabled bool
}

type staffSession struct {
	UserID    int64
	Username  string
	CSRF      string
	ExpiresAt time.Time
	TokenHash [sha256.Size]byte
}

func (s *Service) Register(mux *http.ServeMux) {
	if !s.Enabled || !strings.HasPrefix(s.Origin, "https://") {
		return
	}
	mux.HandleFunc("POST /api/v1/admin/session", s.login)
	mux.HandleFunc("GET /api/v1/admin/session", s.session)
	mux.HandleFunc("DELETE /api/v1/admin/session", s.logout)
	mux.HandleFunc("GET /api/v1/admin/overview", s.overview)
	mux.HandleFunc("GET /api/v1/admin/retailers", s.retailers)
	mux.HandleFunc("GET /api/v1/admin/listings", s.listings)
	mux.HandleFunc("PATCH /api/v1/admin/listings/{id}", s.updateListing)
	mux.HandleFunc("GET /api/v1/admin/rules", s.rules)
	mux.HandleFunc("PATCH /api/v1/admin/rules/{id}", s.updateRule)
	mux.HandleFunc("GET /api/v1/admin/channels", s.channels)
	mux.HandleFunc("GET /api/v1/admin/countries", s.countries)
	mux.HandleFunc("POST /api/v1/admin/channels", s.createChannel)
	mux.HandleFunc("PATCH /api/v1/admin/channels/{id}", s.updateChannel)
	mux.HandleFunc("GET /api/v1/admin/routes", s.routes)
	mux.HandleFunc("POST /api/v1/admin/routes", s.createRoute)
	mux.HandleFunc("PATCH /api/v1/admin/routes/{id}", s.updateRoute)
	mux.HandleFunc("GET /api/v1/admin/candidates", s.candidates)
	mux.HandleFunc("PATCH /api/v1/admin/candidates/{id}", s.reviewCandidate)
	mux.HandleFunc("GET /api/v1/admin/candidates/{id}/evidence", s.candidateEvidence)
	mux.HandleFunc("GET /api/v1/admin/matches", s.matches)
	mux.HandleFunc("PATCH /api/v1/admin/matches/{id}", s.reviewMatch)
	mux.HandleFunc("GET /api/v1/admin/publications", s.publications)
}

func adminJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}

func (s *Service) sameOrigin(r *http.Request) bool {
	return r.Header.Get("Origin") == s.Origin && r.Host == strings.TrimPrefix(s.Origin, "https://")
}

func randomHex(size int) (string, error) {
	data := make([]byte, size)
	if _, err := rand.Read(data); err != nil {
		return "", err
	}
	return hex.EncodeToString(data), nil
}

func (s *Service) login(w http.ResponseWriter, r *http.Request) {
	if !s.sameOrigin(r) {
		adminJSON(w, http.StatusForbidden, map[string]string{"error": "origin rejected"})
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 4096)
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		TOTP     string `json:"totp"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF || len(input.Username) > 150 || len(input.Password) > 1024 || !validTOTPInput(input.TOTP) {
		adminJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid login request"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	var userID int64
	var passwordHash, username string
	err := s.DB.QueryRow(ctx, `SELECT id,password,username FROM auth_user WHERE username=$1 AND is_staff AND is_active`, input.Username).Scan(&userID, &passwordHash, &username)
	if err != nil || !verifyDjangoPassword(passwordHash, input.Password) {
		adminJSON(w, http.StatusForbidden, map[string]string{"error": "invalid credentials or code"})
		return
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	defer tx.Rollback(ctx)
	var deviceID, t0, lastT int64
	var secret string
	var step, digits, tolerance, drift int
	err = tx.QueryRow(ctx, `SELECT id,key,step,t0,digits,tolerance,drift,last_t FROM otp_totp_totpdevice
WHERE user_id=$1 AND confirmed ORDER BY id LIMIT 1 FOR UPDATE`, userID).Scan(&deviceID, &secret, &step, &t0, &digits, &tolerance, &drift, &lastT)
	if errors.Is(err, pgx.ErrNoRows) {
		adminJSON(w, http.StatusForbidden, map[string]string{"error": "set up TOTP in the existing admin first"})
		return
	}
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	counter, valid := verifyTOTP(secret, input.TOTP, step, t0, digits, tolerance, drift, lastT, time.Now())
	if !valid {
		adminJSON(w, http.StatusForbidden, map[string]string{"error": "invalid credentials or code"})
		return
	}
	tag, updateErr := tx.Exec(ctx, `UPDATE otp_totp_totpdevice SET last_t=$2,last_used_at=now(),
throttling_failure_count=0,throttling_failure_timestamp=NULL WHERE id=$1 AND last_t<$2`, deviceID, counter)
	if updateErr != nil || tag.RowsAffected() != 1 {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	token, err := randomHex(32)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	csrf, err := randomHex(32)
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	tokenBytes, _ := hex.DecodeString(token)
	hash := sha256.Sum256(tokenBytes)
	expires := time.Now().Add(8 * time.Hour)
	_, err = tx.Exec(ctx, `INSERT INTO go_admin_session(token_hash,user_id,csrf_token,expires_at)
VALUES($1,$2,$3,$4)`, hash[:], userID, csrf, expires)
	if err != nil || tx.Commit(ctx) != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "authentication unavailable"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: token, Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: int((8 * time.Hour).Seconds())})
	adminJSON(w, http.StatusOK, map[string]any{"username": username, "csrf_token": csrf, "expires_at": expires})
}

func (s *Service) authenticate(r *http.Request) (*staffSession, error) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, err
	}
	raw, err := hex.DecodeString(cookie.Value)
	if err != nil || len(raw) != 32 {
		return nil, errors.New("invalid session cookie")
	}
	hash := sha256.Sum256(raw)
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Second)
	defer cancel()
	result := &staffSession{TokenHash: hash}
	err = s.DB.QueryRow(ctx, `SELECT s.user_id,u.username,s.csrf_token,s.expires_at
FROM go_admin_session s JOIN auth_user u ON u.id=s.user_id
WHERE s.token_hash=$1 AND s.expires_at>now() AND u.is_active AND u.is_staff`, hash[:]).Scan(
		&result.UserID, &result.Username, &result.CSRF, &result.ExpiresAt)
	return result, err
}

func (s *Service) requireSession(w http.ResponseWriter, r *http.Request, mutation bool) *staffSession {
	staff, err := s.authenticate(r)
	if err != nil {
		adminJSON(w, http.StatusUnauthorized, map[string]string{"error": "staff login required"})
		return nil
	}
	if mutation && (!s.sameOrigin(r) || subtle.ConstantTimeCompare([]byte(staff.CSRF), []byte(r.Header.Get("X-CSRF-Token"))) != 1) {
		adminJSON(w, http.StatusForbidden, map[string]string{"error": "request verification failed"})
		return nil
	}
	return staff
}

func (s *Service) session(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, false)
	if staff != nil {
		adminJSON(w, http.StatusOK, map[string]any{"username": staff.Username, "csrf_token": staff.CSRF, "expires_at": staff.ExpiresAt})
	}
}

func (s *Service) logout(w http.ResponseWriter, r *http.Request) {
	staff := s.requireSession(w, r, true)
	if staff == nil {
		return
	}
	_, err := s.DB.Exec(r.Context(), `DELETE FROM go_admin_session WHERE token_hash=$1`, staff.TokenHash[:])
	if err != nil {
		adminJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "logout unavailable"})
		return
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", Secure: true,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	adminJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
