package domain

import (
	"context"
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func TestLoadPublicUsers(t *testing.T) {
	db := testutil.NewDB(t, &User{})
	ann := User{Username: "ann", Email: "ann@example.com", DisplayName: "Ann", AvatarURL: "http://pic"}
	if err := db.Create(&ann).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()

	got, err := LoadPublicUsers(ctx, db, []uuid.UUID{ann.ID, uuid.New()})
	if err != nil || len(got) != 1 {
		t.Fatalf("got %v, %v", got, err)
	}
	if p := got[ann.ID]; p.DisplayName != "Ann" || p.AvatarURL != "http://pic" || p.ID != ann.ID {
		t.Errorf("profile: %+v", p)
	}
	if empty, err := LoadPublicUsers(ctx, db, nil); err != nil || len(empty) != 0 {
		t.Errorf("no ids: %v, %v", empty, err)
	}

	missing := uuid.New()
	if p := PublicUserOrID(got, missing); p.ID != missing || p.DisplayName != "" {
		t.Errorf("missing user should fall back to a bare ID: %+v", p)
	}
	if p := PublicUserOrID(got, ann.ID); p.DisplayName != "Ann" {
		t.Errorf("known user: %+v", p)
	}

	testutil.FailOn(t, db, "query", "users")
	if _, err := LoadPublicUsers(ctx, db, []uuid.UUID{ann.ID}); !errors.Is(err, testutil.ErrInjected) {
		t.Errorf("db failure: %v", err)
	}
}
