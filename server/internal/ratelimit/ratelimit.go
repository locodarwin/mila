// Package ratelimit is an in-memory token bucket per key, such as a client
// IP. One process means memory is enough; a restart forgives everyone.
package ratelimit

import (
	"sync"
	"time"
)

// Limiter allows Burst events at once per key, refilling at Rate per second.
type Limiter struct {
	Rate  float64
	Burst float64
	now   func() time.Time

	mu      sync.Mutex
	buckets map[string]*bucket
	swept   time.Time
}

type bucket struct {
	tokens float64
	last   time.Time
}

// New returns a limiter allowing burst events, then perMinute a minute.
func New(perMinute, burst int) *Limiter {
	return &Limiter{Rate: float64(perMinute) / 60, Burst: float64(burst), now: time.Now, buckets: map[string]*bucket{}}
}

// NewPerHour returns a limiter allowing burst events, then perHour an hour.
func NewPerHour(perHour, burst int) *Limiter {
	return &Limiter{Rate: float64(perHour) / 3600, Burst: float64(burst), now: time.Now, buckets: map[string]*bucket{}}
}

// Allow spends one token for key and reports whether one was available.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.Burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate)
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// Take spends n tokens for key if it has them, and otherwise says how long
// until it will (forever, as far as a caller cares, when n is over Burst).
// A refusal spends nothing, so what is too large never starves the rest.
func (l *Limiter) Take(key string, n float64) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)
	b, ok := l.buckets[key]
	if !ok {
		b = &bucket{tokens: l.Burst, last: now}
		l.buckets[key] = b
	}
	b.tokens = min(l.Burst, b.tokens+now.Sub(b.last).Seconds()*l.Rate)
	b.last = now
	if b.tokens < n {
		return false, time.Duration((n - b.tokens) / l.Rate * float64(time.Second))
	}
	b.tokens -= n
	return true, 0
}

// sweep drops full buckets now and then so the map cannot grow forever.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.swept) < time.Minute {
		return
	}
	l.swept = now
	for k, b := range l.buckets {
		if b.tokens+now.Sub(b.last).Seconds()*l.Rate >= l.Burst {
			delete(l.buckets, k)
		}
	}
}
