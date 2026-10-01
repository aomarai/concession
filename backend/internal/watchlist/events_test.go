package watchlist

import (
	"errors"
	"sync"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/events"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/userref"
	"github.com/google/uuid"
)

type publishRecorder struct {
	mu  sync.Mutex
	got []events.Event
}

func (p *publishRecorder) Publish(ev events.Event) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.got = append(p.got, ev)
}

func (p *publishRecorder) types() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var out []string
	for _, ev := range p.got {
		out = append(out, ev.Type)
	}
	return out
}

func (p *publishRecorder) reset() { p.got = nil }

func same(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestChangesArePublishedWithTheActor(t *testing.T) {
	e := newEnv(t)
	pub := &publishRecorder{}
	e.svc.Events = pub
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")

	item, err := e.svc.AddItem(ctx, e.owner, list, 1, "")
	if err != nil {
		t.Fatal(err)
	}
	other, _ := e.svc.AddItem(ctx, e.owner, list, 2, "")
	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, list, item.ID, "n"); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Reorder(ctx, e.owner, list, []uuid.UUID{other.ID, item.ID}); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveItem(ctx, e.owner, list, other.ID); err != nil {
		t.Fatal(err)
	}
	title := "New"
	if _, err := e.svc.Update(ctx, e.owner, list, UpdateInput{Title: &title}); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: ann}, domain.RoleViewer); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.SetRole(ctx, e.owner, list, ann, domain.RoleEditor); err != nil {
		t.Fatal(err)
	}
	invites, _ := e.svc.ListInvites(ctx, ann)
	if err := e.svc.AcceptInvite(ctx, ann, invites[0].ID); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.RemoveMember(ctx, e.owner, list, ann); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Delete(ctx, e.owner, list); err != nil {
		t.Fatal(err)
	}

	want := []string{
		events.ItemAdded, events.ItemAdded, events.ItemUpdated, events.ItemsReordered, events.ItemRemoved,
		events.ListUpdated, events.MembersChanged, events.MembersChanged, events.MembersChanged,
		events.MembersChanged, events.ListDeleted,
	}
	if got := pub.types(); !same(got, want) {
		t.Fatalf("published %v, want %v", got, want)
	}
	for _, ev := range pub.got {
		if ev.ListID != list {
			t.Errorf("wrong list on %+v", ev)
		}
	}
	if ev := pub.got[0]; ev.ActorID != e.owner || ev.ItemID == nil || *ev.ItemID != item.ID {
		t.Errorf("item_added: %+v", ev)
	}
	if ev := pub.got[8]; ev.ActorID != ann { // the acceptance
		t.Errorf("acceptance actor: %+v", ev)
	}
}

func TestDeclineIsPublished(t *testing.T) {
	e := newEnv(t)
	pub := &publishRecorder{}
	e.svc.Events = pub
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")
	e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: ann}, domain.RoleViewer)
	invites, _ := e.svc.ListInvites(ctx, ann)
	pub.reset()
	if err := e.svc.DeclineInvite(ctx, ann, invites[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := pub.types(); !same(got, []string{events.MembersChanged}) || pub.got[0].ActorID != ann || pub.got[0].ListID != list {
		t.Errorf("published %+v", pub.got)
	}
}

func TestFailedChangesPublishNothing(t *testing.T) {
	e := newEnv(t)
	pub := &publishRecorder{}
	e.svc.Events = pub
	list := e.newList(t, domain.WatchlistTypeMovie)
	items := e.addMovies(t, list, 1, 2)
	ann := e.account(t, "ann")
	pub.reset()

	testutil.FailOn(t, e.db, "create", "watchlist_items")
	if _, err := e.svc.AddItem(ctx, e.owner, list, 3, ""); !errors.Is(err, testutil.ErrInjected) {
		t.Fatal(err)
	}
	testutil.FailOn(t, e.db, "update", "watchlist_items")
	if _, err := e.svc.UpdateItemNotes(ctx, e.owner, list, items[0], "x"); err == nil {
		t.Fatal("expected failure")
	}
	if err := e.svc.Reorder(ctx, e.owner, list, []uuid.UUID{items[1], items[0]}); err == nil {
		t.Fatal("expected failure")
	}
	testutil.FailOn(t, e.db, "delete", "watchlist_items")
	if err := e.svc.RemoveItem(ctx, e.owner, list, items[0]); err == nil {
		t.Fatal("expected failure")
	}
	testutil.FailOn(t, e.db, "create", "collaborators")
	if _, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: ann}, domain.RoleViewer); err == nil {
		t.Fatal("expected failure")
	}
	if got := pub.types(); len(got) != 0 {
		t.Errorf("failed operations must not announce anything, got %v", got)
	}
}
