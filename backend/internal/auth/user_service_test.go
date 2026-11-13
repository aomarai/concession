package auth

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var googleInfo = GoogleUserInfo{ID: "g-1", Email: "a@example.com", VerifiedEmail: true, Name: "Ann", Picture: "http://pic"}

func TestFindOrCreateGoogleUserCreatesThenFinds(t *testing.T) {
	db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{})
	svc := NewUserAuthService(db)
	ctx := context.Background()

	created, err := svc.FindOrCreateGoogleUser(ctx, googleInfo)
	if err != nil {
		t.Fatal(err)
	}
	if created.Email != "a@example.com" || created.DisplayName != "Ann" || created.AvatarURL != "http://pic" || !created.IsEmailVerified {
		t.Errorf("profile not mapped: %+v", created)
	}

	found, err := svc.FindOrCreateGoogleUser(ctx, googleInfo)
	if err != nil {
		t.Fatal(err)
	}
	if found.ID != created.ID {
		t.Errorf("expected same user, got %v and %v", found.ID, created.ID)
	}
	var lists []domain.Watchlist
	if err := db.Where("owner_id = ?", created.ID).Order("title").Find(&lists).Error; err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || lists[0].Title != "Movies to watch" || lists[1].Title != "Shows to watch" {
		t.Errorf("expected the two starter lists, got %+v", lists)
	}
	var users, accounts int64
	db.Model(&domain.User{}).Count(&users)
	db.Model(&domain.OAuthAccount{}).Count(&accounts)
	if users != 1 || accounts != 1 {
		t.Errorf("expected 1 user and 1 account, got %d and %d", users, accounts)
	}
}

func TestFindOrCreateGoogleUserErrors(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T) *UserAuthService
	}{
		{"oauth lookup fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{})
			testutil.FailOn(t, db, "query", "o_auth_accounts")
			return NewUserAuthService(db)
		}},
		{"linked user lookup fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{})
			if _, err := NewUserAuthService(db).FindOrCreateGoogleUser(context.Background(), googleInfo); err != nil {
				t.Fatal(err)
			}
			testutil.FailOn(t, db, "query", "users")
			return NewUserAuthService(db)
		}},
		{"user create fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{})
			testutil.FailOn(t, db, "create", "users")
			return NewUserAuthService(db)
		}},
		{"starter list create fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{})
			testutil.FailOn(t, db, "create", "watchlists")
			return NewUserAuthService(db)
		}},
		{"oauth account create fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{})
			testutil.FailOn(t, db, "create", "o_auth_accounts")
			return NewUserAuthService(db)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc := tc.setup(t)
			user, err := svc.FindOrCreateGoogleUser(context.Background(), googleInfo)
			if !errors.Is(err, testutil.ErrInjected) || user != nil {
				t.Errorf("expected injected error and nil user, got %v, %v", user, err)
			}
			if tc.name == "oauth account create fails" || tc.name == "starter list create fails" {
				var n int64
				svc.DB.Model(&domain.User{}).Count(&n)
				if n != 0 {
					t.Errorf("expected user insert to roll back, found %d users", n)
				}
			}
		})
	}
}

// ---- local (password) accounts --------------------------------------------

var localRegistration = LocalUserRegistration{
	Email:       "Grace@Example.com",
	Username:    "Grace.H",
	DisplayName: "Grace H",
	Password:    "correct horse battery",
}

func newLocalService(t *testing.T) (*UserAuthService, *gorm.DB) {
	t.Helper()
	db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{}, &domain.Session{})
	return NewUserAuthService(db), db
}

func TestRegisterLocalUser(t *testing.T) {
	svc, db := newLocalService(t)
	ctx := context.Background()

	user, err := svc.RegisterLocalUser(ctx, localRegistration)
	if err != nil {
		t.Fatal(err)
	}
	if user.Email != "grace@example.com" || user.Username != "grace.h" || user.DisplayName != "Grace H" {
		t.Errorf("registration did not normalize the account: %+v", user)
	}
	if user.PasswordHash == "" || user.PasswordHash == localRegistration.Password {
		t.Error("the password was not hashed")
	}
	if !user.HasPassword {
		t.Error("HasPassword should be set on a password account")
	}
	if !CheckPassword(user.PasswordHash, localRegistration.Password) {
		t.Error("the stored hash does not match the password")
	}
	if user.IsEmailVerified {
		t.Error("a local account's e-mail is not verified (there is no mailer)")
	}

	var lists []domain.Watchlist
	if err := db.Where("owner_id = ?", user.ID).Order("title").Find(&lists).Error; err != nil {
		t.Fatal(err)
	}
	if len(lists) != 2 || lists[0].Title != "Movies to watch" || lists[1].Title != "Shows to watch" {
		t.Errorf("expected the two starter lists, got %+v", lists)
	}

	// The same password signs in, by e-mail or username, in any case.
	for _, identifier := range []string{"grace@example.com", "GRACE.H", " grace.h "} {
		signedIn, err := svc.AuthenticateLocal(ctx, identifier, localRegistration.Password)
		if err != nil {
			t.Fatalf("sign-in as %q failed: %v", identifier, err)
		}
		if signedIn.ID != user.ID {
			t.Errorf("sign-in as %q returned %v, want %v", identifier, signedIn.ID, user.ID)
		}
	}
}

func TestRegisterLocalUserDisplayNameDefaultsToUsername(t *testing.T) {
	svc, _ := newLocalService(t)
	in := localRegistration
	in.DisplayName = "   "
	user, err := svc.RegisterLocalUser(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	if user.DisplayName != "grace.h" {
		t.Errorf("expected the username as the display name, got %q", user.DisplayName)
	}
}

func TestRegisterLocalUserRejections(t *testing.T) {
	cases := []struct {
		name    string
		mutate  func(*LocalUserRegistration)
		wantErr error
	}{
		{"no at sign", func(in *LocalUserRegistration) { in.Email = "nope" }, svcerr.ErrInvalid},
		{"empty local part", func(in *LocalUserRegistration) { in.Email = "@example.com" }, svcerr.ErrInvalid},
		{"empty domain", func(in *LocalUserRegistration) { in.Email = "grace@" }, svcerr.ErrInvalid},
		{"invalid username", func(in *LocalUserRegistration) { in.Username = "Grace H!" }, svcerr.ErrInvalid},
		{"weak password", func(in *LocalUserRegistration) { in.Password = "short" }, svcerr.ErrInvalid},
		{"display name too long", func(in *LocalUserRegistration) { in.DisplayName = strings.Repeat("a", 51) }, svcerr.ErrInvalid},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _ := newLocalService(t)
			in := localRegistration
			tc.mutate(&in)
			user, err := svc.RegisterLocalUser(context.Background(), in)
			if !errors.Is(err, tc.wantErr) || user != nil {
				t.Fatalf("expected %v and no user, got %v and %v", tc.wantErr, err, user)
			}
		})
	}
}

func TestRegisterLocalUserDuplicates(t *testing.T) {
	svc, _ := newLocalService(t)
	ctx := context.Background()
	if _, err := svc.RegisterLocalUser(ctx, localRegistration); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		mutate func(*LocalUserRegistration)
		msg    string
	}{
		{"same e-mail", func(in *LocalUserRegistration) { in.Email = "GRACE@example.com"; in.Username = "another.name" }, "That e-mail already has an account"},
		{"same username", func(in *LocalUserRegistration) { in.Email = "other@example.com"; in.Username = "grace.h" }, "That username is already taken"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := localRegistration
			tc.mutate(&in)
			_, err := svc.RegisterLocalUser(ctx, in)
			if !errors.Is(err, svcerr.ErrDuplicate) {
				t.Fatalf("expected a duplicate error, got %v", err)
			}
			if svcerr.Message(err) != tc.msg {
				t.Errorf("expected message %q, got %q", tc.msg, svcerr.Message(err))
			}
		})
	}
}

func TestRegisterLocalUserErrors(t *testing.T) {
	cases := []struct {
		name  string
		table string
		op    string
	}{
		{"e-mail lookup fails", "users", "query"},
		{"user create fails", "users", "create"},
		{"starter list create fails", "watchlists", "create"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, db := newLocalService(t)
			testutil.FailOn(t, db, tc.op, tc.table)
			user, err := svc.RegisterLocalUser(context.Background(), localRegistration)
			if !errors.Is(err, testutil.ErrInjected) || user != nil {
				t.Fatalf("expected the injected error, got %v and %v", err, user)
			}
		})
	}
}

func TestAuthenticateLocalFailures(t *testing.T) {
	svc, _ := newLocalService(t)
	ctx := context.Background()
	if _, err := svc.RegisterLocalUser(ctx, localRegistration); err != nil {
		t.Fatal(err)
	}
	// A Google-only account cannot sign in with a password.
	if _, err := svc.FindOrCreateGoogleUser(ctx, googleInfo); err != nil {
		t.Fatal(err)
	}

	cases := []string{
		`grace.h` + "|" + `wrong password here`,
		`stranger@example.com|correct horse battery`,
		`a@example.com|correct horse battery`, // the Google account: no local password
		`|`,
	}
	for _, tc := range cases {
		identifier, password, _ := strings.Cut(tc, "|")
		user, err := svc.AuthenticateLocal(ctx, identifier, password)
		if !errors.Is(err, ErrInvalidCredentials) || user != nil {
			t.Errorf("expected invalid credentials for %q, got %v and %v", tc, err, user)
		}
	}
}

func TestAuthenticateLocalRecordsLastLogin(t *testing.T) {
	svc, db := newLocalService(t)
	ctx := context.Background()
	user, err := svc.RegisterLocalUser(ctx, localRegistration)
	if err != nil {
		t.Fatal(err)
	}
	signedIn, err := svc.AuthenticateLocal(ctx, "grace.h", localRegistration.Password)
	if err != nil {
		t.Fatal(err)
	}
	if signedIn.LastLoginAt == nil {
		t.Fatal("last_login_at should be set on sign-in")
	}
	var stored domain.User
	if err := db.First(&stored, "id = ?", user.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.LastLoginAt == nil {
		t.Error("last_login_at was not persisted")
	}
}

func TestAuthenticateLocalErrors(t *testing.T) {
	cases := []struct {
		name  string
		table string
		op    string
	}{
		{"lookup fails", "users", "query"},
		{"last login update fails", "users", "update"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, db := newLocalService(t)
			ctx := context.Background()
			if _, err := svc.RegisterLocalUser(ctx, localRegistration); err != nil {
				t.Fatal(err)
			}
			testutil.FailOn(t, db, tc.op, tc.table)
			user, err := svc.AuthenticateLocal(ctx, "grace.h", localRegistration.Password)
			if !errors.Is(err, testutil.ErrInjected) || user != nil {
				t.Fatalf("expected the injected error, got %v and %v", err, user)
			}
		})
	}
}

func TestSetPassword(t *testing.T) {
	svc, db := newLocalService(t)
	ctx := context.Background()

	// A Google-only account can gain a password without supplying one.
	googleUser, err := svc.FindOrCreateGoogleUser(ctx, googleInfo)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.SetPassword(ctx, googleUser.ID, "", "a brand new password"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.AuthenticateLocal(ctx, googleInfo.Email, "a brand new password"); err != nil {
		t.Fatalf("the new password does not work: %v", err)
	}

	// Changing it again requires the current password.
	if err := svc.SetPassword(ctx, googleUser.ID, "a brand new password", "another password 1"); err != nil {
		t.Fatal(err)
	}
	err = svc.SetPassword(ctx, googleUser.ID, "stale password", "yet another password")
	if !errors.Is(err, svcerr.ErrInvalid) {
		t.Fatalf("expected a validation error for a wrong current password, got %v", err)
	}
	err = svc.SetPassword(ctx, googleUser.ID, "another password 1", "short")
	if !errors.Is(err, svcerr.ErrInvalid) {
		t.Fatalf("expected a validation error for a weak new password, got %v", err)
	}
	if err := svc.SetPassword(ctx, uuid.New(), "", "another password 1"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("expected not found for an unknown user, got %v", err)
	}

	// The hash is stored, not the password.
	var stored domain.User
	if err := db.First(&stored, "id = ?", googleUser.ID).Error; err != nil {
		t.Fatal(err)
	}
	if stored.PasswordHash == "" || stored.PasswordHash == "another password 1" {
		t.Error("the password was not hashed")
	}
}

func TestSetPasswordUpdateFailure(t *testing.T) {
	svc, db := newLocalService(t)
	ctx := context.Background()
	user, err := svc.RegisterLocalUser(ctx, localRegistration)
	if err != nil {
		t.Fatal(err)
	}
	testutil.FailOn(t, db, "update", "users")
	if err := svc.SetPassword(ctx, user.ID, localRegistration.Password, "a brand new password"); !errors.Is(err, testutil.ErrInjected) {
		t.Fatalf("expected the injected error, got %v", err)
	}
}

func TestRegisterLocalUserConcurrentSameEmail(t *testing.T) {
	// Two registrations racing for the same e-mail: the pre-check cannot see
	// the other insert, so the unique index is what decides.
	db := testutil.NewFileDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Watchlist{}, &domain.Session{})
	svc := NewUserAuthService(db)

	usernames := []string{"first.name", "second.name"}
	errs := make([]error, len(usernames))
	var wg sync.WaitGroup
	for i, name := range usernames {
		wg.Add(1)
		go func(i int, name string) {
			defer wg.Done()
			in := localRegistration
			in.Username = name
			_, errs[i] = svc.RegisterLocalUser(context.Background(), in)
		}(i, name)
	}
	wg.Wait()

	created, duplicated := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			created++
		case errors.Is(err, svcerr.ErrDuplicate):
			duplicated++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if created != 1 || duplicated != 1 {
		t.Fatalf("expected one registration to win and one to conflict, got %d and %d (%v)", created, duplicated, errs)
	}
	var users int64
	db.Model(&domain.User{}).Count(&users)
	if users != 1 {
		t.Errorf("expected a single account, found %d", users)
	}
}
