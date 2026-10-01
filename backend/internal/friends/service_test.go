package friends

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/userref"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ctx = context.Background()

type sent struct {
	to, actor uuid.UUID
	typ       domain.NotificationType
}

type recorder struct {
	mu   sync.Mutex
	got  []sent
	fail error
}

func (r *recorder) Notify(_ context.Context, to, actor uuid.UUID, typ domain.NotificationType, _, _ string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.got = append(r.got, sent{to, actor, typ})
	return r.fail
}

type env struct {
	svc *Service
	db  *gorm.DB
	rec *recorder
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.NewDB(t, &domain.User{}, &domain.Friendship{})
	rec := &recorder{}
	s := NewService(db)
	s.Notifier = rec
	return &env{svc: s, db: db, rec: rec}
}

func (e *env) user(t *testing.T, name string) uuid.UUID {
	t.Helper()
	u := domain.User{Username: name, Email: name + "@example.com", DisplayName: strings.ToUpper(name[:1]) + name[1:]}
	if err := e.db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func byName(n string) userref.Ref { return userref.Ref{Identifier: n} }

func TestSendAndAcceptFlow(t *testing.T) {
	e := newEnv(t)
	ann, bob := e.user(t, "ann"), e.user(t, "bob")

	entry, err := e.svc.Send(ctx, ann, byName("bob"))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != domain.FriendshipPending || entry.User.ID != bob || entry.User.DisplayName != "Bob" || entry.AcceptedAt != nil {
		t.Errorf("sent: %+v", entry)
	}
	if len(e.rec.got) != 1 || e.rec.got[0] != (sent{bob, ann, domain.NotificationFriendRequest}) {
		t.Errorf("notifications: %+v", e.rec.got)
	}

	reqs, _ := e.svc.ListRequests(ctx, bob)
	if len(reqs.Incoming) != 1 || len(reqs.Outgoing) != 0 || reqs.Incoming[0].User.ID != ann {
		t.Fatalf("bob's requests: %+v", reqs)
	}
	reqs, _ = e.svc.ListRequests(ctx, ann)
	if len(reqs.Outgoing) != 1 || len(reqs.Incoming) != 0 || reqs.Outgoing[0].User.ID != bob {
		t.Errorf("ann's requests: %+v", reqs)
	}
	if friends, _ := e.svc.List(ctx, ann); len(friends) != 0 {
		t.Errorf("a pending request is not a friendship: %+v", friends)
	}

	// Only the addressee can accept.
	id := reqs.Outgoing[0].ID
	if err := e.svc.Accept(ctx, ann, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("the sender accepting their own request: %v", err)
	}
	if err := e.svc.Accept(ctx, e.user(t, "cy"), id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("a third party accepting: %v", err)
	}
	if err := e.svc.Accept(ctx, bob, id); err != nil {
		t.Fatal(err)
	}
	if last := e.rec.got[len(e.rec.got)-1]; last != (sent{ann, bob, domain.NotificationFriendAccepted}) {
		t.Errorf("accept notification: %+v", last)
	}
	for _, me := range []uuid.UUID{ann, bob} {
		friends, err := e.svc.List(ctx, me)
		if err != nil || len(friends) != 1 || friends[0].Status != domain.FriendshipAccepted || friends[0].AcceptedAt == nil {
			t.Errorf("friends of %v: %+v, %v", me, friends, err)
		}
	}
	if err := e.svc.Accept(ctx, bob, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("accepting twice: %v", err)
	}
}

func TestSendValidation(t *testing.T) {
	e := newEnv(t)
	ann, bob := e.user(t, "ann"), e.user(t, "bob")

	if _, err := e.svc.Send(ctx, ann, byName("ann")); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("befriending yourself: %v", err)
	}
	if _, err := e.svc.Send(ctx, ann, userref.Ref{ID: ann}); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("befriending yourself by id: %v", err)
	}
	if _, err := e.svc.Send(ctx, ann, byName("nobody")); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
	if _, err := e.svc.Send(ctx, ann, userref.Ref{}); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("no user given: %v", err)
	}
	if _, err := e.svc.Send(ctx, ann, userref.Ref{ID: bob}); err != nil {
		t.Errorf("by id: %v", err)
	}
	if _, err := e.svc.Send(ctx, ann, byName("bob")); !errors.Is(err, svcerr.ErrDuplicate) || !strings.Contains(svcerr.MessageOr(err, ""), "already sent") {
		t.Errorf("sending twice: %v", err)
	}
	reqs, _ := e.svc.ListRequests(ctx, ann)
	if err := e.svc.Accept(ctx, bob, reqs.Outgoing[0].ID); err != nil {
		t.Fatal(err)
	}
	for _, from := range []uuid.UUID{ann, bob} {
		other := "bob"
		if from == bob {
			other = "ann"
		}
		if _, err := e.svc.Send(ctx, from, byName(other)); !errors.Is(err, svcerr.ErrDuplicate) || !strings.Contains(svcerr.MessageOr(err, ""), "already friends") {
			t.Errorf("requesting an existing friend: %v", err)
		}
	}
}

func TestMutualRequestsBecomeAFriendship(t *testing.T) {
	e := newEnv(t)
	ann, bob := e.user(t, "ann"), e.user(t, "bob")
	if _, err := e.svc.Send(ctx, ann, byName("bob")); err != nil {
		t.Fatal(err)
	}
	entry, err := e.svc.Send(ctx, bob, byName("ann"))
	if err != nil {
		t.Fatal(err)
	}
	if entry.Status != domain.FriendshipAccepted || entry.User.ID != ann || entry.AcceptedAt == nil {
		t.Errorf("a reverse request should accept: %+v", entry)
	}
	if last := e.rec.got[len(e.rec.got)-1]; last != (sent{ann, bob, domain.NotificationFriendAccepted}) {
		t.Errorf("notification: %+v", last)
	}
	var n int64
	e.db.Model(&domain.Friendship{}).Count(&n)
	if n != 1 {
		t.Errorf("exactly one row per pair, got %d", n)
	}
}

func TestConcurrentRequestsCreateOneRow(t *testing.T) {
	db := testutil.NewFileDB(t, &domain.User{}, &domain.Friendship{})
	s := NewService(db)
	var ids []uuid.UUID
	for _, n := range []string{"ann", "bob"} {
		u := domain.User{Username: n, Email: n + "@x.com", DisplayName: n}
		if err := db.Create(&u).Error; err != nil {
			t.Fatal(err)
		}
		ids = append(ids, u.ID)
	}
	var wg sync.WaitGroup
	results := make(chan error, 12)
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			from, to := ids[0], ids[1]
			if i%2 == 1 { // half of them go the other way
				from, to = to, from
			}
			_, err := s.Send(ctx, from, userref.Ref{ID: to})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil && !errors.Is(err, svcerr.ErrDuplicate) {
			t.Errorf("unexpected error: %v", err)
		}
	}
	var n int64
	db.Model(&domain.Friendship{}).Count(&n)
	if n != 1 {
		t.Errorf("expected one friendship row, got %d", n)
	}
	if s.locks.Len() != 0 {
		t.Errorf("locks leaked: %d", s.locks.Len())
	}
}

func TestDeclineRemoveAndCancel(t *testing.T) {
	e := newEnv(t)
	ann, bob, cy := e.user(t, "ann"), e.user(t, "bob"), e.user(t, "cy")

	// Decline.
	e.svc.Send(ctx, ann, byName("bob"))
	reqs, _ := e.svc.ListRequests(ctx, bob)
	id := reqs.Incoming[0].ID
	if err := e.svc.Decline(ctx, ann, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("sender declining: %v", err)
	}
	if err := e.svc.Decline(ctx, bob, id); err != nil {
		t.Fatal(err)
	}
	if err := e.svc.Decline(ctx, bob, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("declining twice: %v", err)
	}
	// A declined request can be sent again.
	if _, err := e.svc.Send(ctx, ann, byName("bob")); err != nil {
		t.Errorf("re-request after decline: %v", err)
	}

	// The sender cancels their request.
	if err := e.svc.Remove(ctx, ann, bob); err != nil {
		t.Errorf("cancel: %v", err)
	}
	if reqs, _ := e.svc.ListRequests(ctx, bob); len(reqs.Incoming) != 0 {
		t.Errorf("cancelled request lingers: %+v", reqs)
	}
	if err := e.svc.Remove(ctx, ann, bob); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("nothing left to remove: %v", err)
	}

	// Unfriend, from either side.
	for _, unfriender := range []uuid.UUID{ann, bob} {
		e.svc.Send(ctx, ann, byName("bob"))
		reqs, _ := e.svc.ListRequests(ctx, bob)
		e.svc.Accept(ctx, bob, reqs.Incoming[0].ID)
		other := bob
		if unfriender == bob {
			other = ann
		}
		if err := e.svc.Remove(ctx, unfriender, other); err != nil {
			t.Fatal(err)
		}
		if friends, _ := e.svc.List(ctx, ann); len(friends) != 0 {
			t.Errorf("still friends after unfriending: %+v", friends)
		}
	}
	if err := e.svc.Remove(ctx, ann, cy); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("never friends: %v", err)
	}
}

func TestListOrderAndDeletedAccounts(t *testing.T) {
	e := newEnv(t)
	ann := e.user(t, "ann")
	for _, n := range []string{"bob", "cy"} {
		other := e.user(t, n)
		e.svc.Send(ctx, ann, byName(n))
		reqs, _ := e.svc.ListRequests(ctx, other)
		if err := e.svc.Accept(ctx, other, reqs.Incoming[0].ID); err != nil {
			t.Fatal(err)
		}
		time.Sleep(3 * time.Millisecond)
	}
	friends, err := e.svc.List(ctx, ann)
	if err != nil || len(friends) != 2 || friends[0].User.DisplayName != "Cy" || friends[1].User.DisplayName != "Bob" {
		t.Fatalf("most recent first: %+v, %v", friends, err)
	}
	// An account that no longer exists shows as a bare ID instead of breaking the list.
	var cy domain.User
	e.db.Where("username = ?", "cy").First(&cy)
	e.db.Unscoped().Delete(&domain.User{}, cy.ID)
	friends, _ = e.svc.List(ctx, ann)
	if len(friends) != 2 || friends[0].User.ID != cy.ID || friends[0].User.DisplayName != "" {
		t.Errorf("deleted friend: %+v", friends)
	}
}

func TestNotificationFailuresDoNotFailTheAction(t *testing.T) {
	e := newEnv(t)
	e.rec.fail = errors.New("notifier down")
	ann, bob := e.user(t, "ann"), e.user(t, "bob")
	if _, err := e.svc.Send(ctx, ann, byName("bob")); err != nil {
		t.Errorf("send: %v", err)
	}
	reqs, _ := e.svc.ListRequests(ctx, bob)
	if err := e.svc.Accept(ctx, bob, reqs.Incoming[0].ID); err != nil {
		t.Errorf("accept: %v", err)
	}

	// And without any notifier at all.
	e2 := newEnv(t)
	e2.svc.Notifier = nil
	a, b := e2.user(t, "ann"), e2.user(t, "bob")
	if _, err := e2.svc.Send(ctx, a, byName("bob")); err != nil {
		t.Errorf("no notifier: %v", err)
	}
	reqs, _ = e2.svc.ListRequests(ctx, b)
	if err := e2.svc.Accept(ctx, b, reqs.Incoming[0].ID); err != nil {
		t.Errorf("no notifier: %v", err)
	}
}

func TestDatabaseFailures(t *testing.T) {
	type setup func(e *env, ann, bob uuid.UUID)
	pending := func(e *env, ann, bob uuid.UUID) { e.svc.Send(ctx, ann, byName("bob")) }
	accepted := func(e *env, ann, bob uuid.UUID) {
		e.svc.Send(ctx, ann, byName("bob"))
		reqs, _ := e.svc.ListRequests(ctx, bob)
		e.svc.Accept(ctx, bob, reqs.Incoming[0].ID)
	}
	requestID := func(e *env, bob uuid.UUID) uuid.UUID {
		var f domain.Friendship
		e.db.First(&f)
		return f.ID
	}
	for _, tc := range []struct {
		name, op, table string
		after           int
		seed            setup
		call            func(e *env, ann, bob uuid.UUID) error
	}{
		{"send: resolve user", "query", "users", 0, nil, func(e *env, a, b uuid.UUID) error { _, err := e.svc.Send(ctx, a, byName("bob")); return err }},
		{"send: lookup", "query", "friendships", 0, nil, func(e *env, a, b uuid.UUID) error { _, err := e.svc.Send(ctx, a, byName("bob")); return err }},
		{"send: insert", "create", "friendships", 0, nil, func(e *env, a, b uuid.UUID) error { _, err := e.svc.Send(ctx, a, byName("bob")); return err }},
		{"send: profile", "query", "users", 1, nil, func(e *env, a, b uuid.UUID) error { _, err := e.svc.Send(ctx, a, byName("bob")); return err }},
		{"send: auto-accept", "update", "friendships", 0, pending, func(e *env, a, b uuid.UUID) error { _, err := e.svc.Send(ctx, b, byName("ann")); return err }},
		{"accept: lookup", "query", "friendships", 0, pending, func(e *env, a, b uuid.UUID) error { return e.svc.Accept(ctx, b, uuid.New()) }},
		{"accept: update", "update", "friendships", 0, pending, func(e *env, a, b uuid.UUID) error { return e.svc.Accept(ctx, b, requestID(e, b)) }},
		{"decline: delete", "delete", "friendships", 0, pending, func(e *env, a, b uuid.UUID) error { return e.svc.Decline(ctx, b, requestID(e, b)) }},
		{"remove", "delete", "friendships", 0, accepted, func(e *env, a, b uuid.UUID) error { return e.svc.Remove(ctx, a, b) }},
		{"list: rows", "query", "friendships", 0, accepted, func(e *env, a, b uuid.UUID) error { _, err := e.svc.List(ctx, a); return err }},
		{"list: profiles", "query", "users", 0, accepted, func(e *env, a, b uuid.UUID) error { _, err := e.svc.List(ctx, a); return err }},
		{"requests: rows", "query", "friendships", 0, pending, func(e *env, a, b uuid.UUID) error { _, err := e.svc.ListRequests(ctx, a); return err }},
		{"requests: profiles", "query", "users", 0, pending, func(e *env, a, b uuid.UUID) error { _, err := e.svc.ListRequests(ctx, a); return err }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := newEnv(t)
			ann, bob := e.user(t, "ann"), e.user(t, "bob")
			if tc.seed != nil {
				tc.seed(e, ann, bob)
			}
			testutil.FailAfter(t, e.db, tc.op, tc.table, tc.after)
			if err := tc.call(e, ann, bob); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}
