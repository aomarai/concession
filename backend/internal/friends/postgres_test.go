package friends

import (
	"errors"
	"testing"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/aomarai/concession/internal/userref"
	"gorm.io/gorm/clause"
)

// Friendships have no foreign key to users, so the database cannot stop a
// request being written to someone whose account is being deleted. Send has to
// coordinate with the purge itself, which this checks on a real Postgres
// (skipped unless TEST_POSTGRES_DSN is set).
func TestSendWaitsForAnAccountDeletionInProgress(t *testing.T) {
	db := testutil.NewPostgresDB(t, &domain.User{}, &domain.OAuthAccount{}, &domain.Session{}, &domain.Friendship{})
	svc := NewService(db)
	svc.Notifier = &recorder{}
	a := domain.User{Username: "a", Email: "a@example.com", DisplayName: "A"}
	b := domain.User{Username: "b", Email: "b@example.com", DisplayName: "B"}
	if err := db.Create(&a).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&b).Error; err != nil {
		t.Fatal(err)
	}

	// The purge has locked b's row (what domain.PurgeUser does first) and is
	// still working.
	purge := db.Begin()
	if err := purge.Clauses(clause.Locking{Strength: "UPDATE"}).Select("id").Where("id = ?", b.ID).First(&domain.User{}).Error; err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() {
		_, err := svc.Send(ctx, a.ID, userref.Ref{ID: b.ID})
		done <- err
	}()
	select {
	case err := <-done:
		_ = purge.Rollback()
		t.Fatalf("Send finished (%v) while b's deletion was in progress; it must wait", err)
	case <-time.After(500 * time.Millisecond):
	}

	// The purge commits: b no longer exists, so the request must fail rather
	// than leave a row pointing at nobody.
	if err := purge.Unscoped().Where("id = ?", b.ID).Delete(&domain.User{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := purge.Commit().Error; err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if !errors.Is(err, svcerr.ErrNotFound) {
			t.Errorf("Send = %v, want not found", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Send never finished after the deletion committed")
	}
	var n int64
	db.Model(&domain.Friendship{}).Count(&n)
	if n != 0 {
		t.Errorf("%d friendship rows were left behind", n)
	}
}
