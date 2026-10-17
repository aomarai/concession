package ratelimit

import (
	"sync"
	"testing"
	"time"
)

type clock struct {
	mu sync.Mutex
	t  time.Time
}

func (c *clock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

func (c *clock) advance(d time.Duration) {
	c.mu.Lock()
	c.t = c.t.Add(d)
	c.mu.Unlock()
}

func newLimiter(limit int, per time.Duration) (*Limiter, *clock) {
	c := &clock{t: time.Unix(1_000_000, 0)}
	l := New(limit, per)
	l.now = c.now
	return l, c
}

func TestAllowsUpToTheLimitThenDenies(t *testing.T) {
	l, _ := newLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d should be allowed", i+1)
		}
	}
	ok, retry := l.Allow("a")
	if ok {
		t.Fatal("fourth request should be denied")
	}
	if retry <= 0 || retry > 20*time.Second {
		t.Errorf("retry-after should be about one token (20s), got %v", retry)
	}
}

func TestKeysAreIndependent(t *testing.T) {
	l, _ := newLimiter(1, time.Minute)
	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("a should be allowed")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Error("b has its own bucket")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("a is out of tokens")
	}
}

func TestTokensRefillOverTime(t *testing.T) {
	l, c := newLimiter(2, time.Minute)
	l.Allow("a")
	l.Allow("a")
	if ok, _ := l.Allow("a"); ok {
		t.Fatal("should be empty")
	}
	c.advance(30 * time.Second) // one token back
	if ok, _ := l.Allow("a"); !ok {
		t.Error("a token should have refilled")
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("only one token refilled")
	}
	c.advance(time.Hour) // refill is capped at the limit
	for i := 0; i < 2; i++ {
		if ok, _ := l.Allow("a"); !ok {
			t.Fatalf("request %d after a long pause should be allowed", i+1)
		}
	}
	if ok, _ := l.Allow("a"); ok {
		t.Error("the bucket must not exceed its capacity")
	}
}

func TestNonPositiveLimitDisables(t *testing.T) {
	for _, limit := range []int{0, -1} {
		l := New(limit, time.Minute)
		if l != nil {
			t.Errorf("New(%d) should return nil (disabled)", limit)
		}
		var nilLimiter *Limiter
		if ok, retry := nilLimiter.Allow("a"); !ok || retry != 0 {
			t.Error("a nil limiter allows everything")
		}
	}
}

func TestIdleBucketsAreSwept(t *testing.T) {
	l, c := newLimiter(2, time.Minute)
	for _, k := range []string{"a", "b", "c"} {
		l.Allow(k)
	}
	if n := l.size(); n != 3 {
		t.Fatalf("want 3 buckets, got %d", n)
	}
	c.advance(2 * time.Minute) // all refilled, and the sweep interval has passed
	l.Allow("d")
	if n := l.size(); n != 1 {
		t.Errorf("idle buckets should be dropped, got %d", n)
	}
}

func TestSweepKeepsBucketsThatAreStillDraining(t *testing.T) {
	l, c := newLimiter(2, time.Hour)
	l.Allow("busy")
	l.Allow("busy") // empty
	c.advance(sweepEvery)
	l.Allow("other")
	if ok, _ := l.Allow("busy"); ok {
		t.Error("a bucket that has not refilled must survive the sweep")
	}
}

func TestConcurrentUse(t *testing.T) {
	l := New(1000, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				l.Allow("k")
			}
		}()
	}
	wg.Wait()
	// 1000 tokens, 1000 calls: all allowed, so the next one is the first denial
	// unless a token refilled meanwhile (at most a few in this short window).
	denied := 0
	for j := 0; j < 20; j++ {
		if ok, _ := l.Allow("k"); !ok {
			denied++
		}
	}
	if denied == 0 {
		t.Error("expected the bucket to be exhausted")
	}
}

func TestGateCapsConcurrentHolders(t *testing.T) {
	g := NewGate(2)
	r1, ok := g.Acquire("u")
	if !ok {
		t.Fatal("first acquire")
	}
	r2, ok := g.Acquire("u")
	if !ok {
		t.Fatal("second acquire")
	}
	if _, ok := g.Acquire("u"); ok {
		t.Fatal("third acquire should be refused")
	}
	if _, ok := g.Acquire("other"); !ok {
		t.Error("another key has its own slots")
	}
	r1()
	r1() // releasing twice must not free a second slot
	r3, ok := g.Acquire("u")
	if !ok {
		t.Fatal("a slot was released")
	}
	if _, ok := g.Acquire("u"); ok {
		t.Error("double release must not leak slots")
	}
	r2()
	r3()
	if n := g.size(); n != 1 { // only "other" still holds a slot
		t.Errorf("empty keys should be removed, got %d", n)
	}
}

func TestNilGateAllowsEverything(t *testing.T) {
	for _, g := range []*Gate{NewGate(0), NewGate(-3), nil} {
		release, ok := g.Acquire("u")
		if !ok {
			t.Fatal("a disabled gate allows everything")
		}
		release() // must be safe to call
	}
}
