package admin

import (
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDjangoPasswordAndTOTP(t *testing.T) {
	// The PBKDF2 fixture was generated independently with Python hashlib.
	encoded := "pbkdf2_sha256$10000$integration-salt$1ojjG/dDl5wqb8r2csASu+JkeXhjXft504k0zW/lumo="
	if !verifyDjangoPassword(encoded, "correct horse") || verifyDjangoPassword(encoded, "wrong") {
		t.Fatal("Django PBKDF2 verification failed")
	}
	// RFC 6238 SHA-1 vector at Unix time 59, 8 digits.
	secret := "3132333435363738393031323334353637383930"
	counter, valid := verifyTOTP(secret, "94287082", 30, 0, 8, 0, 0, -1, time.Unix(59, 0))
	if !valid || counter != 1 {
		t.Fatalf("RFC TOTP code rejected: counter=%d valid=%v", counter, valid)
	}
	if _, valid = verifyTOTP(secret, "94287082", 30, 0, 8, 0, 0, 1, time.Unix(59, 0)); valid {
		t.Fatal("replayed TOTP code accepted")
	}
}

func TestAdminOriginMustMatchConfiguredHTTPSHost(t *testing.T) {
	s := &Service{Origin: "https://134.122.89.95"}
	r := httptest.NewRequest("POST", "http://134.122.89.95/api/v1/admin/session", strings.NewReader("{}"))
	r.Header.Set("Origin", "https://134.122.89.95")
	if !s.sameOrigin(r) {
		t.Fatal("correct origin rejected")
	}
	r.Header.Set("Origin", "https://evil.example")
	if s.sameOrigin(r) {
		t.Fatal("cross-origin request accepted")
	}
	r.Header.Set("Origin", "https://134.122.89.95")
	r.Host = "evil.example"
	if s.sameOrigin(r) {
		t.Fatal("mismatched Host accepted")
	}
}
