package fetch

import (
	"context"
	"crypto/x509"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	utls "github.com/refraction-networking/utls"
)

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestValidateURL(t *testing.T) {
	for _, raw := range []string{"http://amazon.eg/dp/B01", "https://amazon.eg.evil.test/dp/B01", "https://user@amazon.eg/dp/B01", "https://amazon.eg:8443/dp/B01"} {
		if err := ValidateURL(raw, "amazon.eg"); !errors.Is(err, ErrURL) {
			t.Fatalf("accepted %q: %v", raw, err)
		}
	}
	if err := ValidateURL("https://www.amazon.eg/dp/B01?tag=x", "amazon.eg"); err != nil {
		t.Fatal(err)
	}
}

func TestBudgetAndCooldown(t *testing.T) {
	g := NewGate(Policy{MaxPerHour: 1, MinInterval: time.Second, BlockLimit: 2, BlockCooldown: time.Hour})
	if err := g.Acquire(context.Background()); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if err := g.Acquire(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("budget: %v", err)
	}
	g.Record(true)
	g.Record(true)
	if err := g.Acquire(context.Background()); !errors.Is(err, ErrPaused) {
		t.Fatalf("cooldown: %v", err)
	}
}

func TestChallengeNeverReturnsOfferHTML(t *testing.T) {
	c := NewClient(Policy{MaxPerHour: 10})
	c.HTTP.Transport = roundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Hostname() != "amazon.eg" {
			t.Fatalf("wrong host: %s", r.URL.Host)
		}
		return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"text/html"}}, Body: io.NopCloser(strings.NewReader("<html>Robot Check CAPTCHA</html>"))}, nil
	})
	if _, err := c.GetHTML(context.Background(), "https://amazon.eg/dp/B01", "amazon.eg"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("challenge: %v", err)
	}
}

func TestCloudflareChallengeHeaderStopsParser(t *testing.T) {
	c := NewClient(Policy{MaxPerHour: 10})
	c.HTTP.Transport = roundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"Cf-Mitigated": []string{"challenge"},
				"Content-Type": []string{"text/html"},
			},
			Body: io.NopCloser(strings.NewReader("<html>Challenge in progress</html>")),
		}, nil
	})
	if _, err := c.GetHTML(context.Background(), "https://amazon.eg/dp/B01", "amazon.eg"); !errors.Is(err, ErrBlocked) {
		t.Fatalf("challenge header classified as %v", err)
	}
}

func TestDialUTLSVerifiesCertificateAndALPN(t *testing.T) {
	for _, protocol := range []string{"http/1.1", "h2"} {
		t.Run(protocol, func(t *testing.T) {
			server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
			server.EnableHTTP2 = protocol == "h2"
			server.StartTLS()
			defer server.Close()

			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			sessions := utls.NewLRUClientSessionCache(2)
			if conn, err := dialUTLS(ctx, server.Listener.Addr().String(), protocol, sessions, nil); err == nil {
				conn.Close()
				t.Fatal("accepted untrusted certificate")
			}

			leaf, err := x509.ParseCertificate(server.TLS.Certificates[0].Certificate[0])
			if err != nil {
				t.Fatal(err)
			}
			roots := x509.NewCertPool()
			roots.AddCert(leaf)
			conn, err := dialUTLS(ctx, server.Listener.Addr().String(), protocol, sessions, roots)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if got := conn.(*utls.UConn).ConnectionState().NegotiatedProtocol; got != protocol {
				t.Fatalf("negotiated %q, want %q", got, protocol)
			}
		})
	}
}
