package watchlist

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/userref"
	"github.com/google/uuid"
)

type note struct {
	to, actor uuid.UUID
	typ       domain.NotificationType
	subject   string
	link      string
}

type recorder struct {
	mu   sync.Mutex
	got  []note
	fail error
}

func (r *recorder) Notify(_ context.Context, to, actor uuid.UUID, typ domain.NotificationType, subject, link string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, note{to, actor, typ, subject, link})
	return r.fail
}

func (r *recorder) of(typ domain.NotificationType) []note {
	var out []note
	for _, n := range r.got {
		if n.typ == typ {
			out = append(out, n)
		}
	}
	return out
}

func withRecorder(e *env) *recorder {
	r := &recorder{}
	e.svc.Notifier = r
	return r
}

func TestInviteAndAcceptSendNotifications(t *testing.T) {
	e := newEnv(t)
	rec := withRecorder(e)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")

	if _, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{Identifier: "ann"}, domain.RoleEditor); err != nil {
		t.Fatal(err)
	}
	if got := rec.of(domain.NotificationWatchlistInvite); len(got) != 1 || got[0] != (note{ann, e.owner, domain.NotificationWatchlistInvite, "List", "/invites"}) {
		t.Errorf("invite notification: %+v", got)
	}

	invites, _ := e.svc.ListInvites(ctx, ann)
	if err := e.svc.AcceptInvite(ctx, ann, invites[0].ID); err != nil {
		t.Fatal(err)
	}
	want := note{e.owner, ann, domain.NotificationInviteAccepted, "List", "/watchlists/" + list.String()}
	if got := rec.of(domain.NotificationInviteAccepted); len(got) != 1 || got[0] != want {
		t.Errorf("accept notification: %+v", got)
	}
}

func TestInviteByUserID(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")
	if m, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: ann}, domain.RoleViewer); err != nil || m.ID != ann {
		t.Errorf("by id: %+v, %v", m, err)
	}
	if _, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: uuid.New()}, domain.RoleViewer); err == nil {
		t.Error("unknown id should fail")
	}
}

func TestAddItemNotifiesTheOtherMembers(t *testing.T) {
	e := newEnv(t)
	rec := withRecorder(e)
	list := e.newList(t, domain.WatchlistTypeMovie)
	editor := e.member(t, list, domain.RoleEditor)
	viewer := e.member(t, list, domain.RoleViewer)
	pending := e.account(t, "pat")
	if _, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: pending}, domain.RoleViewer); err != nil {
		t.Fatal(err)
	}
	rec.got = nil

	if _, err := e.svc.AddItem(ctx, editor, list, 1, ""); err != nil {
		t.Fatal(err)
	}
	got := map[uuid.UUID]note{}
	for _, n := range rec.of(domain.NotificationItemAdded) {
		got[n.to] = n
	}
	if len(got) != 2 || got[e.owner].actor != editor || got[viewer].subject != "List" || got[viewer].link != "/watchlists/"+list.String() {
		t.Errorf("owner and viewer should be told, the editor (actor) and pending invitee not: %+v", got)
	}
	if _, ok := got[editor]; ok {
		t.Error("the actor must not be notified about their own action")
	}
	if _, ok := got[pending]; ok {
		t.Error("a pending invitee has no access and is not notified")
	}

	rec.got = nil
	if _, err := e.svc.AddItem(ctx, e.owner, list, 2, ""); err != nil {
		t.Fatal(err)
	}
	if n := len(rec.of(domain.NotificationItemAdded)); n != 2 {
		t.Errorf("the owner adding should notify both collaborators, got %d", n)
	}
}

func TestNotificationFailuresNeverFailTheAction(t *testing.T) {
	e := newEnv(t)
	rec := withRecorder(e)
	rec.fail = errors.New("notifier down")
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")
	e.member(t, list, domain.RoleEditor)

	if _, err := e.svc.InviteUser(ctx, e.owner, list, userref.Ref{ID: ann}, domain.RoleViewer); err != nil {
		t.Errorf("invite: %v", err)
	}
	invites, _ := e.svc.ListInvites(ctx, ann)
	if err := e.svc.AcceptInvite(ctx, ann, invites[0].ID); err != nil {
		t.Errorf("accept: %v", err)
	}
	if _, err := e.svc.AddItem(ctx, e.owner, list, 1, ""); err != nil {
		t.Errorf("add item: %v", err)
	}
}

func TestLookupFailureWhileNotifyingDoesNotFailAddItem(t *testing.T) {
	e := newEnv(t)
	withRecorder(e)
	list := e.newList(t, domain.WatchlistTypeMovie)
	// For the owner the only collaborators query is the recipients lookup.
	testutil.FailOn(t, e.db, "query", "collaborators")
	if _, err := e.svc.AddItem(ctx, e.owner, list, 1, ""); err != nil {
		t.Errorf("add item must succeed even when recipients cannot be looked up: %v", err)
	}
}

func TestAcceptWithoutKnownInviterOrTitle(t *testing.T) {
	e := newEnv(t)
	rec := withRecorder(e)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")
	if err := e.db.Create(&domain.Collaborator{UserID: ann, WatchlistID: list, Role: domain.RoleViewer, Status: domain.CollaboratorPending}).Error; err != nil {
		t.Fatal(err)
	}
	invites, _ := e.svc.ListInvites(ctx, ann)
	if err := e.svc.AcceptInvite(ctx, ann, invites[0].ID); err != nil {
		t.Fatal(err)
	}
	if got := rec.of(domain.NotificationInviteAccepted); len(got) != 0 {
		t.Errorf("no inviter is recorded, so nobody is notified: %+v", got)
	}

	// With an inviter but a list that can no longer be read, the message just lacks the title.
	bob := e.account(t, "bob")
	owner := e.owner
	if err := e.db.Create(&domain.Collaborator{UserID: bob, WatchlistID: list, Role: domain.RoleViewer, Status: domain.CollaboratorPending, InvitedBy: &owner}).Error; err != nil {
		t.Fatal(err)
	}
	invites, _ = e.svc.ListInvites(ctx, bob)
	testutil.FailOn(t, e.db, "query", "watchlists")
	if err := e.svc.AcceptInvite(ctx, bob, invites[0].ID); err != nil {
		t.Fatalf("accept must not depend on the title lookup: %v", err)
	}
	if got := rec.of(domain.NotificationInviteAccepted); len(got) != 1 || got[0].subject != "" {
		t.Errorf("notification without a title: %+v", got)
	}
}
