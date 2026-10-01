// Package keyedlock serializes work per string key. It lets services make a
// read-then-write sequence (check for duplicates, then insert) atomic for one
// logical entity without a global lock. Locks are per process; cross-process
// safety still relies on database constraints.
package keyedlock

import "sync"

// Locks is a set of mutexes addressed by key. The zero value is ready to use.
// Entries are reference-counted and removed when idle.
type Locks struct {
	mu sync.Mutex
	m  map[string]*entry
}

type entry struct {
	mu   sync.Mutex
	refs int
}

// Lock blocks until key is free and returns the function that releases it:
//
//	defer locks.Lock("movie:603")()
func (k *Locks) Lock(key string) (unlock func()) {
	k.mu.Lock()
	if k.m == nil {
		k.m = make(map[string]*entry)
	}
	e, ok := k.m[key]
	if !ok {
		e = &entry{}
		k.m[key] = e
	}
	e.refs++
	k.mu.Unlock()

	e.mu.Lock()
	return func() {
		e.mu.Unlock()
		k.mu.Lock()
		if e.refs--; e.refs == 0 {
			delete(k.m, key)
		}
		k.mu.Unlock()
	}
}

// Len reports how many keys currently have a lock entry (held or awaited).
func (k *Locks) Len() int {
	k.mu.Lock()
	defer k.mu.Unlock()
	return len(k.m)
}
