package httpx

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/netip"
	"syscall"
	"time"
)

const DefaultUserAgent = "Mozilla/5.0 (compatible; multiagent-seo-bot/1.0)"

type config struct {
	timeout       time.Duration
	dialTimeout   time.Duration
	userAgent     string
	maxRedirects  int
	blockPrivate  bool
	allowLoopback bool
	insecureTLS   bool
}

type Option func(*config)

func WithTimeout(d time.Duration) Option     { return func(c *config) { c.timeout = d } }
func WithDialTimeout(d time.Duration) Option { return func(c *config) { c.dialTimeout = d } }
func WithUserAgent(ua string) Option         { return func(c *config) { c.userAgent = ua } }

// WithMaxRedirects: n <= 0 keeps Go's default redirect policy.
func WithMaxRedirects(n int) Option { return func(c *config) { c.maxRedirects = n } }

// BlockPrivateIPs is the SSRF guard for clients that fetch arbitrary
// user-supplied URLs: it refuses to dial loopback/private/link-local/CGNAT.
func BlockPrivateIPs() Option { return func(c *config) { c.blockPrivate = true } }

func AllowLoopback() Option { return func(c *config) { c.allowLoopback = true } }

// InsecureTLS is a deliberate opt-in so disabled cert verification can't spread
// by copy-paste the way it did across the old per-adapter clients.
func InsecureTLS() Option { return func(c *config) { c.insecureTLS = true } }

func New(opts ...Option) *http.Client {
	cfg := config{
		timeout:     30 * time.Second,
		dialTimeout: 10 * time.Second,
		userAgent:   DefaultUserAgent,
	}
	for _, apply := range opts {
		apply(&cfg)
	}

	dialer := &net.Dialer{Timeout: cfg.dialTimeout}
	if cfg.blockPrivate {
		allowLoopback := cfg.allowLoopback
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("httpx dial guard: %w", err)
			}
			ip, err := netip.ParseAddr(host)
			if err != nil {
				return fmt.Errorf("httpx dial guard parse %q: %w", host, err)
			}
			if allowLoopback && ip.IsLoopback() {
				return nil
			}
			if DisallowedIP(ip) {
				return fmt.Errorf("httpx dial guard: blocked address %s", ip)
			}
			return nil
		}
	}

	tr := &http.Transport{DialContext: dialer.DialContext}
	if cfg.insecureTLS {
		tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec // explicit opt-in via InsecureTLS()
	}

	var rt http.RoundTripper = tr
	if cfg.userAgent != "" {
		rt = userAgentTransport{base: tr, ua: cfg.userAgent}
	}

	client := &http.Client{Timeout: cfg.timeout, Transport: rt}
	if cfg.maxRedirects > 0 {
		max := cfg.maxRedirects
		client.CheckRedirect = func(_ *http.Request, via []*http.Request) error {
			if len(via) >= max {
				return fmt.Errorf("httpx: stopped after %d redirects", max)
			}
			return nil
		}
	}
	return client
}

type userAgentTransport struct {
	base http.RoundTripper
	ua   string
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Only set the default UA when the caller didn't, so a per-request override wins.
	if req.Header.Get("User-Agent") == "" {
		req = req.Clone(req.Context())
		req.Header.Set("User-Agent", t.ua)
	}
	return t.base.RoundTrip(req)
}

var cgnat = netip.MustParsePrefix("100.64.0.0/10")

func DisallowedIP(ip netip.Addr) bool {
	ip = ip.Unmap()
	switch {
	case ip.IsLoopback(),
		ip.IsPrivate(),
		ip.IsLinkLocalUnicast(),
		ip.IsLinkLocalMulticast(),
		ip.IsMulticast(),
		ip.IsUnspecified(),
		ip.IsInterfaceLocalMulticast():
		return true
	}
	if ip.Is4() && cgnat.Contains(ip) {
		return true
	}
	return false
}
