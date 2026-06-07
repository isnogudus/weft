package server

import (
	"sync"
	"time"
)

// rateLimiter is a simple fixed-window limiter keyed by a string (e.g. client
// IP). It guards the login endpoint against brute force.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string]*window
	limit  int
	window time.Duration
}

type window struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, w time.Duration) *rateLimiter {
	return &rateLimiter{hits: map[string]*window{}, limit: limit, window: w}
}

// allow records an attempt for key and reports whether it is within the limit.
func (r *rateLimiter) allow(key string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	w, ok := r.hits[key]
	if !ok || now.After(w.reset) {
		r.hits[key] = &window{count: 1, reset: now.Add(r.window)}
		return true
	}
	if w.count >= r.limit {
		return false
	}
	w.count++
	return true
}

// reset clears the counter for a key (called on successful login).
func (r *rateLimiter) reset(key string) {
	r.mu.Lock()
	delete(r.hits, key)
	r.mu.Unlock()
}
