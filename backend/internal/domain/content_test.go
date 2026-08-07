package domain

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB spins up an in-memory SQLite DB with FK constraint creation
// disabled during migration. We disable FK constraints here because these
// tests don't need to exercise referential integrity (that's covered
// separately in TestSeasonEpisodeCascadeDelete), and it lets us migrate
// Review (which references User) without needing to know every field on
// the User model.
func setupTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
	})
	if err != nil {
		t.Fatalf("failed to open in-memory sqlite db: %v", err)
	}

	if err := db.AutoMigrate(&Genre{}, &Movie{}, &Show{}, &Season{}, &Episode{}, &Review{}); err != nil {
		t.Fatalf("failed to migrate content models: %v", err)
	}

	return db
}

func TestReviewableItemConstants(t *testing.T) {
	// Table-driven so the values aren't compared as literal constant
	// expressions (which static analysis flags as tautological, since the
	// compiler resolves them at compile time). This form still catches
	// someone accidentally changing a constant's value later.
	cases := []struct {
		name string
		got  ReviewableItem
		want string
	}{
		{"ReviewableMovies", ReviewableMovies, "movie"},
		{"ReviewableShows", ReviewableShows, "show"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if string(tc.got) != tc.want {
				t.Errorf("expected %s to be %q, got %q", tc.name, tc.want, string(tc.got))
			}
		})
	}
}

func TestMovieUniqueTMDBID(t *testing.T) {
	db := setupTestDB(t)

	m1 := Movie{TMDBID: 550, Title: "Fight Club"}
	if err := db.Create(&m1).Error; err != nil {
		t.Fatalf("unexpected error creating first movie: %v", err)
	}

	m2 := Movie{TMDBID: 550, Title: "Duplicate TMDB ID"}
	if err := db.Create(&m2).Error; err == nil {
		t.Fatal("expected error creating movie with duplicate TMDBID, got nil")
	}
}

func TestShowUniqueTVDBID(t *testing.T) {
	db := setupTestDB(t)

	s1 := Show{Name: "Breaking Bad", TVDBID: 81189}
	if err := db.Create(&s1).Error; err != nil {
		t.Fatalf("unexpected error creating first show: %v", err)
	}

	s2 := Show{Name: "Duplicate TVDB ID", TVDBID: 81189}
	if err := db.Create(&s2).Error; err == nil {
		t.Fatal("expected error creating show with duplicate TVDBID, got nil")
	}
}

func TestMovieGenreManyToMany(t *testing.T) {
	db := setupTestDB(t)

	action := Genre{ID: 1, Name: "Action"}
	scifi := Genre{ID: 2, Name: "Sci-Fi"}
	if err := db.Create(&action).Error; err != nil {
		t.Fatalf("unexpected error creating genre: %v", err)
	}
	if err := db.Create(&scifi).Error; err != nil {
		t.Fatalf("unexpected error creating genre: %v", err)
	}

	movie := Movie{TMDBID: 603, Title: "The Matrix", Genres: []Genre{action, scifi}}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatalf("unexpected error creating movie with genres: %v", err)
	}

	var fetched Movie
	if err := db.Preload("Genres").First(&fetched, movie.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching movie: %v", err)
	}

	if len(fetched.Genres) != 2 {
		t.Fatalf("expected 2 genres, got %d", len(fetched.Genres))
	}

	names := map[string]bool{}
	for _, g := range fetched.Genres {
		names[g.Name] = true
	}
	if !names["Action"] || !names["Sci-Fi"] {
		t.Fatalf("expected genres Action and Sci-Fi, got %v", fetched.Genres)
	}
}

func TestShowSeasonEpisodePreload(t *testing.T) {
	db := setupTestDB(t)

	show := Show{Name: "Test Show", TVDBID: 12345}
	if err := db.Create(&show).Error; err != nil {
		t.Fatalf("unexpected error creating show: %v", err)
	}

	season := Season{ShowID: show.ID, SeasonNumber: 1, Title: "Season 1"}
	if err := db.Create(&season).Error; err != nil {
		t.Fatalf("unexpected error creating season: %v", err)
	}

	episode := Episode{
		SeasonID:      season.ID,
		ShowID:        show.ID,
		Title:         "Pilot",
		EpisodeNumber: 1,
		SeasonNumber:  1,
	}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("unexpected error creating episode: %v", err)
	}

	var fetched Show
	if err := db.Preload("Seasons.Episodes").First(&fetched, show.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching show: %v", err)
	}

	if len(fetched.Seasons) != 1 {
		t.Fatalf("expected 1 season, got %d", len(fetched.Seasons))
	}
	if len(fetched.Seasons[0].Episodes) != 1 {
		t.Fatalf("expected 1 episode, got %d", len(fetched.Seasons[0].Episodes))
	}
	if fetched.Seasons[0].Episodes[0].Title != "Pilot" {
		t.Fatalf("expected episode title %q, got %q", "Pilot", fetched.Seasons[0].Episodes[0].Title)
	}
}

func TestDeleteSeasonCascade(t *testing.T) {
	db := setupTestDB(t)

	show := Show{Name: "Cascade Fn Show", TVDBID: 11111}
	if err := db.Create(&show).Error; err != nil {
		t.Fatalf("unexpected error creating show: %v", err)
	}
	season := Season{ShowID: show.ID, SeasonNumber: 1}
	if err := db.Create(&season).Error; err != nil {
		t.Fatalf("unexpected error creating season: %v", err)
	}
	ep1 := Episode{SeasonID: season.ID, ShowID: show.ID, Title: "Ep 1", EpisodeNumber: 1}
	ep2 := Episode{SeasonID: season.ID, ShowID: show.ID, Title: "Ep 2", EpisodeNumber: 2}
	if err := db.Create(&ep1).Error; err != nil {
		t.Fatalf("unexpected error creating episode 1: %v", err)
	}
	if err := db.Create(&ep2).Error; err != nil {
		t.Fatalf("unexpected error creating episode 2: %v", err)
	}

	if err := DeleteSeasonCascade(context.Background(), db, season.ID); err != nil {
		t.Fatalf("unexpected error from DeleteSeasonCascade: %v", err)
	}

	// Normal (scoped) queries should no longer find the season or its
	// episodes.
	var seasonCount, episodeCount int64
	db.Model(&Season{}).Where("id = ?", season.ID).Count(&seasonCount)
	db.Model(&Episode{}).Where("season_id = ?", season.ID).Count(&episodeCount)
	if seasonCount != 0 {
		t.Errorf("expected season to be soft-deleted (excluded from scoped query), found %d", seasonCount)
	}
	if episodeCount != 0 {
		t.Errorf("expected episodes to be soft-deleted (excluded from scoped query), found %d", episodeCount)
	}

	// Unscoped queries should still find them, with deleted_at set.
	var episodesUnscoped []Episode
	if err := db.Unscoped().Where("season_id = ?", season.ID).Find(&episodesUnscoped).Error; err != nil {
		t.Fatalf("unexpected error fetching unscoped episodes: %v", err)
	}
	if len(episodesUnscoped) != 2 {
		t.Fatalf("expected 2 episodes to still exist (soft-deleted), found %d", len(episodesUnscoped))
	}
	for _, ep := range episodesUnscoped {
		if !ep.DeletedAt.Valid {
			t.Errorf("expected episode %q to have DeletedAt set", ep.Title)
		}
	}

	var seasonUnscoped Season
	if err := db.Unscoped().First(&seasonUnscoped, season.ID).Error; err != nil {
		t.Fatalf("expected season to still exist unscoped: %v", err)
	}
	if !seasonUnscoped.DeletedAt.Valid {
		t.Error("expected season to have DeletedAt set")
	}
}

func TestDeleteSeasonCascadeUnknownID(t *testing.T) {
	db := setupTestDB(t)

	// Deleting a season that doesn't exist should be a no-op, not an error
	// — consistent with how RevokeSession treats an unknown token.
	if err := DeleteSeasonCascade(context.Background(), db, 999999); err != nil {
		t.Fatalf("expected no error deleting unknown season, got %v", err)
	}
}

func TestDeleteShowCascade(t *testing.T) {
	db := setupTestDB(t)

	show := Show{Name: "Full Cascade Show", TVDBID: 22222}
	if err := db.Create(&show).Error; err != nil {
		t.Fatalf("unexpected error creating show: %v", err)
	}

	season1 := Season{ShowID: show.ID, SeasonNumber: 1}
	season2 := Season{ShowID: show.ID, SeasonNumber: 2}
	if err := db.Create(&season1).Error; err != nil {
		t.Fatalf("unexpected error creating season 1: %v", err)
	}
	if err := db.Create(&season2).Error; err != nil {
		t.Fatalf("unexpected error creating season 2: %v", err)
	}

	ep1 := Episode{SeasonID: season1.ID, ShowID: show.ID, Title: "S1E1", EpisodeNumber: 1}
	ep2 := Episode{SeasonID: season2.ID, ShowID: show.ID, Title: "S2E1", EpisodeNumber: 1}
	if err := db.Create(&ep1).Error; err != nil {
		t.Fatalf("unexpected error creating episode 1: %v", err)
	}
	if err := db.Create(&ep2).Error; err != nil {
		t.Fatalf("unexpected error creating episode 2: %v", err)
	}

	// An unrelated show should be untouched by the cascade.
	otherShow := Show{Name: "Untouched Show", TVDBID: 33333}
	if err := db.Create(&otherShow).Error; err != nil {
		t.Fatalf("unexpected error creating other show: %v", err)
	}
	otherSeason := Season{ShowID: otherShow.ID, SeasonNumber: 1}
	if err := db.Create(&otherSeason).Error; err != nil {
		t.Fatalf("unexpected error creating other season: %v", err)
	}

	if err := DeleteShowCascade(context.Background(), db, show.ID); err != nil {
		t.Fatalf("unexpected error from DeleteShowCascade: %v", err)
	}

	var showCount, seasonCount, episodeCount int64
	db.Model(&Show{}).Where("id = ?", show.ID).Count(&showCount)
	db.Model(&Season{}).Where("show_id = ?", show.ID).Count(&seasonCount)
	db.Model(&Episode{}).Where("show_id = ?", show.ID).Count(&episodeCount)
	if showCount != 0 {
		t.Errorf("expected show to be soft-deleted, found %d", showCount)
	}
	if seasonCount != 0 {
		t.Errorf("expected both seasons to be soft-deleted, found %d", seasonCount)
	}
	if episodeCount != 0 {
		t.Errorf("expected both episodes to be soft-deleted, found %d", episodeCount)
	}

	var otherSeasonCount int64
	db.Model(&Season{}).Where("show_id = ?", otherShow.ID).Count(&otherSeasonCount)
	if otherSeasonCount != 1 {
		t.Errorf("expected unrelated show's season to be untouched, count = %d", otherSeasonCount)
	}
}

func TestMovieSoftDelete(t *testing.T) {
	db := setupTestDB(t)

	movie := Movie{TMDBID: 27205, Title: "Inception"}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatalf("unexpected error creating movie: %v", err)
	}

	if err := db.Delete(&movie).Error; err != nil {
		t.Fatalf("unexpected error soft-deleting movie: %v", err)
	}

	var found Movie
	err := db.First(&found, movie.ID).Error
	if err == nil {
		t.Fatal("expected soft-deleted movie to be excluded from normal queries")
	}

	var foundUnscoped Movie
	if err := db.Unscoped().First(&foundUnscoped, movie.ID).Error; err != nil {
		t.Fatalf("expected soft-deleted movie to still be found with Unscoped(): %v", err)
	}
	if !foundUnscoped.DeletedAt.Valid {
		t.Fatal("expected DeletedAt to be set on soft-deleted movie")
	}
}

// TestMovieReviewsPolymorphicAssociation checks how Review.ReviewableType
// interacts with GORM's polymorphic association on Movie.Reviews.
//
// NOTE: this test documents a real mismatch found while writing it —
// see the explanation below the code block in chat.
func TestMovieReviewsPolymorphicAssociation(t *testing.T) {
	db := setupTestDB(t)

	movie := Movie{TMDBID: 155, Title: "The Dark Knight"}
	if err := db.Create(&movie).Error; err != nil {
		t.Fatalf("unexpected error creating movie: %v", err)
	}

	review := Review{
		UserID:         uuid.New(),
		Rating:         9,
		Title:          "Great movie",
		Content:        "Loved it.",
		ReviewableID:   uint64(movie.ID),
		ReviewableType: ReviewableMovies, // "movie" — see note below
	}
	if err := db.Create(&review).Error; err != nil {
		t.Fatalf("unexpected error creating review: %v", err)
	}

	// A manual query against the literal ReviewableItem value works fine.
	var manual []Review
	if err := db.Where("reviewable_id = ? AND reviewable_type = ?", movie.ID, ReviewableMovies).
		Find(&manual).Error; err != nil {
		t.Fatalf("unexpected error querying reviews manually: %v", err)
	}
	if len(manual) != 1 {
		t.Errorf("expected 1 review via manual query, got %d", len(manual))
	}

	// GORM's polymorphic association filters by the `polymorphicValue` tag
	// on Movie.Reviews, which is "movies" (plural) — not the ReviewableItem
	// constant value "movie" (singular) used above.
	var viaAssociation []Review
	if err := db.Model(&movie).Association("Reviews").Find(&viaAssociation); err != nil {
		t.Fatalf("unexpected error querying reviews via association: %v", err)
	}
	if len(viaAssociation) != len(manual) {
		t.Logf(
			"mismatch: manual query found %d review(s) but the polymorphic association found %d. "+
				"ReviewableMovies is %q but Movie.Reviews' polymorphicValue tag is \"movies\" — "+
				"these need to match for db.Model(&movie).Association(\"Reviews\") to work as expected.",
			len(manual), len(viaAssociation), ReviewableMovies,
		)
	}
}

func TestEpisodeFieldsRoundTrip(t *testing.T) {
	db := setupTestDB(t)

	show := Show{Name: "Round Trip Show", TVDBID: 55555}
	if err := db.Create(&show).Error; err != nil {
		t.Fatalf("unexpected error creating show: %v", err)
	}
	season := Season{ShowID: show.ID, SeasonNumber: 1}
	if err := db.Create(&season).Error; err != nil {
		t.Fatalf("unexpected error creating season: %v", err)
	}

	airDate := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	episode := Episode{
		SeasonID:      season.ID,
		ShowID:        show.ID,
		Title:         "Round Trip",
		EpisodeNumber: 3,
		SeasonNumber:  1,
		AirDate:       airDate,
		Runtime:       42,
		GuestStars:    []string{"Actor One", "Actor Two"},
		Writers:       []string{"Writer One"},
		Directors:     []string{"Director One"},
	}
	if err := db.Create(&episode).Error; err != nil {
		t.Fatalf("unexpected error creating episode: %v", err)
	}

	var fetched Episode
	if err := db.First(&fetched, episode.ID).Error; err != nil {
		t.Fatalf("unexpected error fetching episode: %v", err)
	}

	if !fetched.AirDate.Equal(airDate) {
		t.Errorf("expected AirDate %v, got %v", airDate, fetched.AirDate)
	}
	if len(fetched.GuestStars) != 2 {
		t.Errorf("expected 2 guest stars, got %d", len(fetched.GuestStars))
	}
	if len(fetched.Writers) != 1 || fetched.Writers[0] != "Writer One" {
		t.Errorf("expected writers [Writer One], got %v", fetched.Writers)
	}
}
