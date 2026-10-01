package keyedlock

import (
	"sync"
	"testing"
	"time"
)

func TestSameKeyIsMutuallyExclusive(t *testing.T) {
	var l Locks
	var inside, maxInside int
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			defer l.Lock("k")()
			mu.Lock()
			inside++
			if inside > maxInside {
				maxInside = inside
			}
			mu.Unlock()
			time.Sleep(time.Millisecond)
			mu.Lock()
			inside--
			mu.Unlock()
		}()
	}
	wg.Wait()
	if maxInside != 1 {
		t.Errorf("up to %d goroutines held the lock at once", maxInside)
	}
	if l.Len() != 0 {
		t.Errorf("idle lock entries should be removed, %d left", l.Len())
	}
}

func TestDifferentKeysDoNotBlockEachOther(t *testing.T) {
	var l Locks
	unlockA := l.Lock("a")
	done := make(chan struct{})
	go func() {
		defer l.Lock("b")()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("locking b blocked on a")
	}
	if l.Len() != 1 {
		t.Errorf("expected only a to remain, got %d", l.Len())
	}
	unlockA()
	if l.Len() != 0 {
		t.Errorf("expected no entries, got %d", l.Len())
	}
}
