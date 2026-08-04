package hostlimit

import (
	"context"
	"sync"

	"golang.org/x/time/rate"
)

type Limiter struct {
	mu    sync.Mutex
	hosts map[string]*rate.Limiter // host → limiter
	limit rate.Limit               // requests per second
	burst int                      // burst size for each host
}

func New(perHostRPS float64, burst int) *Limiter {
	limit := rate.Limit(perHostRPS)
	if perHostRPS <= 0 {
		limit = rate.Inf
	}
	if burst < 1 { // Default to 1 if not set or invalid
		burst = 1
	}
	return &Limiter{
		hosts: make(map[string]*rate.Limiter),
		limit: limit,
		burst: burst,
	}
}

// Wait blocks until the host's limiter allows one request or ctx is done.
func (l *Limiter) Wait(ctx context.Context, host string) error {
	return l.limiterFor(host).Wait(ctx) // Wait returns nil if the request is allowed, or an error if the context is done.
}

// limiterFor returns the rate limiter for the given host, creating it if necessary
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
