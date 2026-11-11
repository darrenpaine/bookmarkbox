// Package ratelimit counts attempts per key, such as a route and a client IP,
// and refuses them once a key goes over its rate.
//
// The counts are kept in memory, so they are per server and start again on a
// restart. More than one server would need a shared store behind the same
// Allow method.
package ratelimit

import (
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// Limiter allows each key a burst of attempts, refilled at a steady rate.
// A key that goes over is then refused for a whole minute, rather than being
// let back in as soon as one attempt has refilled: someone typing again by
// hand would otherwise never notice the limit.
type Limiter struct {
	mu        sync.Mutex
	every     rate.Limit
	burst     int
	maxKeys   int
	keys      map[string]*key
	lastSweep time.Time

	// Now is the clock, replaceable in tests.
	Now func() time.Time
}

// New allows perMinute attempts a minute for each key, and up to burst at
// once. At most maxKeys keys are tracked at a time.
func New(perMinute, burst, maxKeys int) *Limiter {
	return &Limiter{
		every:   rate.Limit(float64(perMinute) / 60),
		burst:   burst,
		maxKeys: maxKeys,
		keys:    make(map[string]*key),
		Now:     time.Now,
	}
}

// Block is how long a key is refused once it goes over.
const Block = time.Minute

type key struct {
	limiter      *rate.Limiter
	blockedUntil time.Time
}

// Allow reports whether an attempt for key may go ahead, and counts it if so.
// When maxKeys keys are already busy, a new key is refused rather than
// letting the map grow without bound.
func (l *Limiter) Allow(name string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.Now()
	l.sweep(now)

	k, ok := l.keys[name]
	if !ok {
		if len(l.keys) >= l.maxKeys {
			return false
		}
		k = &key{limiter: rate.NewLimiter(l.every, l.burst)}
		l.keys[name] = k
	}
	if now.Before(k.blockedUntil) {
		return false
	}
	if !k.limiter.AllowN(now, 1) {
		k.blockedUntil = now.Add(Block)
		return false
	}
	return true
}

// sweep forgets keys that have refilled completely, at most once a minute. A
// full key behaves exactly like a new one, so nothing is lost.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < time.Minute {
		return
	}
	l.lastSweep = now
	for name, k := range l.keys {
		if !now.Before(k.blockedUntil) && k.limiter.TokensAt(now) >= float64(l.burst) {
			delete(l.keys, name)
		}
	}
}

// Len is the number of keys tracked.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.keys)
}
