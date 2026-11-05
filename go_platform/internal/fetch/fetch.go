// Package fetch provides bounded, source-specific HTTP reads. The pacing and
// cooldown design is adapted from the user's scopeDpi project (Apache-2.0).
package fetch

import (
	"compress/gzip"
	"compress/zlib"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/andybalholm/brotli"
	"github.com/klauspost/compress/zstd"
	utls "github.com/refraction-networking/utls"
	"golang.org/x/net/http2"
)

var (
	ErrBlocked = errors.New("source blocked")
	ErrPaused  = errors.New("source paused")
	ErrBody    = errors.New("response too large")
	ErrURL     = errors.New("invalid source URL")
)

type Policy struct {
	MaxPerHour    int
	MinInterval   time.Duration
	BlockLimit    int
	BlockCooldown time.Duration
	MaxBytes      int64
	Timeout       time.Duration
	UseHTTP2      bool
}

func (p Policy) withDefaults() Policy {
	if p.MaxPerHour < 1 {
		p.MaxPerHour = 30
	}
	if p.MinInterval < time.Second {
		p.MinInterval = time.Second
	}
	if p.BlockLimit < 1 {
		p.BlockLimit = 3
	}
	if p.BlockCooldown < time.Minute {
		p.BlockCooldown = time.Hour
	}
	if p.MaxBytes < 1 {
		p.MaxBytes = 5_000_000
	}
	if p.Timeout < time.Second {
		p.Timeout = 15 * time.Second
	}
	return p
}

// Gate is shared by every caller for one retailer. It serializes acquisition
// so concurrent workers cannot accidentally exceed the retailer's budget.
type Gate struct {
	mu          sync.Mutex
	policy      Policy
	attempts    []time.Time
	last        time.Time
	blocks      int
	pausedUntil time.Time
}

func NewGate(policy Policy) *Gate { return &Gate{policy: policy.withDefaults()} }

func (g *Gate) Acquire(ctx context.Context) error {
	for {
		g.mu.Lock()
		now := time.Now()
		if now.Before(g.pausedUntil) {
			g.mu.Unlock()
			return ErrPaused
		}
		var kept []time.Time
		for _, at := range g.attempts {
			if now.Sub(at) < time.Hour {
				kept = append(kept, at)
			}
		}
		g.attempts = kept
		wait := time.Duration(0)
		if len(kept) >= g.policy.MaxPerHour {
			wait = kept[0].Add(time.Hour).Sub(now)
		}
		if spacing := g.last.Add(g.policy.MinInterval).Sub(now); spacing > wait {
			wait = spacing
		}
		if wait <= 0 {
			g.attempts = append(g.attempts, now)
			g.last = now
			g.mu.Unlock()
			return nil
		}
		g.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return ctx.Err()
		case <-timer.C:
		}
	}
}

func (g *Gate) Record(blocked bool) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if blocked {
		g.blocks++
		if g.blocks >= g.policy.BlockLimit {
			g.pausedUntil = time.Now().Add(g.policy.BlockCooldown)
			g.blocks = 0
		}
	} else {
		g.blocks = 0
	}
}

func ValidateURL(raw, domain string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" ||
		(u.Hostname() != domain && u.Hostname() != "www."+domain) ||
		(u.Port() != "" && u.Port() != "443") {
		return ErrURL
	}
	return nil
}

type Client struct {
	HTTP   *http.Client
	Gate   *Gate
	Policy Policy
}

func NewClient(p Policy) *Client {
	p = p.withDefaults()
	// Keep the cache for the lifetime of the client, not one TCP connection.
	sessions := utls.NewLRUClientSessionCache(100)
	var transport http.RoundTripper
	if p.UseHTTP2 {
		t := &http2.Transport{
			DialTLSContext: func(ctx context.Context, network, addr string, cfg *tls.Config) (net.Conn, error) {
				return dialUTLS(ctx, addr, "h2", sessions, nil)
			},
			MaxDecoderHeaderTableSize:  65536,
			MaxReadFrameSize:           16384,
			MaxHeaderListSize:          262144,
			StrictMaxConcurrentStreams: false,
			IdleConnTimeout:            300 * time.Second,
			ReadIdleTimeout:            240 * time.Second,
			PingTimeout:                30 * time.Second,
		}
		transport = t
	} else {
		transport = &http.Transport{
			MaxIdleConnsPerHost: 2,
			MaxConnsPerHost:     2,
			ForceAttemptHTTP2:   false,
			DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return dialUTLS(ctx, addr, "http/1.1", sessions, nil)
			},
		}
	}
	return &Client{
		HTTP: &http.Client{Transport: transport, Timeout: p.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		Gate: NewGate(p), Policy: p,
	}
}

// dialUTLS establishes a TLS connection using a real Chrome 145 fingerprint
// with full ClientHello customization, ALPN override, and session resumption.
func dialUTLS(ctx context.Context, addr, protocol string, sessions utls.ClientSessionCache, roots *x509.CertPool) (net.Conn, error) {
	serverName := addr
	if host, _, err := net.SplitHostPort(addr); err == nil {
		serverName = host
	}

	dialer := &net.Dialer{Timeout: 15 * time.Second}
	rawConn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}

	config := &utls.Config{
		ServerName:                serverName,
		RootCAs:                   roots,
		NextProtos:                []string{protocol},
		PQSignatureSchemesEnabled: true,
		ClientSessionCache:        sessions,
		OmitEmptyPsk:              true,
	}

	uconn := utls.UClient(rawConn, config, utls.HelloCustom)
	spec := chrome145Spec(protocol)
	if err := uconn.ApplyPreset(&spec); err != nil {
		rawConn.Close()
		return nil, err
	}

	if err := uconn.BuildHandshakeState(); err != nil {
		rawConn.Close()
		return nil, err
	}
	foundALPN := false
	for i, ext := range uconn.Extensions {
		if alpn, ok := ext.(*utls.ALPNExtension); ok {
			alpn.AlpnProtocols = []string{protocol}
			uconn.Extensions[i] = alpn
			foundALPN = true
		}
	}
	if !foundALPN {
		uconn.Extensions = append(uconn.Extensions, &utls.ALPNExtension{AlpnProtocols: []string{protocol}})
	}
	if uconn.HandshakeState.Hello != nil {
		uconn.HandshakeState.Hello.AlpnProtocols = []string{protocol}
	}

	if err := uconn.HandshakeContext(ctx); err != nil {
		rawConn.Close()
		return nil, err
	}
	if negotiated := uconn.ConnectionState().NegotiatedProtocol; negotiated != protocol && !(protocol == "http/1.1" && negotiated == "") {
		uconn.Close()
		return nil, fmt.Errorf("unexpected ALPN %q for %s transport", negotiated, protocol)
	}
	return uconn, nil
}

// chrome145Spec returns the exact ClientHelloSpec used by real Chrome 145.
// This is extracted from enetx/surf's profiles/chrome/145.go.
func chrome145Spec(protocol string) utls.ClientHelloSpec {
	return utls.ClientHelloSpec{
		CipherSuites: []uint16{
			utls.GREASE_PLACEHOLDER,
			utls.TLS_AES_128_GCM_SHA256,
			utls.TLS_AES_256_GCM_SHA384,
			utls.TLS_CHACHA20_POLY1305_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_ECDHE_ECDSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_ECDHE_ECDSA_WITH_CHACHA20_POLY1305,
			utls.TLS_ECDHE_RSA_WITH_CHACHA20_POLY1305,
			utls.TLS_ECDHE_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_ECDHE_RSA_WITH_AES_256_CBC_SHA,
			utls.TLS_RSA_WITH_AES_128_GCM_SHA256,
			utls.TLS_RSA_WITH_AES_256_GCM_SHA384,
			utls.TLS_RSA_WITH_AES_128_CBC_SHA,
			utls.TLS_RSA_WITH_AES_256_CBC_SHA,
		},
		CompressionMethods: []byte{0x00},
		Extensions: utls.ShuffleChromeTLSExtensions(
			[]utls.TLSExtension{
				&utls.UtlsGREASEExtension{},
				&utls.SNIExtension{},
				&utls.ExtendedMasterSecretExtension{},
				&utls.RenegotiationInfoExtension{
					Renegotiation: utls.RenegotiateOnceAsClient,
				},
				&utls.SupportedCurvesExtension{
					Curves: []utls.CurveID{
						utls.GREASE_PLACEHOLDER,
						utls.X25519MLKEM768,
						utls.X25519,
						utls.CurveP256,
						utls.CurveP384,
					},
				},
				&utls.SupportedPointsExtension{
					SupportedPoints: []byte{0x00},
				},
				&utls.SessionTicketExtension{},
				&utls.ALPNExtension{
					AlpnProtocols: []string{protocol},
				},
				&utls.StatusRequestExtension{},
				&utls.SignatureAlgorithmsExtension{
					SupportedSignatureAlgorithms: []utls.SignatureScheme{
						utls.ECDSAWithP256AndSHA256,
						utls.PSSWithSHA256,
						utls.PKCS1WithSHA256,
						utls.ECDSAWithP384AndSHA384,
						utls.PSSWithSHA384,
						utls.PKCS1WithSHA384,
						utls.PSSWithSHA512,
						utls.PKCS1WithSHA512,
					},
				},
				&utls.SCTExtension{},
				&utls.KeyShareExtension{
					KeyShares: []utls.KeyShare{
						{Group: utls.GREASE_PLACEHOLDER, Data: []byte{0}},
						{Group: utls.X25519MLKEM768},
						{Group: utls.X25519},
					},
				},
				&utls.PSKKeyExchangeModesExtension{
					Modes: []uint8{
						utls.PskModeDHE,
					},
				},
				&utls.SupportedVersionsExtension{
					Versions: []uint16{
						utls.GREASE_PLACEHOLDER,
						utls.VersionTLS13,
						utls.VersionTLS12,
					},
				},
				&utls.UtlsCompressCertExtension{
					Algorithms: []utls.CertCompressionAlgo{
						utls.CertCompressionBrotli,
					},
				},
				&utls.ApplicationSettingsExtensionNew{
					SupportedProtocols: []string{"h2"},
				},
				utls.BoringGREASEECH(),
				&utls.UtlsGREASEExtension{},
				&utls.UtlsPreSharedKeyExtension{},
			},
		),
	}
}

// browserHeaders applies real Chrome 145 document navigation headers.
// This matches enetx/surf's profiles/chrome/headers.go and ua.go.
func browserHeaders(req *http.Request) {
	req.Header.Set("Sec-Ch-Ua", `"Not:A-Brand";v="99", "Google Chrome";v="145", "Chromium";v="145"`)
	req.Header.Set("Sec-Ch-Ua-Mobile", "?0")
	req.Header.Set("Sec-Ch-Ua-Platform", `"Windows"`)
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/145.0.0.0 Safari/537.36")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,image/apng,*/*;q=0.8,application/signed-exchange;v=b3;q=0.7")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "none")
	req.Header.Set("Sec-Fetch-User", "?1")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br, zstd")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9")
	req.Header.Set("Priority", "u=0, i")
}

func (c *Client) GetHTML(ctx context.Context, raw, domain string) ([]byte, error) {
	if err := ValidateURL(raw, domain); err != nil {
		return nil, err
	}
	if err := c.Gate.Acquire(ctx); err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	browserHeaders(req)
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	// Cloudflare marks challenge pages independently of the HTTP status code.
	// Never let a 200 challenge page reach the product parser as an offer.
	if strings.EqualFold(resp.Header.Get("cf-mitigated"), "challenge") {
		if os.Getenv("AHSANLY_SAVE_BODY") != "" {
			data, _ := readDecodedBody(resp, c.Policy.MaxBytes)
			_ = os.WriteFile(os.Getenv("AHSANLY_SAVE_BODY"), data, 0644)
		}
		c.Gate.Record(true)
		return nil, ErrBlocked
	}
	if resp.StatusCode == 403 || resp.StatusCode == 429 || resp.StatusCode == 503 {
		c.Gate.Record(true)
		return nil, fmt.Errorf("%w: HTTP %d", ErrBlocked, resp.StatusCode)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("source HTTP %d", resp.StatusCode)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	if contentType != "" && !strings.Contains(contentType, "html") && !strings.Contains(contentType, "text") {
		return nil, errors.New("source did not return HTML")
	}
	data, err := readDecodedBody(resp, c.Policy.MaxBytes)
	if err != nil {
		return nil, err
	}
	lower := strings.ToLower(string(data))
	if strings.Contains(lower, "captcha") || strings.Contains(lower, "robot check") || strings.Contains(lower, "access denied") {
		c.Gate.Record(true)
		return nil, ErrBlocked
	}
	c.Gate.Record(false)
	return data, nil
}

// readDecodedBody decompresses gzip/deflate/br/zstd response bodies.
// This matches scopeDpi's readDecodedBody in internal/discovery/web/scrape.go.
func readDecodedBody(resp *http.Response, maxBytes int64) ([]byte, error) {
	reader := io.Reader(resp.Body)
	var closer io.Closer
	switch strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Encoding"))) {
	case "", "identity":
	case "gzip":
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		reader = gz
		closer = gz
	case "deflate":
		zr, err := zlib.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		reader = zr
		closer = zr
	case "br":
		reader = brotli.NewReader(resp.Body)
	case "zstd":
		zr, err := zstd.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		reader = zr
		defer zr.Close()
	default:
		return nil, fmt.Errorf("unsupported content encoding %q", resp.Header.Get("Content-Encoding"))
	}
	if closer != nil {
		defer closer.Close()
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, ErrBody
	}
	return data, nil
}
