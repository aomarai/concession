package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
)

var googleInfo = GoogleUserInfo{ID: "g-1", Email: "a@example.com", VerifiedEmail: true, Name: "Ann", Picture: "http://pic"}

func TestFindOrCreateGoogleUserCreatesThenFinds(t *testing.T) {
	db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{})
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
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{})
			testutil.FailOn(t, db, "query", "o_auth_accounts")
			return NewUserAuthService(db)
		}},
		{"linked user lookup fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{})
			if _, err := NewUserAuthService(db).FindOrCreateGoogleUser(context.Background(), googleInfo); err != nil {
				t.Fatal(err)
			}
			testutil.FailOn(t, db, "query", "users")
			return NewUserAuthService(db)
		}},
		{"user create fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{})
			testutil.FailOn(t, db, "create", "users")
			return NewUserAuthService(db)
		}},
		{"oauth account create fails", func(t *testing.T) *UserAuthService {
			db := testutil.NewDB(t, &domain.User{}, &domain.OAuthAccount{})
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
			if tc.name == "oauth account create fails" {
				var n int64
				svc.DB.Model(&domain.User{}).Count(&n)
				if n != 0 {
					t.Errorf("expected user insert to roll back, found %d users", n)
				}
			}
		})
	}
}
