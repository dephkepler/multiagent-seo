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

const (
	// User-Agent string to use when crawling
	DefaultUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	MaxResponseBytes = 1 << 20 // 1 MiB, for sanity limits on responses
	MaxPageBytes     = 4 << 20 // 4 MiB, for sanity limits on HTML pages
)

type config struct {
	timeout       time.Duration // Total timeout for the request, including all retries and redirects.
	dialTimeout   time.Duration // Timeout for the initial TCP connection (part of the total timeout).
	userAgent     string        // User-Agent header DefaultUserAgent
	maxRedirects  int           // Maximum number of redirects to follow.
	blockPrivate  bool          // Whether to block private IP addresses.
	allowLoopback bool          // loopback addresses for testing
	insecureTLS   bool          // Whether to skip TLS certificate verification.
}

func defaults() config {
	return config{
		timeout:     30 * time.Second,
		dialTimeout: 10 * time.Second,
		userAgent:   DefaultUserAgent,
	}
}

type Option func(*config)

func WithTimeout(d time.Duration) Option     { return func(c *config) { c.timeout = d } }
func WithDialTimeout(d time.Duration) Option { return func(c *config) { c.dialTimeout = d } }
func WithUserAgent(ua string) Option         { return func(c *config) { c.userAgent = ua } }
func WithMaxRedirects(n int) Option          { return func(c *config) { c.maxRedirects = n } }
func BlockPrivateIPs() Option                { return func(c *config) { c.blockPrivate = true } }
func AllowLoopback() Option                  { return func(c *config) { c.allowLoopback = true } }
func InsecureTLS() Option                    { return func(c *config) { c.insecureTLS = true } }

func New(opts ...Option) *http.Client {
	cfg := defaults()
	for _, apply := range opts {
		apply(&cfg)
	}
	return &http.Client{
		Timeout:       cfg.timeout,
		Transport:     transportFor(cfg),
		CheckRedirect: redirectLimit(cfg.maxRedirects),
	}
}

func NewTransport(opts ...Option) http.RoundTripper {
	cfg := defaults()
	for _, apply := range opts {
		apply(&cfg)
	}
	return transportFor(cfg)
}

func transportFor(cfg config) http.RoundTripper {
	transport := &http.Transport{
		DialContext: newDialer(cfg).DialContext,
	}
	if cfg.insecureTLS {
		transport.TLSClientConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}
	return withUserAgent(transport, cfg.userAgent)
}

// newDialer builds the dialer; with BlockPrivateIPs it installs the SSRF guard.
func newDialer(cfg config) *net.Dialer {
	d := &net.Dialer{
		Timeout: cfg.dialTimeout,
	}
	if cfg.blockPrivate {
		d.Control = dialGuard(cfg.allowLoopback)
	}
	return d
}

// dialGuard refuses connections to internal/private addresses (SSRF defense).
func dialGuard(allowLoopback bool) func(network, address string, _ syscall.RawConn) error {
	return func(_, address string, _ syscall.RawConn) error {
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

// redirectLimit returns nil (Go's default policy) when max <= 0, otherwise a
// policy that stops the chain after max hops.
func redirectLimit(max int) func(*http.Request, []*http.Request) error {
	if max <= 0 {
		return nil
	}
	return func(_ *http.Request, via []*http.Request) error {
		if len(via) >= max {
			return fmt.Errorf("httpx: stopped after %d redirects", max)
		}
		return nil
	}
}

// withUserAgent sets the default UA unless the caller already set one; an empty
// ua leaves the transport untouched.
func withUserAgent(rt http.RoundTripper, ua string) http.RoundTripper {
	if ua == "" {
		return rt
	}
	return userAgentTransport{base: rt, ua: ua}
}

type userAgentTransport struct {
	base http.RoundTripper
	ua   string
}

func (t userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
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
