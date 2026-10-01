package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/friends"
	"github.com/aomarai/concession/internal/notifications"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

type friendsBody struct {
	Friends []friends.Entry `json:"friends"`
}

func (a *api) befriend(from, to uuid.UUID, toName string) {
	a.t.Helper()
	a.expect(a.do(from, http.MethodPost, "/friends", map[string]any{"user": toName}), http.StatusCreated, "")
	reqs := decode[friends.Requests](a.t, a.do(to, http.MethodGet, "/friends/requests", nil))
	a.expect(a.do(to, http.MethodPost, "/friends/requests/"+reqs.Incoming[0].ID.String()+"/accept", nil), http.StatusNoContent, "")
}

func TestFriendsFlow(t *testing.T) {
	a := newAPI(t)
	ann, bob := a.newUser("ann"), a.newUser("bob")

	w := a.do(ann, http.MethodPost, "/friends", map[string]any{"user": "bob@example.com"})
	a.expect(w, http.StatusCreated, "")
	if e := decode[friends.Entry](t, w); e.Status != domain.FriendshipPending || e.User.ID != bob || strings.Contains(w.Body.String(), "@") {
		t.Errorf("request: %s", w.Body)
	}
	a.expect(a.do(ann, http.MethodPost, "/friends", map[string]any{"user_id": bob}), http.StatusConflict, "conflict")

	reqs := decode[friends.Requests](t, a.do(bob, http.MethodGet, "/friends/requests", nil))
	if len(reqs.Incoming) != 1 || reqs.Incoming[0].User.ID != ann || len(reqs.Outgoing) != 0 {
		t.Fatalf("incoming: %+v", reqs)
	}
	// Bob is told about the request.
	n := decode[notifications.Page](t, a.do(bob, http.MethodGet, "/me/notifications", nil))
	if n.Total != 1 || n.Notifications[0].Message != "Ann sent you a friend request" || n.Notifications[0].Type != domain.NotificationFriendRequest {
		t.Errorf("notification: %+v", n)
	}

	a.expect(a.do(ann, http.MethodPost, "/friends/requests/"+reqs.Incoming[0].ID.String()+"/accept", nil), http.StatusNotFound, "not_found")
	a.expect(a.do(bob, http.MethodPost, "/friends/requests/"+reqs.Incoming[0].ID.String()+"/accept", nil), http.StatusNoContent, "")
	for _, u := range []uuid.UUID{ann, bob} {
		if fl := decode[friendsBody](t, a.do(u, http.MethodGet, "/friends", nil)); len(fl.Friends) != 1 {
			t.Errorf("friends: %+v", fl)
		}
	}
	// Ann is told Bob accepted.
	n = decode[notifications.Page](t, a.do(ann, http.MethodGet, "/me/notifications", nil))
	if n.Total != 1 || n.Notifications[0].Message != "Bob accepted your friend request" {
		t.Errorf("accept notification: %+v", n)
	}

	a.expect(a.do(ann, http.MethodDelete, "/friends/"+bob.String(), nil), http.StatusNoContent, "")
	a.expect(a.do(ann, http.MethodDelete, "/friends/"+bob.String(), nil), http.StatusNotFound, "not_found")

	// Decline path.
	a.expect(a.do(bob, http.MethodPost, "/friends", map[string]any{"user": "ann"}), http.StatusCreated, "")
	reqs = decode[friends.Requests](t, a.do(ann, http.MethodGet, "/friends/requests", nil))
	a.expect(a.do(ann, http.MethodPost, "/friends/requests/"+reqs.Incoming[0].ID.String()+"/decline", nil), http.StatusNoContent, "")
	if fl := decode[friendsBody](t, a.do(ann, http.MethodGet, "/friends", nil)); len(fl.Friends) != 0 {
		t.Errorf("declined: %+v", fl)
	}
}

func TestFriendRequestErrors(t *testing.T) {
	a := newAPI(t)
	ann := a.newUser("ann")
	a.newUser("bob")
	for _, tc := range []struct {
		name, method, path string
		body               any
		status             int
		code               string
	}{
		{"send: bad json", http.MethodPost, "/friends", "{", 400, "bad_request"},
		{"send: nobody given", http.MethodPost, "/friends", map[string]any{}, 400, "bad_request"},
		{"send: both given", http.MethodPost, "/friends", map[string]any{"user": "bob", "user_id": uuid.NewString()}, 400, "bad_request"},
		{"send: yourself", http.MethodPost, "/friends", map[string]any{"user": "ann"}, 400, "bad_request"},
		{"send: unknown user", http.MethodPost, "/friends", map[string]any{"user": "ghost"}, 404, "not_found"},
		{"send: malformed user_id", http.MethodPost, "/friends", map[string]any{"user_id": "nope"}, 400, "bad_request"},
		{"remove: malformed id", http.MethodDelete, "/friends/nope", nil, 400, "bad_request"},
		{"remove: not friends", http.MethodDelete, "/friends/" + uuid.NewString(), nil, 404, "not_found"},
		{"accept: malformed id", http.MethodPost, "/friends/requests/nope/accept", nil, 400, "bad_request"},
		{"accept: unknown id", http.MethodPost, "/friends/requests/" + uuid.NewString() + "/accept", nil, 404, "not_found"},
		{"decline: unknown id", http.MethodPost, "/friends/requests/" + uuid.NewString() + "/decline", nil, 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.expect(a.do(ann, tc.method, tc.path, tc.body), tc.status, tc.code)
		})
	}
}

func TestInvitesProduceNotificationsAndWorkByFriendID(t *testing.T) {
	a := newAPI(t)
	owner, ann := a.newUser("owen"), a.newUser("ann")
	a.befriend(owner, ann, "ann")
	list := a.createList(owner, "movie")
	id := list.ID.String()

	// Invite the friend by ID (their email is never needed).
	a.expect(a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user_id": ann, "role": "editor"}), http.StatusCreated, "")
	a.expect(a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user_id": ann, "user": "ann", "role": "editor"}), http.StatusBadRequest, "bad_request")

	n := decode[notifications.Page](t, a.do(ann, http.MethodGet, "/me/notifications?unread=true", nil))
	var invite *notifications.View
	for i := range n.Notifications {
		if n.Notifications[i].Type == domain.NotificationWatchlistInvite {
			invite = &n.Notifications[i]
		}
	}
	if invite == nil || invite.Message != `Owen invited you to collaborate on "Friday"` || invite.LinkURL != "/invites" || invite.Actor.ID != owner {
		t.Fatalf("invite notification: %+v", n)
	}

	inv := decode[invitesBody](t, a.do(ann, http.MethodGet, "/me/invites", nil))
	a.expect(a.do(ann, http.MethodPost, "/invites/"+inv.Invites[0].ID.String()+"/accept", nil), http.StatusNoContent, "")
	// Owen hears that Ann accepted, and Ann's new item reaches Owen.
	a.addItem(ann, list.ID, 10)
	got := decode[notifications.Page](t, a.do(owner, http.MethodGet, "/me/notifications", nil))
	msgs := map[string]bool{}
	for _, v := range got.Notifications {
		msgs[v.Message] = true
	}
	if !msgs[`Ann accepted your invitation to "Friday"`] || !msgs[`Ann added a title to "Friday"`] {
		t.Errorf("owner's notifications: %v", msgs)
	}
}

func TestNotificationEndpoints(t *testing.T) {
	a := newAPI(t)
	ann, bob := a.newUser("ann"), a.newUser("bob")
	for i := 0; i < 3; i++ {
		other := a.newUser("u" + string(rune('a'+i)))
		a.expect(a.do(other, http.MethodPost, "/friends", map[string]any{"user": "ann"}), http.StatusCreated, "")
	}
	a.expect(a.do(bob, http.MethodPost, "/friends", map[string]any{"user": "ann"}), http.StatusCreated, "")

	count := decode[struct {
		UnreadCount int64 `json:"unread_count"`
	}](t, a.do(ann, http.MethodGet, "/me/notifications/unread-count", nil))
	if count.UnreadCount != 4 {
		t.Fatalf("unread = %d", count.UnreadCount)
	}
	page := decode[notifications.Page](t, a.do(ann, http.MethodGet, "/me/notifications?page=2&per_page=3", nil))
	if len(page.Notifications) != 1 || page.Total != 4 || page.UnreadCount != 4 {
		t.Errorf("paging: %+v", page)
	}

	first := decode[notifications.Page](t, a.do(ann, http.MethodGet, "/me/notifications", nil)).Notifications[0]
	a.expect(a.do(bob, http.MethodPost, "/me/notifications/"+first.ID.String()+"/read", nil), http.StatusNotFound, "not_found")
	a.expect(a.do(ann, http.MethodPost, "/me/notifications/"+first.ID.String()+"/read", nil), http.StatusNoContent, "")
	if unread := decode[notifications.Page](t, a.do(ann, http.MethodGet, "/me/notifications?unread=true", nil)); unread.Total != 3 {
		t.Errorf("unread only: %+v", unread)
	}
	w := a.do(ann, http.MethodPost, "/me/notifications/read-all", nil)
	a.expect(w, http.StatusOK, "")
	if marked := decode[struct{ Marked int64 }](t, w); marked.Marked != 3 {
		t.Errorf("marked: %s", w.Body)
	}
	if left := decode[notifications.Page](t, a.do(ann, http.MethodGet, "/me/notifications?unread=1", nil)); left.Total != 0 || left.UnreadCount != 0 {
		t.Errorf("after read-all: %+v", left)
	}
}

func TestSocialRequestErrorsAndServerErrors(t *testing.T) {
	a := newAPI(t)
	u := a.newUser("ann")
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"notifications: bad unread", http.MethodGet, "/me/notifications?unread=maybe", 400},
		{"notifications: bad page", http.MethodGet, "/me/notifications?page=x", 400},
		{"mark read: malformed id", http.MethodPost, "/me/notifications/nope/read", 400},
		{"mark read: unknown id", http.MethodPost, "/me/notifications/" + uuid.NewString() + "/read", 404},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.status == 404 {
				a.expect(a.do(u, tc.method, tc.path, nil), tc.status, "not_found")
			} else {
				a.expect(a.do(u, tc.method, tc.path, nil), tc.status, "bad_request")
			}
		})
	}

	for _, tc := range []struct {
		name, method, path string
		body               any
		op, table          string
	}{
		{"notifications list", http.MethodGet, "/me/notifications", nil, "query", "notifications"},
		{"unread count", http.MethodGet, "/me/notifications/unread-count", nil, "query", "notifications"},
		{"mark read", http.MethodPost, "/me/notifications/" + uuid.NewString() + "/read", nil, "update", "notifications"},
		{"mark all read", http.MethodPost, "/me/notifications/read-all", nil, "update", "notifications"},
		{"friends list", http.MethodGet, "/friends", nil, "query", "friendships"},
		{"friend requests", http.MethodGet, "/friends/requests", nil, "query", "friendships"},
		{"send request", http.MethodPost, "/friends", map[string]any{"user": "bob"}, "create", "friendships"},
		{"remove friend", http.MethodDelete, "/friends/" + uuid.NewString(), nil, "delete", "friendships"},
		{"accept", http.MethodPost, "/friends/requests/" + uuid.NewString() + "/accept", nil, "query", "friendships"},
		{"decline", http.MethodPost, "/friends/requests/" + uuid.NewString() + "/decline", nil, "query", "friendships"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newAPI(t)
			me := a.newUser("ann")
			a.newUser("bob")
			testutil.FailOn(t, a.db, tc.op, tc.table)
			a.expect(a.do(me, tc.method, tc.path, tc.body), http.StatusInternalServerError, "internal_error")
		})
	}
}

func TestSocialRoutesNeedAnAuthenticatedUser(t *testing.T) {
	a := newAPI(t)
	id := uuid.NewString()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/me/notifications"}, {http.MethodGet, "/me/notifications/unread-count"},
		{http.MethodPost, "/me/notifications/read-all"}, {http.MethodPost, "/me/notifications/" + id + "/read"},
		{http.MethodGet, "/friends"}, {http.MethodPost, "/friends"}, {http.MethodDelete, "/friends/" + id},
		{http.MethodGet, "/friends/requests"}, {http.MethodPost, "/friends/requests/" + id + "/accept"},
		{http.MethodPost, "/friends/requests/" + id + "/decline"},
	} {
		a.expect(a.do(uuid.Nil, tc.method, tc.path, nil), http.StatusInternalServerError, "internal_error")
	}
}
