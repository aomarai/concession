package domain

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func TestOrderedPairIsCanonical(t *testing.T) {
	for i := 0; i < 50; i++ {
		a, b := uuid.New(), uuid.New()
		lo1, hi1 := OrderedPair(a, b)
		lo2, hi2 := OrderedPair(b, a)
		if lo1 != lo2 || hi1 != hi2 {
			t.Fatalf("order depends on argument order for %v, %v", a, b)
		}
		if bytes.Compare(lo1[:], hi1[:]) > 0 {
			t.Fatalf("pair not ascending: %v, %v", lo1, hi1)
		}
	}
	same := uuid.New()
	if lo, hi := OrderedPair(same, same); lo != same || hi != same {
		t.Error("a pair of equal IDs stays as is")
	}
}

func TestFriendshipHelpers(t *testing.T) {
	a, b := uuid.New(), uuid.New()
	lo, hi := OrderedPair(a, b)
	f := Friendship{UserAID: lo, UserBID: hi, RequestedBy: a}
	if f.Other(a) != b || f.Other(b) != a {
		t.Error("Other returned the wrong member")
	}
	if f.Addressee() != b {
		t.Error("the addressee is the user who did not send the request")
	}
	f.RequestedBy = b
	if f.Addressee() != a {
		t.Error("addressee follows RequestedBy")
	}
}

func TestFriendshipUniquePerPair(t *testing.T) {
	db := testutil.NewDB(t, &Friendship{})
	a, b := uuid.New(), uuid.New()
	lo, hi := OrderedPair(a, b)
	if err := db.Create(&Friendship{UserAID: lo, UserBID: hi, RequestedBy: a}).Error; err != nil {
		t.Fatal(err)
	}
	// The reverse request lands on the same canonical pair.
	lo2, hi2 := OrderedPair(b, a)
	if err := db.Create(&Friendship{UserAID: lo2, UserBID: hi2, RequestedBy: b}).Error; err == nil {
		t.Error("a pair of users can have only one friendship row")
	}
	var got Friendship
	db.First(&got)
	if got.Status != FriendshipPending {
		t.Errorf("status should default to pending, got %q", got.Status)
	}
}

func TestFindUserByIdentifier(t *testing.T) {
	db := testutil.NewDB(t, &User{})
	u := User{Username: "ann", Email: "Ann@Example.com", DisplayName: "Ann"}
	if err := db.Create(&u).Error; err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for _, id := range []string{"ann", "  ann  ", "ann@example.com", "ANN@EXAMPLE.COM"} {
		got, err := FindUserByIdentifier(ctx, db, id)
		if err != nil || got.ID != u.ID {
			t.Errorf("%q: %v, %v", id, got.ID, err)
		}
	}
	for _, id := range []string{"", "bob", "Ann"} { // username match is exact (case-sensitive)
		if _, err := FindUserByIdentifier(ctx, db, id); !errors.Is(err, gorm.ErrRecordNotFound) {
			t.Errorf("%q: expected not found, got %v", id, err)
		}
	}
}
