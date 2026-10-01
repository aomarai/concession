package notifications

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/paging"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var ctx = context.Background()

func setup(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := testutil.NewDB(t, &domain.User{}, &domain.Notification{})
	return NewService(db), db
}

func user(t *testing.T, db *gorm.DB, name string) uuid.UUID {
	t.Helper()
	u := domain.User{Username: name, Email: name + "@example.com", DisplayName: strings.ToUpper(name[:1]) + name[1:], AvatarURL: "http://pic/" + name}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestNotifyMessages(t *testing.T) {
	s, db := setup(t)
	ann, bob := user(t, db, "ann"), user(t, db, "bob")

	for typ, want := range map[domain.NotificationType]string{
		domain.NotificationWatchlistInvite: `Ann invited you to collaborate on "Friday"`,
		domain.NotificationInviteAccepted:  `Ann accepted your invitation to "Friday"`,
		domain.NotificationItemAdded:       `Ann added a title to "Friday"`,
		domain.NotificationFriendRequest:   `Ann sent you a friend request`,
		domain.NotificationFriendAccepted:  `Ann accepted your friend request`,
	} {
		if err := s.Notify(ctx, bob, ann, typ, "Friday", "/somewhere"); err != nil {
			t.Fatalf("%s: %v", typ, err)
		}
		var n domain.Notification
		if err := db.Where("type = ?", typ).First(&n).Error; err != nil {
			t.Fatal(err)
		}
		if n.Message != want || n.UserID != bob || n.ActorID != ann || n.IsRead || n.LinkURL != "/somewhere" {
			t.Errorf("%s: %+v", typ, n)
		}
	}
}

func TestNotifyEdgeCases(t *testing.T) {
	s, db := setup(t)
	ann, bob := user(t, db, "ann"), user(t, db, "bob")

	if err := s.Notify(ctx, ann, ann, domain.NotificationItemAdded, "x", ""); err != nil {
		t.Errorf("self-notification should be a silent no-op: %v", err)
	}
	var n int64
	db.Model(&domain.Notification{}).Count(&n)
	if n != 0 {
		t.Error("nothing should have been stored")
	}
	if err := s.Notify(ctx, bob, ann, "made_up", "x", ""); err == nil {
		t.Error("an unknown type must be rejected")
	}
	// An actor whose account is gone is described generically.
	if err := s.Notify(ctx, bob, uuid.New(), domain.NotificationFriendRequest, "", ""); err != nil {
		t.Fatal(err)
	}
	var got domain.Notification
	db.First(&got)
	if got.Message != "Someone sent you a friend request" {
		t.Errorf("message = %q", got.Message)
	}

	for _, tc := range []struct{ op, table string }{{"query", "users"}, {"create", "notifications"}} {
		s2, db2 := setup(t)
		a, b := user(t, db2, "ann"), user(t, db2, "bob")
		testutil.FailOn(t, db2, tc.op, tc.table)
		if err := s2.Notify(ctx, b, a, domain.NotificationItemAdded, "x", ""); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("%s: %v", tc.table, err)
		}
	}
}

func seed(t *testing.T, s *Service, to, from uuid.UUID, count int) {
	t.Helper()
	for i := 0; i < count; i++ {
		if err := s.Notify(ctx, to, from, domain.NotificationItemAdded, "L", "/l"); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func TestList(t *testing.T) {
	s, db := setup(t)
	ann, bob, cy := user(t, db, "ann"), user(t, db, "bob"), user(t, db, "cy")
	seed(t, s, bob, ann, 3)
	seed(t, s, bob, cy, 1)
	seed(t, s, ann, bob, 2) // someone else's notifications never show up

	p, err := s.List(ctx, bob, false, 0, 0)
	if err != nil || p.Total != 4 || p.UnreadCount != 4 || len(p.Notifications) != 4 || p.Page != 1 || p.PerPage != paging.DefaultPerPage {
		t.Fatalf("got %+v, %v", p, err)
	}
	first := p.Notifications[0]
	if first.Actor.DisplayName != "Cy" || first.Actor.AvatarURL != "http://pic/cy" || first.IsRead || first.LinkURL != "/l" || first.ID == uuid.Nil {
		t.Errorf("newest first with the actor's public profile: %+v", first)
	}

	p, _ = s.List(ctx, bob, false, 2, 3)
	if len(p.Notifications) != 1 || p.Total != 4 {
		t.Errorf("page 2: %+v", p)
	}
	if err := s.MarkRead(ctx, bob, first.ID); err != nil {
		t.Fatal(err)
	}
	p, _ = s.List(ctx, bob, true, 1, 10)
	if p.Total != 3 || p.UnreadCount != 3 || len(p.Notifications) != 3 {
		t.Errorf("unread only: %+v", p)
	}
	p, _ = s.List(ctx, bob, false, 1, 10)
	if p.Total != 4 || p.UnreadCount != 3 {
		t.Errorf("all, with one read: %+v", p)
	}
	empty, err := s.List(ctx, uuid.New(), false, 1, 10)
	if err != nil || empty.Total != 0 || empty.Notifications == nil || len(empty.Notifications) != 0 {
		t.Errorf("none: %+v, %v", empty, err)
	}
	if _, err := s.List(ctx, bob, false, -1, 10); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("bad page: %v", err)
	}
}

func TestListFailures(t *testing.T) {
	for _, tc := range []struct {
		name, op, table string
		after           int
	}{
		{"count", "query", "notifications", 0},
		{"unread count", "query", "notifications", 1},
		{"page", "query", "notifications", 2},
		{"actors", "query", "users", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, db := setup(t)
			ann, bob := user(t, db, "ann"), user(t, db, "bob")
			seed(t, s, bob, ann, 1)
			testutil.FailAfter(t, db, tc.op, tc.table, tc.after)
			if _, err := s.List(ctx, bob, false, 1, 10); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func TestMarkRead(t *testing.T) {
	s, db := setup(t)
	ann, bob := user(t, db, "ann"), user(t, db, "bob")
	seed(t, s, bob, ann, 2)
	seed(t, s, ann, bob, 1)
	p, _ := s.List(ctx, bob, false, 1, 10)
	id := p.Notifications[0].ID

	if err := s.MarkRead(ctx, ann, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("someone else's notification: %v", err)
	}
	if err := s.MarkRead(ctx, bob, uuid.New()); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown: %v", err)
	}
	if err := s.MarkRead(ctx, bob, id); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkRead(ctx, bob, id); err != nil {
		t.Errorf("marking twice is harmless: %v", err)
	}
	if n, _ := s.UnreadCount(ctx, bob); n != 1 {
		t.Errorf("unread = %d", n)
	}
	if n, _ := s.UnreadCount(ctx, ann); n != 1 {
		t.Errorf("another user's unread count changed: %d", n)
	}

	testutil.FailOn(t, db, "update", "notifications")
	if err := s.MarkRead(ctx, bob, id); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("db failure: %v", err)
	}
}

func TestMarkAllRead(t *testing.T) {
	s, db := setup(t)
	ann, bob := user(t, db, "ann"), user(t, db, "bob")
	seed(t, s, bob, ann, 3)
	seed(t, s, ann, bob, 1)

	n, err := s.MarkAllRead(ctx, bob)
	if err != nil || n != 3 {
		t.Fatalf("changed %d, %v", n, err)
	}
	if n, _ := s.MarkAllRead(ctx, bob); n != 0 {
		t.Errorf("nothing left to mark, got %d", n)
	}
	if c, _ := s.UnreadCount(ctx, ann); c != 1 {
		t.Errorf("another user's notifications must be untouched: %d", c)
	}
	testutil.FailOn(t, db, "update", "notifications")
	if _, err := s.MarkAllRead(ctx, ann); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("db failure: %v", err)
	}
	testutil.FailOn(t, db, "query", "notifications")
	if _, err := s.UnreadCount(ctx, bob); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("unread count failure: %v", err)
	}
}
