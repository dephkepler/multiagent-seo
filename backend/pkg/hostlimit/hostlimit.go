package hostlimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

// Limiter throttles outbound requests per host so a wide crawl stays polite to
// each site and doesn't exhaust local sockets against any single host.
type Limiter struct {
	mu    sync.Mutex
	hosts map[string]*rate.Limiter
	limit rate.Limit
	burst int
}

// New builds a limiter allowing perHostRPS requests/second per host with the
// given burst. perHostRPS <= 0 means unlimited; burst < 1 is raised to 1.
func New(perHostRPS float64, burst int) *Limiter {
	limit := rate.Limit(perHostRPS)
	if perHostRPS <= 0 {
		limit = rate.Inf
	}
	if burst < 1 {
		burst = 1
	}
	return &Limiter{hosts: make(map[string]*rate.Limiter), limit: limit, burst: burst}
}

// Wait blocks until the host's limiter allows one request or ctx is done.
func (l *Limiter) Wait(ctx context.Context, host string) error {
	return l.limiterFor(host).Wait(ctx)
}

func (l *Limiter) limiterFor(host string) *rate.Limiter {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.hosts[host]
	if !ok {
		lim = rate.NewLimiter(l.limit, l.burst)
		l.hosts[host] = lim
	}
	return lim
}
