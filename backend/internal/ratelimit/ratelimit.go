// Package ratelimit holds small in-memory limiters: a token bucket for
// request rates and a gate for concurrent holders (e.g. open event streams).
// State is per process; running several instances multiplies the limits.
package ratelimit

import (
	"sync"
	"time"
)

// sweepEvery is how often idle buckets are dropped so memory stays bounded.
const sweepEvery = time.Minute

// maxTrackedKeys caps how many keys a limiter remembers. Past it, new keys are
// let through untracked until the next sweep frees room: an attacker who
// rotates through addresses can already dodge a per-address limit, so failing
// open costs little, while failing closed would lock real users out.
const maxTrackedKeys = 100_000

type bucket struct {
	tokens float64
	last   time.Time
}

// Limiter is a token bucket per key: it holds up to `limit` tokens, refilled
// continuously so a full bucket is restored over `per`. A nil Limiter allows
// everything, which is how a limit is switched off.
type Limiter struct {
	mu        sync.Mutex
	capacity  float64
	rate      float64 // tokens per second
	now       func() time.Time
	buckets   map[string]*bucket
	lastSweep time.Time
	maxKeys   int
}

// New returns a limiter allowing `limit` requests per key per `per`, or nil
// (no limit) when limit or per is not positive.
func New(limit int, per time.Duration) *Limiter {
	if limit <= 0 || per <= 0 {
		return nil
	}
	return &Limiter{
		capacity: float64(limit),
		rate:     float64(limit) / per.Seconds(),
		now:      time.Now,
		buckets:  map[string]*bucket{},
		maxKeys:  maxTrackedKeys,
	}
}

// Allow takes a token for key. When none is left it reports how long until
// one is.
func (l *Limiter) Allow(key string) (bool, time.Duration) {
	if l == nil {
		return true, 0
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	b, ok := l.buckets[key]
	if !ok {
		if len(l.buckets) >= l.maxKeys {
			return true, 0 // at the cap: do not remember this key (see maxTrackedKeys)
		}
		b = &bucket{tokens: l.capacity, last: now}
		l.buckets[key] = b
	}
	b.tokens = l.refilled(b, now)
	b.last = now
	if b.tokens >= 1 {
		b.tokens--
		return true, 0
	}
	return false, time.Duration((1 - b.tokens) / l.rate * float64(time.Second))
}

func (l *Limiter) refilled(b *bucket, now time.Time) float64 {
	t := b.tokens + now.Sub(b.last).Seconds()*l.rate
	if t > l.capacity {
		return l.capacity
	}
	return t
}

// sweep drops buckets that have refilled completely: they behave exactly like
// a brand-new bucket, so forgetting them changes nothing.
func (l *Limiter) sweep(now time.Time) {
	if now.Sub(l.lastSweep) < sweepEvery {
		return
	}
	l.lastSweep = now
	for k, b := range l.buckets {
		if l.refilled(b, now) >= l.capacity {
			delete(l.buckets, k)
		}
	}
}

func (l *Limiter) size() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.buckets)
}

// Gate limits how many things a key may hold at once. A nil Gate allows
// everything.
type Gate struct {
	mu    sync.Mutex
	max   int
	count map[string]int
}

// NewGate returns a gate allowing max concurrent holders per key, or nil (no
// limit) when max is not positive.
func NewGate(max int) *Gate {
	if max <= 0 {
		return nil
	}
	return &Gate{max: max, count: map[string]int{}}
}

// Acquire takes a slot for key. The returned release func frees it and is safe
// to call more than once.
func (g *Gate) Acquire(key string) (release func(), ok bool) {
	if g == nil {
		return func() {}, true
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.count[key] >= g.max {
		return nil, false
	}
	g.count[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			g.mu.Lock()
			defer g.mu.Unlock()
			if g.count[key]--; g.count[key] <= 0 {
				delete(g.count, key)
			}
		})
	}, true
}

func (g *Gate) size() int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return len(g.count)
}
