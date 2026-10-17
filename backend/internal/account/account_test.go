package account

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func newService(t *testing.T) (*Service, *gorm.DB) {
	t.Helper()
	db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Movie{}, &domain.Show{}, &domain.Genre{},
		&domain.Review{}, &domain.Watchlist{}, &domain.WatchlistItem{}, &domain.Collaborator{},
		&domain.UserWatchProgress{}, &domain.Session{}, &domain.Notification{}, &domain.Friendship{})
	return NewService(db), db
}

func newUser(t *testing.T, db *gorm.DB, username string) domain.User {
	t.Helper()
	u := domain.User{Username: username, Email: username + "@example.com", DisplayName: "Display " + username}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func ptr(s string) *string { return &s }

func TestUpdateDisplayName(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	got, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr("  Ada Lovelace  ")})
	if err != nil {
		t.Fatal(err)
	}
	if got.DisplayName != "Ada Lovelace" || got.Username != "ada" {
		t.Errorf("got %+v", got)
	}
	var stored domain.User
	db.First(&stored, "id = ?", u.ID)
	if stored.DisplayName != "Ada Lovelace" {
		t.Errorf("stored name = %q", stored.DisplayName)
	}
}

func TestDisplayNameValidation(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	for name, in := range map[string]string{
		"empty":         "",
		"blank":         "   ",
		"too long":      strings.Repeat("x", 51),
		"control chars": "bad\x00name",
		"newline":       "two\nlines",
	} {
		_, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr(in)})
		if !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("%s: error = %v, want invalid", name, err)
		}
	}
	// 50 characters is fine even when each takes several bytes.
	if _, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr(strings.Repeat("é", 50))}); err != nil {
		t.Errorf("50 multi-byte characters should be accepted: %v", err)
	}
}

func TestUpdateUsername(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	got, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{Username: ptr("  Ada_L.99  ")})
	if err != nil {
		t.Fatal(err)
	}
	if got.Username != "ada_l.99" {
		t.Errorf("username = %q, want it trimmed and lower-cased", got.Username)
	}
	// Setting the one you already have is accepted.
	if _, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{Username: ptr("ada_l.99")}); err != nil {
		t.Errorf("keeping the same username: %v", err)
	}
}

func TestUsernameValidation(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	for name, in := range map[string]string{
		"too short":        "ab",
		"too long":         strings.Repeat("a", 33),
		"looks like email": "ada@example.com", // would be ambiguous with e-mail lookups
		"space":            "ada l",
		"symbol":           "ada!",
		"unicode":          "adá",
		"empty":            "",
	} {
		_, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{Username: ptr(in)})
		if !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("%s: error = %v, want invalid", name, err)
		}
	}
}

func TestUsernameMustBeUnique(t *testing.T) {
	svc, db := newService(t)
	a, b := newUser(t, db, "ada"), newUser(t, db, "grace")
	if _, err := svc.UpdateProfile(context.Background(), a.ID, ProfileUpdate{Username: ptr("taken")}); err != nil {
		t.Fatal(err)
	}
	_, err := svc.UpdateProfile(context.Background(), b.ID, ProfileUpdate{Username: ptr("TAKEN")})
	if !errors.Is(err, svcerr.ErrDuplicate) {
		t.Fatalf("error = %v, want duplicate", err)
	}
	if !strings.Contains(svcerr.MessageOr(err, ""), "taken") {
		t.Errorf("message should explain: %q", svcerr.MessageOr(err, ""))
	}
	var stored domain.User
	db.First(&stored, "id = ?", b.ID)
	if stored.Username != "grace" {
		t.Errorf("a failed change must leave the username alone, got %q", stored.Username)
	}
}

func TestUpdateBothAndNeither(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	got, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr("A"), Username: ptr("new_name")})
	if err != nil || got.DisplayName != "A" || got.Username != "new_name" {
		t.Errorf("both: %+v %v", got, err)
	}
	if _, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{}); !errors.Is(err, svcerr.ErrInvalid) {
		t.Errorf("nothing to change should be invalid, got %v", err)
	}
	// A bad field rejects the whole request: the good one is not applied.
	_, err = svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr("Fine"), Username: ptr("no")})
	if !errors.Is(err, svcerr.ErrInvalid) {
		t.Fatal(err)
	}
	var stored domain.User
	db.First(&stored, "id = ?", u.ID)
	if stored.DisplayName != "A" {
		t.Errorf("display name changed despite the rejected request: %q", stored.DisplayName)
	}
}

func TestUpdateUnknownUserAndDatabaseErrors(t *testing.T) {
	svc, db := newService(t)
	if _, err := svc.UpdateProfile(context.Background(), uuid.New(), ProfileUpdate{DisplayName: ptr("x")}); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
	u := newUser(t, db, "ada")
	testutil.FailOn(t, db, "update", "users")
	if _, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr("x")}); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("update failure should pass through, got %v", err)
	}
}

func TestUpdateReportsLoadFailures(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	testutil.FailOn(t, db, "query", "users")
	if _, err := svc.UpdateProfile(context.Background(), u.ID, ProfileUpdate{DisplayName: ptr("x")}); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("load failure should pass through, got %v", err)
	}
}

func TestDeleteRequiresConfirmation(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	for _, confirm := range []string{"", "nope", "ada@exampl.com"} {
		if err := svc.Delete(context.Background(), u.ID, confirm); !errors.Is(err, svcerr.ErrInvalid) {
			t.Errorf("confirm %q: error = %v, want invalid", confirm, err)
		}
	}
	var n int64
	db.Model(&domain.User{}).Where("id = ?", u.ID).Count(&n)
	if n != 1 {
		t.Error("the account must survive a wrong confirmation")
	}
}

func TestDeleteConfirmationMatchesUsernameOrEmailLoosely(t *testing.T) {
	svc, db := newService(t)
	for name, confirm := range map[string]string{
		"username":                  "ada",
		"padded, differently cased": "  ADA ",
		"e-mail":                    "ADA@example.com",
	} {
		t.Run(name, func(t *testing.T) {
			u := newUser(t, db, "ada") // earlier subtests removed theirs for good
			if err := svc.Delete(context.Background(), u.ID, confirm); err != nil {
				t.Fatalf("confirm %q: %v", confirm, err)
			}
			var n int64
			db.Unscoped().Model(&domain.User{}).Where("id = ?", u.ID).Count(&n)
			if n != 0 {
				t.Error("the user row should be gone")
			}
		})
	}
}

func TestDeletePurgesTheirData(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	db.Create(&domain.Session{UserID: u.ID, TokenHash: "h", ExpiresAt: time.Now().Add(time.Hour)})
	db.Create(&domain.OAuthAccount{UserID: u.ID, Provider: "google", ProviderUserID: "g1"})
	if err := svc.Delete(context.Background(), u.ID, "ada"); err != nil {
		t.Fatal(err)
	}
	for _, m := range []any{&domain.Session{}, &domain.OAuthAccount{}} {
		var n int64
		db.Unscoped().Model(m).Where("user_id = ?", u.ID).Count(&n)
		if n != 0 {
			t.Errorf("%T rows remain", m)
		}
	}
}

func TestDeleteUnknownUserAndDatabaseErrors(t *testing.T) {
	svc, db := newService(t)
	if err := svc.Delete(context.Background(), uuid.New(), "x"); !errors.Is(err, svcerr.ErrNotFound) {
		t.Errorf("unknown user: %v", err)
	}
	u := newUser(t, db, "ada")
	testutil.FailOn(t, db, "query", "users")
	if err := svc.Delete(context.Background(), u.ID, "ada"); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("load failure should pass through, got %v", err)
	}
}

func TestDeletePassesPurgeFailuresThrough(t *testing.T) {
	svc, db := newService(t)
	u := newUser(t, db, "ada")
	testutil.FailOn(t, db, "delete", "users")
	if err := svc.Delete(context.Background(), u.ID, "ada"); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("purge failure should pass through, got %v", err)
	}
}
