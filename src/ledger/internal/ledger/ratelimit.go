package ledger

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ipRateLimiter implements a per-IP fixed-window rate limiter.
// Each IP address is allowed at most limit requests per window duration.
// Stale window entries are evicted lazily on every Allow call.
type ipRateLimiter struct {
	mu      sync.Mutex
	entries map[string]*rlEntry
	limit   int
	window  time.Duration
}

type rlEntry struct {
	count     int
	windowEnd time.Time
}

// newIPRateLimiter returns a rate limiter that allows at most limit requests
// per window per unique IP address.
func newIPRateLimiter(limit int, window time.Duration) *ipRateLimiter {
	return &ipRateLimiter{
		entries: make(map[string]*rlEntry),
		limit:   limit,
		window:  window,
	}
}

// Allow returns true if the request from ip is within the rate limit.
// It also performs lazy eviction of entries whose window has expired.
func (rl *ipRateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	// Lazy clean-up: remove entries whose window has elapsed.
	for k, e := range rl.entries {
		if now.After(e.windowEnd) {
			delete(rl.entries, k)
		}
	}

	entry, ok := rl.entries[ip]
	if !ok || now.After(entry.windowEnd) {
		rl.entries[ip] = &rlEntry{count: 1, windowEnd: now.Add(rl.window)}
		return true
	}

	if entry.count >= rl.limit {
		return false
	}

	entry.count++
	return true
}

// extractIP returns the client's IP address from the request, stripping the
// port when present.
func extractIP(r *http.Request) string {
	host := r.Header.Get("X-Forwarded-For")
	if host != "" {
		// X-Forwarded-For may be a comma-separated list; use the first entry.
		for i := 0; i < len(host); i++ {
			if host[i] == ',' {
				host = host[:i]
				break
			}
		}
		if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
			return ip.String()
		}
	}

	host = r.Header.Get("X-Real-IP")
	if ip := net.ParseIP(strings.TrimSpace(host)); ip != nil {
		return ip.String()
	}

	remoteHost, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return remoteHost
}
