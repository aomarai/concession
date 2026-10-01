package domain

import (
	"testing"

	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func TestCollaboratorUniquePerListAndUser(t *testing.T) {
	db := testutil.NewDB(t, &Collaborator{})
	list, user := uuid.New(), uuid.New()
	if err := db.Create(&Collaborator{UserID: user, WatchlistID: list}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&Collaborator{UserID: user, WatchlistID: list, Role: RoleViewer}).Error; err == nil {
		t.Error("a user can be on a list only once")
	}
	if err := db.Create(&Collaborator{UserID: uuid.New(), WatchlistID: list}).Error; err != nil {
		t.Errorf("another user on the same list: %v", err)
	}
	if err := db.Create(&Collaborator{UserID: user, WatchlistID: uuid.New()}).Error; err != nil {
		t.Errorf("the same user on another list: %v", err)
	}
}

func TestCollaboratorStatusDefaultsToAccepted(t *testing.T) {
	db := testutil.NewDB(t, &Collaborator{})
	direct := Collaborator{UserID: uuid.New(), WatchlistID: uuid.New()}
	if err := db.Create(&direct).Error; err != nil {
		t.Fatal(err)
	}
	pending := Collaborator{UserID: uuid.New(), WatchlistID: uuid.New(), Status: CollaboratorPending}
	if err := db.Create(&pending).Error; err != nil {
		t.Fatal(err)
	}
	var got1, got2 Collaborator
	db.First(&got1, "id = ?", direct.ID)
	db.First(&got2, "id = ?", pending.ID)
	if got1.Status != CollaboratorAccepted || got2.Status != CollaboratorPending {
		t.Errorf("statuses: %q and %q", got1.Status, got2.Status)
	}
}

func TestShareTokenHelpers(t *testing.T) {
	a, err := NewShareToken()
	if err != nil || a == "" {
		t.Fatalf("got %q, %v", a, err)
	}
	if b, _ := NewShareToken(); a == b {
		t.Error("tokens must be unique")
	}
	SetRandReadForTest(func([]byte) (int, error) { return 0, errTest })
	if _, err := NewShareToken(); err == nil {
		t.Error("expected an error from a failing entropy source")
	}
	ResetRandReadForTest()
	if _, err := NewShareToken(); err != nil {
		t.Errorf("real entropy source restored: %v", err)
	}
}

type testError string

func (e testError) Error() string { return string(e) }

var errTest = testError("no entropy")
