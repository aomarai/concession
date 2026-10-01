package watchlist

import (
	"errors"
	"strings"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func (e *env) account(t *testing.T, name string) uuid.UUID {
	t.Helper()
	u := domain.User{Username: name, Email: name + "@Example.com", DisplayName: strings.ToUpper(name[:1]) + name[1:], AvatarURL: "http://pic/" + name}
	if err := e.db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u.ID
}

// ownerAccount makes e.owner a real user so member listings show its name.
func (e *env) ownerAccount(t *testing.T) {
	t.Helper()
	u := domain.User{BaseUUID: domain.BaseUUID{ID: e.owner}, Username: "olive", Email: "olive@example.com", DisplayName: "Olive"}
	if err := e.db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
}

func TestInviteAcceptFlow(t *testing.T) {
	e := newEnv(t)
	e.ownerAccount(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")

	m, err := e.svc.InviteUser(ctx, e.owner, list, "  ANN@example.com ", domain.RoleEditor)
	if err != nil {
		t.Fatal(err)
	}
	if m.ID != ann || m.DisplayName != "Ann" || m.Role != domain.RoleEditor || m.Status != domain.CollaboratorPending {
		t.Errorf("unexpected member %+v", m)
	}

	// A pending invitation grants nothing.
	if _, err := e.svc.Get(ctx, ann, list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("pending invitee must not see the list: %v", err)
	}
	if lists, _ := e.svc.List(ctx, ann); len(lists) != 0 {
		t.Errorf("pending invitee must not see it in their lists: %+v", lists)
	}

	invites, err := e.svc.ListInvites(ctx, ann)
	if err != nil || len(invites) != 1 {
		t.Fatalf("invites: %+v, %v", invites, err)
	}
	inv := invites[0]
	if inv.WatchlistID != list || inv.WatchlistName != "List" || inv.Role != domain.RoleEditor ||
		inv.InvitedBy == nil || inv.InvitedBy.DisplayName != "Olive" || inv.ID == uuid.Nil {
		t.Errorf("invite wrong: %+v", inv)
	}

	if err := e.svc.AcceptInvite(ctx, ann, inv.ID); err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Get(ctx, ann, list)
	if err != nil || d.Role != domain.RoleEditor {
		t.Fatalf("after accepting: %+v, %v", d, err)
	}
	if lists, _ := e.svc.List(ctx, ann); len(lists) != 1 || lists[0].Role != domain.RoleEditor {
		t.Errorf("accepted list should appear with the role: %+v", lists)
	}
	if invites, _ := e.svc.ListInvites(ctx, ann); len(invites) != 0 {
		t.Errorf("accepted invites are no longer pending: %+v", invites)
	}
	// The new editor can add items.
	if _, err := e.svc.AddItem(ctx, ann, list, 5, ""); err != nil {
		t.Errorf("editor add: %v", err)
	}
	// Accepting twice is not possible.
	if err := e.svc.AcceptInvite(ctx, ann, inv.ID); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("second accept: %v", err)
	}
}

func TestInviteValidation(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")
	e.account(t, "bob")
	e.ownerAccount(t)
	editor := e.member(t, list, domain.RoleEditor)

	cases := []struct {
		name  string
		actor uuid.UUID
		user  string
		role  domain.CollaboratorRole
		want  error
		msg   string
	}{
		{"non-owner", editor, "bob", domain.RoleViewer, svcerr.ErrForbidden, ""},
		{"stranger", uuid.New(), "bob", domain.RoleViewer, svcerr.ErrNotFound, ""},
		{"owner role", e.owner, "bob", domain.RoleOwner, svcerr.ErrInvalid, "role must be"},
		{"empty role", e.owner, "bob", "", svcerr.ErrInvalid, "role must be"},
		{"blank user", e.owner, "  ", domain.RoleViewer, svcerr.ErrInvalid, "user is required"},
		{"unknown user", e.owner, "nobody", domain.RoleViewer, svcerr.ErrNotFound, "No user found"},
		{"yourself", e.owner, "olive", domain.RoleViewer, svcerr.ErrInvalid, "already own"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := e.svc.InviteUser(ctx, tc.actor, list, tc.user, tc.role)
			if !errors.Is(err, tc.want) {
				t.Fatalf("got %v", err)
			}
			if tc.msg != "" && !strings.Contains(svcerr.MessageOr(err, ""), tc.msg) {
				t.Errorf("message %q does not mention %q", svcerr.MessageOr(err, ""), tc.msg)
			}
		})
	}

	t.Run("duplicates", func(t *testing.T) {
		if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleViewer); err != nil {
			t.Fatal(err)
		}
		_, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleEditor)
		if !errors.Is(err, svcerr.ErrDuplicate) || !strings.Contains(svcerr.MessageOr(err, ""), "already been invited") {
			t.Errorf("re-invite: %v", err)
		}
		invites, _ := e.svc.ListInvites(ctx, ann)
		if err := e.svc.AcceptInvite(ctx, ann, invites[0].ID); err != nil {
			t.Fatal(err)
		}
		_, err = e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleEditor)
		if !errors.Is(err, svcerr.ErrDuplicate) || !strings.Contains(svcerr.MessageOr(err, ""), "already collaborates") {
			t.Errorf("invite existing member: %v", err)
		}
		if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleViewer); !errors.Is(err, svcerr.ErrDuplicate) {
			t.Errorf("existing collaborators found by row, not by who invited: %v", err)
		}
	})
}

func TestInviteMatchesUsernameOrEmail(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	e.account(t, "ann")
	e.account(t, "bob")
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleViewer); err != nil {
		t.Errorf("by username: %v", err)
	}
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "BOB@example.COM", domain.RoleViewer); err != nil {
		t.Errorf("by e-mail, any case: %v", err)
	}
}

func TestConcurrentInvitesCreateOneInvitation(t *testing.T) {
	db := testutil.NewFileDB(t, models()...)
	svc := NewService(db, &fakeCatalog{db: db})
	owner := uuid.New()
	w, err := svc.Create(ctx, owner, CreateInput{Title: "L", Type: domain.WatchlistTypeMovie})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.User{Username: "ann", Email: "ann@x.com", DisplayName: "Ann"}).Error; err != nil {
		t.Fatal(err)
	}
	results := make(chan error, 10)
	for i := 0; i < 10; i++ {
		go func() {
			_, err := svc.InviteUser(ctx, owner, w.ID, "ann", domain.RoleViewer)
			results <- err
		}()
	}
	var ok, dup int
	for i := 0; i < 10; i++ {
		switch err := <-results; {
		case err == nil:
			ok++
		case errors.Is(err, svcerr.ErrDuplicate):
			dup++
		default:
			t.Errorf("unexpected error: %v", err)
		}
	}
	if ok != 1 || dup != 9 {
		t.Errorf("expected 1 invitation and 9 duplicates, got %d and %d", ok, dup)
	}
}

func TestDeclineAndWrongRecipient(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann, bob := e.account(t, "ann"), e.account(t, "bob")
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleViewer); err != nil {
		t.Fatal(err)
	}
	invites, _ := e.svc.ListInvites(ctx, ann)
	id := invites[0].ID

	if err := e.svc.AcceptInvite(ctx, bob, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("someone else accepting: %v", err)
	}
	if err := e.svc.DeclineInvite(ctx, bob, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("someone else declining: %v", err)
	}
	if err := e.svc.DeclineInvite(ctx, ann, id); err != nil {
		t.Fatal(err)
	}
	if invites, _ := e.svc.ListInvites(ctx, ann); len(invites) != 0 {
		t.Errorf("declined invite lingers: %+v", invites)
	}
	if err := e.svc.DeclineInvite(ctx, ann, id); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("second decline: %v", err)
	}
	// Declining is permanent, so the owner can invite again.
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleEditor); err != nil {
		t.Errorf("re-invite after decline: %v", err)
	}
}

func TestListMembers(t *testing.T) {
	e := newEnv(t)
	e.ownerAccount(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann, bob := e.account(t, "ann"), e.account(t, "bob")
	e.account(t, "cy")
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleEditor); err != nil {
		t.Fatal(err)
	}
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "bob", domain.RoleViewer); err != nil {
		t.Fatal(err)
	}
	invites, _ := e.svc.ListInvites(ctx, ann)
	if err := e.svc.AcceptInvite(ctx, ann, invites[0].ID); err != nil {
		t.Fatal(err)
	}

	m, err := e.svc.ListMembers(ctx, e.owner, list)
	if err != nil {
		t.Fatal(err)
	}
	if m.Owner.DisplayName != "Olive" || m.Owner.Role != domain.RoleOwner || len(m.Members) != 1 || m.Members[0].ID != ann ||
		m.Members[0].JoinedAt == nil || len(m.Pending) != 1 || m.Pending[0].ID != bob || m.Pending[0].JoinedAt != nil {
		t.Errorf("owner view wrong: %+v", m)
	}
	// Collaborators see who is on the list but not who is still pending.
	m, err = e.svc.ListMembers(ctx, ann, list)
	if err != nil || len(m.Members) != 1 || len(m.Pending) != 0 {
		t.Errorf("member view: %+v, %v", m, err)
	}
	if _, err := e.svc.ListMembers(ctx, bob, list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("pending invitee: %v", err)
	}
	if _, err := e.svc.ListMembers(ctx, uuid.New(), list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
}

func TestListMembersWithDeletedAccounts(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ghost := e.member(t, list, domain.RoleViewer) // no user row
	m, err := e.svc.ListMembers(ctx, e.owner, list)
	if err != nil || len(m.Members) != 1 || m.Members[0].ID != ghost || m.Members[0].DisplayName != "" || m.Owner.ID != e.owner {
		t.Errorf("got %+v, %v", m, err)
	}
}

func TestSetRole(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.member(t, list, domain.RoleViewer)

	if err := e.svc.SetRole(ctx, e.owner, list, ann, domain.RoleEditor); err != nil {
		t.Fatal(err)
	}
	if d, _ := e.svc.Get(ctx, ann, list); d.Role != domain.RoleEditor {
		t.Errorf("role not changed: %+v", d)
	}
	for name, tc := range map[string]struct {
		actor, target uuid.UUID
		role          domain.CollaboratorRole
		want          error
	}{
		"non-owner":    {ann, ann, domain.RoleViewer, svcerr.ErrForbidden},
		"stranger":     {uuid.New(), ann, domain.RoleViewer, svcerr.ErrNotFound},
		"bad role":     {e.owner, ann, domain.RoleOwner, svcerr.ErrInvalid},
		"owner":        {e.owner, e.owner, domain.RoleViewer, svcerr.ErrInvalid},
		"not a member": {e.owner, uuid.New(), domain.RoleViewer, svcerr.ErrNotFound},
	} {
		if err := e.svc.SetRole(ctx, tc.actor, list, tc.target, tc.role); !errors.Is(err, tc.want) {
			t.Errorf("%s: %v", name, err)
		}
	}
	testutil.FailOn(t, e.db, "update", "collaborators")
	if err := e.svc.SetRole(ctx, e.owner, list, ann, domain.RoleViewer); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("db failure: %v", err)
	}
}

func TestRemoveMember(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.member(t, list, domain.RoleEditor)
	bob := e.member(t, list, domain.RoleViewer)

	// Members cannot remove each other, but can leave.
	if err := e.svc.RemoveMember(ctx, ann, list, bob); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("editor removing another member: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, bob, list, bob); err != nil {
		t.Errorf("leaving: %v", err)
	}
	if _, err := e.svc.Get(ctx, bob, list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("left member still has access: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, e.owner, list, ann); err != nil {
		t.Errorf("owner removing: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, e.owner, list, ann); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("already removed: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, e.owner, list, e.owner); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("removing the owner: %v", err)
	}
	if err := e.svc.RemoveMember(ctx, uuid.New(), list, ann); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
	// Permanent removal: the same person can be invited again.
	e.account(t, "ann2")
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann2", domain.RoleViewer); err != nil {
		t.Fatal(err)
	}
	invites, _ := e.svc.ListInvites(ctx, mustUser(t, e, "ann2"))
	if len(invites) != 1 {
		t.Fatalf("invites: %+v", invites)
	}
	if err := e.svc.RemoveMember(ctx, e.owner, list, mustUser(t, e, "ann2")); err != nil {
		t.Errorf("owner cancelling a pending invite: %v", err)
	}
	if invites, _ := e.svc.ListInvites(ctx, mustUser(t, e, "ann2")); len(invites) != 0 {
		t.Errorf("cancelled invite lingers: %+v", invites)
	}
	if _, err := e.svc.InviteUser(ctx, e.owner, list, "ann2", domain.RoleViewer); err != nil {
		t.Errorf("re-invite after removal: %v", err)
	}

	testutil.FailOn(t, e.db, "delete", "collaborators")
	if err := e.svc.RemoveMember(ctx, e.owner, list, uuid.New()); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("db failure: %v", err)
	}
}

func mustUser(t *testing.T, e *env, username string) uuid.UUID {
	t.Helper()
	var u domain.User
	if err := e.db.Where("username = ?", username).First(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u.ID
}

func TestPublicListsAreReadableByAnyone(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	e.addMovies(t, list, 1)
	stranger := uuid.New()

	if _, err := e.svc.Get(ctx, stranger, list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Fatalf("private list leaked: %v", err)
	}
	public := domain.PrivacyPublic
	if _, err := e.svc.Update(ctx, e.owner, list, UpdateInput{Privacy: &public}); err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.Get(ctx, stranger, list)
	if err != nil || d.Role != domain.RoleViewer || len(d.Items) != 1 || d.ShareToken != "" {
		t.Fatalf("public list: %+v, %v", d, err)
	}
	// Read-only: no edits, no member management.
	if _, err := e.svc.AddItem(ctx, stranger, list, 2, ""); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("add to public list: %v", err)
	}
	if err := e.svc.Delete(ctx, stranger, list); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("delete public list: %v", err)
	}
	// Reading a public list does not put it in the reader's own lists.
	if lists, _ := e.svc.List(ctx, stranger); len(lists) != 0 {
		t.Errorf("public lists are not added to everyone's lists: %+v", lists)
	}
	// "shared" is not "public".
	shared := domain.PrivacyShared
	e.svc.Update(ctx, e.owner, list, UpdateInput{Privacy: &shared})
	if _, err := e.svc.Get(ctx, stranger, list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("shared list must need the link: %v", err)
	}
}

func TestShareLinks(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	e.addMovies(t, list, 1)
	stranger := uuid.New()
	var w domain.Watchlist
	e.db.First(&w, "id = ?", list)
	token := w.ShareToken

	if _, err := e.svc.GetShared(ctx, stranger, token); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("private lists have no working link: %v", err)
	}
	shared := domain.PrivacyShared
	if _, err := e.svc.Update(ctx, e.owner, list, UpdateInput{Privacy: &shared}); err != nil {
		t.Fatal(err)
	}
	d, err := e.svc.GetShared(ctx, stranger, token)
	if err != nil || d.Role != domain.RoleViewer || len(d.Items) != 1 || d.ShareToken != "" {
		t.Fatalf("link: %+v, %v", d, err)
	}
	// Members keep their real role (and the owner their token) when using the link.
	if d, _ := e.svc.GetShared(ctx, e.owner, token); d.Role != domain.RoleOwner || d.ShareToken != token {
		t.Errorf("owner via link: %+v", d)
	}
	editor := e.member(t, list, domain.RoleEditor)
	if d, _ := e.svc.GetShared(ctx, editor, token); d.Role != domain.RoleEditor {
		t.Errorf("editor via link: %+v", d)
	}
	// Public lists work through the link too.
	public := domain.PrivacyPublic
	e.svc.Update(ctx, e.owner, list, UpdateInput{Privacy: &public})
	if _, err := e.svc.GetShared(ctx, stranger, token); err != nil {
		t.Errorf("public link: %v", err)
	}

	for _, bad := range []string{"", "nope"} {
		if _, err := e.svc.GetShared(ctx, stranger, bad); !errors.Is(err, svcerr.ErrNotFound) {
			t.Errorf("token %q: %v", bad, err)
		}
	}
}

func TestRotateShareToken(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	shared := domain.PrivacyShared
	e.svc.Update(ctx, e.owner, list, UpdateInput{Privacy: &shared})
	var w domain.Watchlist
	e.db.First(&w, "id = ?", list)
	old := w.ShareToken

	fresh, err := e.svc.RotateShareToken(ctx, e.owner, list)
	if err != nil || fresh == "" || fresh == old {
		t.Fatalf("got %q, %v", fresh, err)
	}
	if _, err := e.svc.GetShared(ctx, uuid.New(), old); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("the old link must stop working: %v", err)
	}
	if _, err := e.svc.GetShared(ctx, uuid.New(), fresh); err != nil {
		t.Errorf("the new link works: %v", err)
	}

	editor := e.member(t, list, domain.RoleEditor)
	if _, err := e.svc.RotateShareToken(ctx, editor, list); !errors.Is(err, svcerr.ErrForbidden) {
		t.Errorf("editor: %v", err)
	}
	if _, err := e.svc.RotateShareToken(ctx, uuid.New(), list); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("stranger: %v", err)
	}
}

func TestEntropyFailureWhenRotating(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	domain.SetRandReadForTest(func([]byte) (int, error) { return 0, errors.New("no entropy") })
	t.Cleanup(domain.ResetRandReadForTest)
	if _, err := e.svc.RotateShareToken(ctx, e.owner, list); err == nil {
		t.Error("expected an error when no token can be generated")
	}
}

func TestCollaborationDBFailures(t *testing.T) {
	setup := func(t *testing.T) (*env, uuid.UUID, uuid.UUID) {
		e := newEnv(t)
		e.ownerAccount(t)
		list := e.newList(t, domain.WatchlistTypeMovie)
		ann := e.account(t, "ann")
		return e, list, ann
	}
	type call func(e *env, list, ann uuid.UUID) error
	invite := func(e *env, list, ann uuid.UUID) error {
		_, err := e.svc.InviteUser(ctx, e.owner, list, "ann", domain.RoleViewer)
		return err
	}
	members := func(e *env, list, ann uuid.UUID) error { _, err := e.svc.ListMembers(ctx, e.owner, list); return err }
	invites := func(e *env, list, ann uuid.UUID) error { _, err := e.svc.ListInvites(ctx, ann); return err }
	accept := func(e *env, list, ann uuid.UUID) error {
		return e.svc.AcceptInvite(ctx, ann, mustInvite(e, list, ann))
	}
	decline := func(e *env, list, ann uuid.UUID) error {
		return e.svc.DeclineInvite(ctx, ann, mustInvite(e, list, ann))
	}
	var token string // read before failures are injected
	shared := func(e *env, list, ann uuid.UUID) error {
		_, err := e.svc.GetShared(ctx, ann, token)
		return err
	}
	rotate := func(e *env, list, ann uuid.UUID) error {
		_, err := e.svc.RotateShareToken(ctx, e.owner, list)
		return err
	}

	for _, tc := range []struct {
		name, op, table string
		after           int
		seed            func(e *env, list, ann uuid.UUID)
		call            call
	}{
		{"invite: user lookup", "query", "users", 0, nil, invite},
		{"invite: existing lookup", "query", "collaborators", 0, nil, invite},
		{"invite: insert", "create", "collaborators", 0, nil, invite},
		{"members: collaborators", "query", "collaborators", 0, nil, members},
		{"members: people", "query", "users", 0, nil, members},
		{"invites: rows", "query", "collaborators", 0, nil, invites},
		{"invites: lists", "query", "watchlists", 0, func(e *env, l, a uuid.UUID) { invite(e, l, a) }, invites},
		{"invites: inviters", "query", "users", 0, func(e *env, l, a uuid.UUID) { invite(e, l, a) }, invites},
		{"accept: lookup", "query", "collaborators", 0, func(e *env, l, a uuid.UUID) { invite(e, l, a) }, func(e *env, l, a uuid.UUID) error {
			return e.svc.AcceptInvite(ctx, a, uuid.New())
		}},
		{"accept: update", "update", "collaborators", 0, func(e *env, l, a uuid.UUID) { invite(e, l, a) }, accept},
		{"decline: delete", "delete", "collaborators", 0, func(e *env, l, a uuid.UUID) { invite(e, l, a) }, decline},
		{"shared: lookup", "query", "watchlists", 0, func(e *env, l, a uuid.UUID) {
			var w domain.Watchlist
			e.db.First(&w, "id = ?", l)
			token = w.ShareToken
		}, shared},
		{"shared: access check", "query", "watchlists", 1, func(e *env, l, a uuid.UUID) {
			var w domain.Watchlist
			e.db.First(&w, "id = ?", l)
			token = w.ShareToken
			shared := domain.PrivacyShared
			e.svc.Update(ctx, e.owner, l, UpdateInput{Privacy: &shared})
		}, shared},
		{"rotate: update", "update", "watchlists", 0, nil, rotate},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e, list, ann := setup(t)
			if tc.seed != nil {
				tc.seed(e, list, ann)
			}
			testutil.FailAfter(t, e.db, tc.op, tc.table, tc.after)
			if err := tc.call(e, list, ann); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("got %v", err)
			}
		})
	}
}

func mustInvite(e *env, list, ann uuid.UUID) uuid.UUID {
	var c domain.Collaborator
	e.db.Where("watchlist_id = ? AND user_id = ?", list, ann).First(&c)
	return c.ID
}

func TestInvitesWithoutAKnownInviter(t *testing.T) {
	e := newEnv(t)
	list := e.newList(t, domain.WatchlistTypeMovie)
	ann := e.account(t, "ann")
	if err := e.db.Create(&domain.Collaborator{UserID: ann, WatchlistID: list, Role: domain.RoleViewer, Status: domain.CollaboratorPending}).Error; err != nil {
		t.Fatal(err)
	}
	invites, err := e.svc.ListInvites(ctx, ann)
	if err != nil || len(invites) != 1 || invites[0].InvitedBy != nil {
		t.Errorf("got %+v, %v", invites, err)
	}
}
