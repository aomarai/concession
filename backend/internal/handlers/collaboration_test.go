package handlers

import (
	"net/http"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/watchlist"
	"github.com/google/uuid"
)

type membersBody = watchlist.Members

type invitesBody struct {
	Invites []watchlist.Invite `json:"invites"`
}

func TestInviteAndShareFlow(t *testing.T) {
	a := newAPI(t)
	owner, ann, bob := a.newUser("owen"), a.newUser("ann"), a.newUser("bob")
	list := a.createList(owner, "movie")
	id := list.ID.String()

	// Invite by username; the invitee cannot see the list until they accept.
	w := a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user": "ann", "role": "editor"})
	a.expect(w, http.StatusCreated, "")
	if m := decode[watchlist.Member](t, w); m.ID != ann || m.Status != domain.CollaboratorPending || strings.Contains(w.Body.String(), "@") {
		t.Errorf("invite: %s", w.Body)
	}
	a.expect(a.do(ann, http.MethodGet, "/watchlists/"+id, nil), http.StatusNotFound, "not_found")

	invites := decode[invitesBody](t, a.do(ann, http.MethodGet, "/me/invites", nil))
	if len(invites.Invites) != 1 || invites.Invites[0].WatchlistName != "Friday" || invites.Invites[0].InvitedBy.DisplayName != "Owen" {
		t.Fatalf("invites: %+v", invites)
	}
	inviteID := invites.Invites[0].ID.String()
	a.expect(a.do(bob, http.MethodPost, "/invites/"+inviteID+"/accept", nil), http.StatusNotFound, "not_found")
	a.expect(a.do(ann, http.MethodPost, "/invites/"+inviteID+"/accept", nil), http.StatusNoContent, "")
	a.expect(a.do(ann, http.MethodGet, "/watchlists/"+id, nil), http.StatusOK, "")
	a.addItem(ann, list.ID, 10) // editors can add

	members := decode[membersBody](t, a.do(owner, http.MethodGet, "/watchlists/"+id+"/collaborators", nil))
	if members.Owner.ID != owner || len(members.Members) != 1 || members.Members[0].Role != domain.RoleEditor {
		t.Errorf("members: %+v", members)
	}

	// Change the role, then the editor can no longer add.
	uid := ann.String()
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+id+"/collaborators/"+uid, map[string]any{"role": "viewer"}), http.StatusNoContent, "")
	a.expect(a.do(ann, http.MethodPost, "/watchlists/"+id+"/items", map[string]any{"tmdb_id": 11}), http.StatusForbidden, "forbidden")

	// Decline path.
	a.expect(a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user": "bob@example.com", "role": "viewer"}), http.StatusCreated, "")
	bobInvites := decode[invitesBody](t, a.do(bob, http.MethodGet, "/me/invites", nil))
	a.expect(a.do(bob, http.MethodPost, "/invites/"+bobInvites.Invites[0].ID.String()+"/decline", nil), http.StatusNoContent, "")
	if left := decode[invitesBody](t, a.do(bob, http.MethodGet, "/me/invites", nil)); len(left.Invites) != 0 {
		t.Errorf("declined invite lingers: %+v", left)
	}

	// Share links: private lists have none; "shared" lists work for any signed-in user.
	token := decode[watchlist.Detail](t, a.do(owner, http.MethodGet, "/watchlists/"+id, nil)).ShareToken
	a.expect(a.do(bob, http.MethodGet, "/shared/"+token, nil), http.StatusNotFound, "not_found")
	a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+id, map[string]any{"privacy": "shared"}), http.StatusOK, "")
	w = a.do(bob, http.MethodGet, "/shared/"+token, nil)
	a.expect(w, http.StatusOK, "")
	if d := decode[watchlist.Detail](t, w); d.Role != domain.RoleViewer || len(d.Items) != 1 || d.ShareToken != "" {
		t.Errorf("shared view: %+v", d)
	}

	// Rotating the token kills the old link.
	w = a.do(owner, http.MethodPost, "/watchlists/"+id+"/share-token", nil)
	a.expect(w, http.StatusOK, "")
	fresh := decode[struct {
		Token string `json:"share_token"`
	}](t, w).Token
	if fresh == "" || fresh == token {
		t.Fatalf("token not rotated: %q", fresh)
	}
	a.expect(a.do(bob, http.MethodGet, "/shared/"+token, nil), http.StatusNotFound, "not_found")
	a.expect(a.do(bob, http.MethodGet, "/shared/"+fresh, nil), http.StatusOK, "")

	// Leaving, and removing.
	a.expect(a.do(ann, http.MethodDelete, "/watchlists/"+id+"/collaborators/"+uid, nil), http.StatusNoContent, "")
	a.expect(a.do(ann, http.MethodGet, "/watchlists/"+id, nil), http.StatusNotFound, "not_found")
}

func TestCollaborationRequestErrors(t *testing.T) {
	a := newAPI(t)
	owner, ann := a.newUser("owen"), a.newUser("ann")
	list := a.createList(owner, "movie")
	id := list.ID.String()
	a.expect(a.do(owner, http.MethodPost, "/watchlists/"+id+"/collaborators", map[string]any{"user": "ann", "role": "viewer"}), http.StatusCreated, "")
	uid := ann.String()

	for _, tc := range []struct {
		name         string
		user         uuid.UUID
		method, path string
		body         any
		status       int
		code         string
	}{
		{"members: malformed id", owner, http.MethodGet, "/watchlists/nope/collaborators", nil, 400, "bad_request"},
		{"members: stranger", ann, http.MethodGet, "/watchlists/" + id + "/collaborators", nil, 404, "not_found"},
		{"invite: malformed id", owner, http.MethodPost, "/watchlists/nope/collaborators", map[string]any{}, 400, "bad_request"},
		{"invite: bad json", owner, http.MethodPost, "/watchlists/" + id + "/collaborators", "{", 400, "bad_request"},
		{"invite: bad role", owner, http.MethodPost, "/watchlists/" + id + "/collaborators", map[string]any{"user": "ann", "role": "owner"}, 400, "bad_request"},
		{"invite: unknown user", owner, http.MethodPost, "/watchlists/" + id + "/collaborators", map[string]any{"user": "ghost", "role": "viewer"}, 404, "not_found"},
		{"invite: already invited", owner, http.MethodPost, "/watchlists/" + id + "/collaborators", map[string]any{"user": "ann", "role": "viewer"}, 409, "conflict"},
		{"invite: not the owner", ann, http.MethodPost, "/watchlists/" + id + "/collaborators", map[string]any{"user": "owen", "role": "viewer"}, 404, "not_found"},
		{"role: malformed list id", owner, http.MethodPatch, "/watchlists/nope/collaborators/" + uid, map[string]any{}, 400, "bad_request"},
		{"role: malformed user id", owner, http.MethodPatch, "/watchlists/" + id + "/collaborators/nope", map[string]any{}, 400, "bad_request"},
		{"role: bad json", owner, http.MethodPatch, "/watchlists/" + id + "/collaborators/" + uid, "{", 400, "bad_request"},
		{"role: bad role", owner, http.MethodPatch, "/watchlists/" + id + "/collaborators/" + uid, map[string]any{"role": "x"}, 400, "bad_request"},
		{"remove: malformed list id", owner, http.MethodDelete, "/watchlists/nope/collaborators/" + uid, nil, 400, "bad_request"},
		{"remove: malformed user id", owner, http.MethodDelete, "/watchlists/" + id + "/collaborators/nope", nil, 400, "bad_request"},
		{"remove: owner", owner, http.MethodDelete, "/watchlists/" + id + "/collaborators/" + owner.String(), nil, 400, "bad_request"},
		{"rotate: malformed id", owner, http.MethodPost, "/watchlists/nope/share-token", nil, 400, "bad_request"},
		{"rotate: not the owner", ann, http.MethodPost, "/watchlists/" + id + "/share-token", nil, 404, "not_found"},
		{"shared: unknown token", owner, http.MethodGet, "/shared/nope", nil, 404, "not_found"},
		{"accept: malformed id", ann, http.MethodPost, "/invites/nope/accept", nil, 400, "bad_request"},
		{"decline: unknown id", ann, http.MethodPost, "/invites/" + uuid.NewString() + "/decline", nil, 404, "not_found"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a.expect(a.do(tc.user, tc.method, tc.path, tc.body), tc.status, tc.code)
		})
	}
}

func TestInviteErrorsCarryAMessage(t *testing.T) {
	a := newAPI(t)
	owner := a.newUser("owen")
	list := a.createList(owner, "movie")
	w := a.do(owner, http.MethodPost, "/watchlists/"+list.ID.String()+"/collaborators", map[string]any{"user": "ghost", "role": "viewer"})
	a.expect(w, http.StatusNotFound, "not_found")
	if !strings.Contains(w.Body.String(), "No user found") {
		t.Errorf("expected the helpful message, got %s", w.Body)
	}
}

func TestCollaborationServerErrors(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		body               any
		op, table          string
	}{
		{"members", http.MethodGet, "/watchlists/{id}/collaborators", nil, "query", "collaborators"},
		{"invite", http.MethodPost, "/watchlists/{id}/collaborators", map[string]any{"user": "cy", "role": "viewer"}, "create", "collaborators"},
		{"role", http.MethodPatch, "/watchlists/{id}/collaborators/{ann}", map[string]any{"role": "editor"}, "update", "collaborators"},
		{"remove", http.MethodDelete, "/watchlists/{id}/collaborators/{ann}", nil, "delete", "collaborators"},
		{"rotate", http.MethodPost, "/watchlists/{id}/share-token", nil, "update", "watchlists"},
		{"shared", http.MethodGet, "/shared/{token}", nil, "query", "watchlists"},
		{"invites", http.MethodGet, "/me/invites", nil, "query", "collaborators"},
		{"accept", http.MethodPost, "/invites/{invite}/accept", nil, "update", "collaborators"},
		{"decline", http.MethodPost, "/invites/{invite}/decline", nil, "delete", "collaborators"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newAPI(t)
			owner, ann := a.newUser("owen"), a.newUser("ann")
			a.newUser("cy")
			list := a.createList(owner, "movie")
			a.expect(a.do(owner, http.MethodPatch, "/watchlists/"+list.ID.String(), map[string]any{"privacy": "shared"}), http.StatusOK, "")
			a.expect(a.do(owner, http.MethodPost, "/watchlists/"+list.ID.String()+"/collaborators", map[string]any{"user": "ann", "role": "viewer"}), http.StatusCreated, "")
			var c domain.Collaborator
			a.db.Where("user_id = ?", ann).First(&c)
			var w domain.Watchlist
			a.db.First(&w, "id = ?", list.ID)

			path := strings.NewReplacer("{id}", list.ID.String(), "{ann}", ann.String(), "{token}", w.ShareToken, "{invite}", c.ID.String()).Replace(tc.path)
			user := owner
			if tc.name == "invites" || tc.name == "accept" || tc.name == "decline" || tc.name == "shared" {
				user = ann
			}
			testutil.FailOn(t, a.db, tc.op, tc.table)
			a.expect(a.do(user, tc.method, path, tc.body), http.StatusInternalServerError, "internal_error")
		})
	}
}

func TestCollaborationRoutesNeedAnAuthenticatedUser(t *testing.T) {
	a := newAPI(t)
	id, other := uuid.NewString(), uuid.NewString()
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/watchlists/" + id + "/collaborators"}, {http.MethodPost, "/watchlists/" + id + "/collaborators"},
		{http.MethodPatch, "/watchlists/" + id + "/collaborators/" + other}, {http.MethodDelete, "/watchlists/" + id + "/collaborators/" + other},
		{http.MethodPost, "/watchlists/" + id + "/share-token"}, {http.MethodGet, "/shared/tok"},
		{http.MethodGet, "/me/invites"}, {http.MethodPost, "/invites/" + id + "/accept"}, {http.MethodPost, "/invites/" + id + "/decline"},
	} {
		a.expect(a.do(uuid.Nil, tc.method, tc.path, nil), http.StatusInternalServerError, "internal_error")
	}
}
