package ledger

import (
	"net"
	"net/http"
	"sync"
	"time"
)

// RateLimiter enforces a fixed-window per-IP request limit.
type RateLimiter struct {
	mu         sync.Mutex
	windows    map[string]*windowEntry
	limit      int           // max requests per window
	window     time.Duration // window duration
	cleanEvery int           // evict stale entries every N requests
	counter    int
}

type windowEntry struct {
	count     int
	windowEnd time.Time
}

// NewRateLimiter creates a RateLimiter allowing limit requests per window per IP.
func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		windows:    make(map[string]*windowEntry),
		limit:      limit,
		window:     window,
		cleanEvery: 500,
	}
}

// Allow returns true if the request from ip is within the allowed rate.
func (rl *RateLimiter) Allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()

	entry, ok := rl.windows[ip]
	if !ok || now.After(entry.windowEnd) {
		rl.windows[ip] = &windowEntry{
			count:     1,
			windowEnd: now.Add(rl.window),
		}
		return true
	}

	entry.count++
	if entry.count > rl.limit {
		return false
	}
	return true
}

// evictExpired removes entries whose window has passed.  Must be called with
// rl.mu held.
func (rl *RateLimiter) evictExpired() {
	now := time.Now()
	for ip, entry := range rl.windows {
		if now.After(entry.windowEnd) {
			delete(rl.windows, ip)
		}
	}
}

// Middleware returns an http.Handler that rate-limits by remote IP.  If the
// limit is exceeded it responds with 429 Too Many Requests.
func (rl *RateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := remoteIP(r)

		rl.mu.Lock()
		rl.counter++
		if rl.counter%rl.cleanEvery == 0 {
			rl.evictExpired()
		}
		rl.mu.Unlock()

		if !rl.Allow(ip) {
			writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
			return
		}

		next.ServeHTTP(w, r)
	})
}

// remoteIP extracts the client IP from the request, honoring X-Forwarded-For
// when present (first hop only).
func remoteIP(r *http.Request) string {
	if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
		// Take the first IP in the comma-separated list.
		for i := 0; i < len(fwd); i++ {
			if fwd[i] == ',' {
				fwd = fwd[:i]
				break
			}
		}
		if ip := net.ParseIP(fwd); ip != nil {
			return ip.String()
		}
	}

	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
