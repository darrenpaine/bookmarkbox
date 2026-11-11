package ratelimit

import (
	"testing"
	"time"

	"github.com/darrenpaine/bookmarkbox/internal/assert"
)

// clock is a fake time that moves only when told to.
type clock struct{ now time.Time }

func (c *clock) Now() time.Time      { return c.now }
func (c *clock) Add(d time.Duration) { c.now = c.now.Add(d) }
func newLimiter(maxKeys int) (*Limiter, *clock) {
	c := &clock{now: time.Date(2026, 11, 11, 9, 0, 0, 0, time.UTC)}
	l := New(10, 10, maxKeys)
	l.Now = c.Now
	return l, c
}

func TestBurstThenRefill(t *testing.T) {
	l, c := newLimiter(100)
	for i := range 10 {
		if !l.Allow("a") {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
	assert.Equal(t, l.Allow("a"), false)

	// Other keys have their own allowance.
	assert.Equal(t, l.Allow("b"), true)

	// Going over blocks the key for a minute, even though an attempt
	// refills every six seconds.
	c.Add(59 * time.Second)
	assert.Equal(t, l.Allow("a"), false)
	c.Add(time.Second)

	// Then the whole burst is back.
	for i := range 10 {
		if !l.Allow("a") {
			t.Fatalf("attempt %d refused after the block", i+1)
		}
	}
}

// Spread out, ten a minute go through without a block: one every six seconds.
func TestSteadyRate(t *testing.T) {
	l, c := newLimiter(100)
	for range 10 {
		l.Allow("a")
	}
	for i := range 20 {
		c.Add(6 * time.Second)
		if !l.Allow("a") {
			t.Fatalf("attempt %d refused", i+1)
		}
	}
}

// Attempts during a block don't make it longer.
func TestBlockIsNotExtended(t *testing.T) {
	l, c := newLimiter(100)
	for range 11 {
		l.Allow("a")
	}
	for range 5 {
		c.Add(10 * time.Second)
		assert.Equal(t, l.Allow("a"), false)
	}
	c.Add(10 * time.Second)
	assert.Equal(t, l.Allow("a"), true)
}

func TestIdleKeysAreForgotten(t *testing.T) {
	l, c := newLimiter(100)
	l.Allow("a")
	for range 10 {
		l.Allow("b")
	}
	assert.Equal(t, l.Len(), 2)

	// Both have refilled after a minute, so both go.
	c.Add(time.Minute)
	l.Allow("c")
	assert.Equal(t, l.Len(), 1)
}

func TestBlockedKeysAreKept(t *testing.T) {
	l, c := newLimiter(100)
	for range 10 {
		l.Allow("a")
	}
	c.Add(5 * time.Second)
	l.Allow("a") // over: blocked until 65s
	// At 62s a has refilled completely, but its block hasn't ended.
	c.Add(57 * time.Second)
	l.Allow("b") // sweeps
	assert.Equal(t, l.Allow("a"), false)
	c.Add(3 * time.Second)
	assert.Equal(t, l.Allow("a"), true)
}

func TestBusyKeysAreKept(t *testing.T) {
	l, c := newLimiter(100)
	for range 10 {
		l.Allow("a")
	}
	c.Add(30 * time.Second)
	for range 5 {
		l.Allow("a") // uses up the five refilled
	}
	c.Add(31 * time.Second)
	l.Allow("b") // sweeps, but a is still refilling
	assert.Equal(t, l.Len(), 2)
}

func TestFullMapRefusesNewKeys(t *testing.T) {
	l, c := newLimiter(2)
	assert.Equal(t, l.Allow("a"), true)
	assert.Equal(t, l.Allow("b"), true)
	assert.Equal(t, l.Allow("c"), false)
	// Keys already tracked carry on.
	assert.Equal(t, l.Allow("a"), true)

	// Once the others have gone idle there's room again.
	c.Add(time.Minute)
	assert.Equal(t, l.Allow("c"), true)
}
