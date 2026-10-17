package ratelimit

import (
	"sync"
	"sync/atomic"
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

func TestNonPositivePeriodDisables(t *testing.T) {
	// A zero period would make the refill rate infinite and the limit a no-op.
	for _, per := range []time.Duration{0, -time.Second} {
		if New(5, per) != nil {
			t.Errorf("New(5, %v) should return nil (disabled)", per)
		}
	}
}

func TestBucketCountIsBounded(t *testing.T) {
	l, _ := newLimiter(1, time.Hour)
	l.maxKeys = 3
	for _, k := range []string{"a", "b", "c"} {
		if ok, _ := l.Allow(k); !ok {
			t.Fatalf("%s should be allowed", k)
		}
	}
	// At the cap new keys are let through untracked, so memory stays bounded.
	for _, k := range []string{"d", "e", "f"} {
		if ok, _ := l.Allow(k); !ok {
			t.Errorf("%s: new keys beyond the cap are allowed (fail open)", k)
		}
	}
	if n := l.size(); n != 3 {
		t.Errorf("buckets = %d, want 3", n)
	}
	// Tracked keys are still limited, and space freed by the sweep is reused.
	if ok, _ := l.Allow("a"); ok {
		t.Error("a tracked key stays limited")
	}
}

func TestSweepMakesRoomAtTheCap(t *testing.T) {
	l, c := newLimiter(1, time.Minute)
	l.maxKeys = 2
	l.Allow("a")
	l.Allow("b")
	c.advance(2 * time.Minute) // both refilled, sweep interval passed
	l.Allow("c")               // the sweep frees a and b, so c is tracked
	if ok, _ := l.Allow("c"); ok {
		t.Error("c should have been tracked (and so be out of tokens)")
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

func TestConcurrentUseAllowsExactlyTheLimit(t *testing.T) {
	l, _ := newLimiter(100, time.Hour) // the clock is frozen, so nothing refills
	var allowed, denied atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 400; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if ok, _ := l.Allow("k"); ok {
				allowed.Add(1)
			} else {
				denied.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 100 || denied.Load() != 300 {
		t.Errorf("allowed %d, denied %d; want exactly 100 and 300", allowed.Load(), denied.Load())
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

func TestGateNeverAdmitsMoreThanTheCapConcurrently(t *testing.T) {
	g := NewGate(10)
	var admitted atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, ok := g.Acquire("u"); ok { // never released
				admitted.Add(1)
			}
		}()
	}
	wg.Wait()
	if admitted.Load() != 10 {
		t.Errorf("admitted %d holders, want exactly 10", admitted.Load())
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
