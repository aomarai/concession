// Package events is a small in-process publish/subscribe hub used to push
// watchlist changes to connected clients (server-sent events). It is per
// process: with several server instances, a client only sees changes made
// through the instance it is connected to.
package events

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
)

// Event types published for a watchlist. Payloads are deliberately tiny: they
// say what changed so clients can refetch, and never carry list contents.
const (
	ListUpdated    = "list_updated"    // title, description or privacy changed
	ListDeleted    = "list_deleted"    // the list no longer exists
	ItemAdded      = "item_added"      // ItemID is the new item
	ItemUpdated    = "item_updated"    // an item's notes changed; ItemID is set
	ItemRemoved    = "item_removed"    // ItemID is the removed item
	ItemsReordered = "items_reordered" // the order of all items changed
	MembersChanged = "members_changed" // invitation, acceptance, role change or removal
)

// subscriberBuffer is how many events a subscriber can fall behind by before
// further events are dropped (and the subscriber is marked lagged).
const subscriberBuffer = 16

// Event is something that happened to a watchlist.
type Event struct {
	ID      uint64     `json:"-"`
	Type    string     `json:"type"`
	ListID  uuid.UUID  `json:"list_id"`
	ActorID uuid.UUID  `json:"actor_id"`
	ItemID  *uuid.UUID `json:"item_id,omitempty"`
	At      time.Time  `json:"at"`
}

// Hub fans events out to the subscribers of each watchlist. The zero value is
// not usable; create one with NewHub.
type Hub struct {
	mu     sync.Mutex
	subs   map[uuid.UUID]map[*Subscription]struct{}
	closed bool
	nextID atomic.Uint64
}

func NewHub() *Hub {
	return &Hub{subs: make(map[uuid.UUID]map[*Subscription]struct{})}
}

// Subscription receives the events of one watchlist on C.
type Subscription struct {
	C <-chan Event

	ch     chan Event
	list   uuid.UUID
	hub    *Hub
	lagged atomic.Bool
}

// Subscribe starts receiving events for a watchlist. On a closed hub it
// returns a subscription whose channel is already closed. Always call Close.
func (h *Hub) Subscribe(list uuid.UUID) *Subscription {
	ch := make(chan Event, subscriberBuffer)
	s := &Subscription{C: ch, ch: ch, list: list, hub: h}

	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		close(ch)
		return s
	}
	if h.subs[list] == nil {
		h.subs[list] = make(map[*Subscription]struct{})
	}
	h.subs[list][s] = struct{}{}
	return s
}

// Close stops the subscription and closes C. It is safe to call repeatedly.
func (s *Subscription) Close() {
	h := s.hub
	h.mu.Lock()
	defer h.mu.Unlock()
	if set, ok := h.subs[s.list]; ok {
		if _, found := set[s]; found {
			delete(set, s)
			close(s.ch)
			if len(set) == 0 {
				delete(h.subs, s.list)
			}
		}
	}
}

// Lagged reports, once, whether events were dropped because the subscriber
// was too slow. The client should refetch the list when it is true.
func (s *Subscription) Lagged() bool { return s.lagged.Swap(false) }

// Publish delivers an event to the list's subscribers without ever blocking:
// a subscriber whose buffer is full misses the event and is marked lagged.
func (h *Hub) Publish(ev Event) {
	if ev.At.IsZero() {
		ev.At = time.Now().UTC()
	}
	ev.ID = h.nextID.Add(1)

	h.mu.Lock()
	defer h.mu.Unlock()
	for s := range h.subs[ev.ListID] {
		select {
		case s.ch <- ev:
		default:
			s.lagged.Store(true)
		}
	}
}

// Subscribers returns how many clients are subscribed to a list.
func (h *Hub) Subscribers(list uuid.UUID) int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.subs[list])
}

// Close ends every subscription (their channels are closed) and makes later
// Subscribe calls return closed subscriptions. Safe to call repeatedly; used
// at shutdown so open streams do not hold up the server.
func (h *Hub) Close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed {
		return
	}
	h.closed = true
	for list, set := range h.subs {
		for s := range set {
			close(s.ch)
		}
		delete(h.subs, list)
	}
}
