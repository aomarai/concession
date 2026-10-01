package domain

import (
	"testing"

	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
)

func TestReviewUniquePerUserAndTitle(t *testing.T) {
	db := testutil.NewDB(t, &Review{})
	user := uuid.New()
	base := Review{UserID: user, Rating: 8, ReviewableID: 1, ReviewableType: ReviewableMovies}
	first := base
	if err := db.Create(&first).Error; err != nil {
		t.Fatal(err)
	}

	dup := base
	if err := db.Create(&dup).Error; err == nil {
		t.Error("a second review of the same title by the same user should violate the unique index")
	}
	other := base
	other.UserID = uuid.New()
	if err := db.Create(&other).Error; err != nil {
		t.Errorf("another user may review the same title: %v", err)
	}
	show := base
	show.ReviewableType = ReviewableShows
	if err := db.Create(&show).Error; err != nil {
		t.Errorf("a show with the same ID is a different title: %v", err)
	}
	otherTitle := base
	otherTitle.ReviewableID = 2
	if err := db.Create(&otherTitle).Error; err != nil {
		t.Errorf("another title: %v", err)
	}
}
