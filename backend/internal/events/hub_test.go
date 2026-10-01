package events

import (
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func recv(t *testing.T, s *Subscription) Event {
	t.Helper()
	select {
	case ev, ok := <-s.C:
		if !ok {
			t.Fatal("channel closed")
		}
		return ev
	case <-time.After(2 * time.Second):
		t.Fatal("no event")
		return Event{}
	}
}

func TestPublishReachesOnlyThatListsSubscribers(t *testing.T) {
	h := NewHub()
	a, b := uuid.New(), uuid.New()
	s1, s2, other := h.Subscribe(a), h.Subscribe(a), h.Subscribe(b)
	defer s1.Close()
	defer s2.Close()
	defer other.Close()

	actor := uuid.New()
	item := uuid.New()
	h.Publish(Event{Type: ItemAdded, ListID: a, ActorID: actor, ItemID: &item})

	for _, s := range []*Subscription{s1, s2} {
		ev := recv(t, s)
		if ev.Type != ItemAdded || ev.ListID != a || ev.ActorID != actor || *ev.ItemID != item || ev.At.IsZero() || ev.ID == 0 {
			t.Errorf("unexpected event %+v", ev)
		}
	}
	select {
	case ev := <-other.C:
		t.Errorf("another list's subscriber got %+v", ev)
	default:
	}
	if h.Subscribers(a) != 2 || h.Subscribers(b) != 1 || h.Subscribers(uuid.New()) != 0 {
		t.Errorf("subscriber counts: %d %d", h.Subscribers(a), h.Subscribers(b))
	}
}

func TestEventIDsIncrease(t *testing.T) {
	h := NewHub()
	list := uuid.New()
	s := h.Subscribe(list)
	defer s.Close()
	h.Publish(Event{Type: ListUpdated, ListID: list})
	h.Publish(Event{Type: ListUpdated, ListID: list})
	first, second := recv(t, s), recv(t, s)
	if second.ID <= first.ID {
		t.Errorf("ids %d then %d", first.ID, second.ID)
	}
	fixed := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	h.Publish(Event{Type: ListUpdated, ListID: list, At: fixed})
	if got := recv(t, s); !got.At.Equal(fixed) {
		t.Errorf("an explicit time is kept, got %v", got.At)
	}
}

func TestCloseUnsubscribesAndIsIdempotent(t *testing.T) {
	h := NewHub()
	list := uuid.New()
	s := h.Subscribe(list)
	s.Close()
	s.Close()
	if _, ok := <-s.C; ok {
		t.Error("channel should be closed")
	}
	if h.Subscribers(list) != 0 {
		t.Error("subscription not removed")
	}
	h.Publish(Event{Type: ItemAdded, ListID: list}) // no subscribers: must not panic

	// Closing one of two leaves the other working.
	s1, s2 := h.Subscribe(list), h.Subscribe(list)
	s1.Close()
	h.Publish(Event{Type: ItemAdded, ListID: list})
	if recv(t, s2).Type != ItemAdded || h.Subscribers(list) != 1 {
		t.Error("the remaining subscriber should still receive events")
	}
	s2.Close()
}

func TestSlowSubscribersAreMarkedLaggedNotBlocking(t *testing.T) {
	h := NewHub()
	list := uuid.New()
	s := h.Subscribe(list)
	defer s.Close()

	done := make(chan struct{})
	go func() {
		for i := 0; i < subscriberBuffer*3; i++ {
			h.Publish(Event{Type: ItemAdded, ListID: list})
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Publish blocked on a slow subscriber")
	}
	if !s.Lagged() {
		t.Error("dropped events must be reported")
	}
	if s.Lagged() {
		t.Error("Lagged reports each drop once")
	}
	for i := 0; i < subscriberBuffer; i++ {
		recv(t, s) // the buffered events are still delivered
	}
}

func TestHubCloseEndsEverything(t *testing.T) {
	h := NewHub()
	list := uuid.New()
	s := h.Subscribe(list)
	h.Close()
	h.Close()
	if _, ok := <-s.C; ok {
		t.Error("open subscriptions are closed with the hub")
	}
	s.Close() // closing after the hub closed is harmless

	late := h.Subscribe(list)
	if _, ok := <-late.C; ok {
		t.Error("subscribing to a closed hub gives a closed channel")
	}
	late.Close()
	h.Publish(Event{ListID: list}) // must not panic
	if h.Subscribers(list) != 0 {
		t.Error("closed hub has no subscribers")
	}
}

func TestConcurrentUseIsRaceFree(t *testing.T) {
	h := NewHub()
	list := uuid.New()
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				s := h.Subscribe(list)
				select {
				case <-s.C:
				default:
				}
				s.Close()
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				h.Publish(Event{Type: ItemAdded, ListID: list})
			}
		}()
	}
	wg.Wait()
	h.Close()
}
