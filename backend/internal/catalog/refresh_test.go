package catalog

import (
	"context"
	"errors"
	"net/http"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestEnsureSeasonServesStoredCopyWhenTMDBIsDown(t *testing.T) {
	var hits atomic.Int32
	status := &sync.Map{}
	s := newUpstreamService(t, &hits, status)
	ctx := context.Background()

	if _, err := s.EnsureSeason(ctx, 1396, 1); err != nil {
		t.Fatal(err)
	}
	status.Store("/tv/1396/season/1", http.StatusInternalServerError)

	season, err := s.EnsureSeason(ctx, 1396, 1)
	if err != nil {
		t.Fatalf("expected stored season, got %v", err)
	}
	if len(season.Episodes) != 2 || season.Episodes[0].Title != "Pilot" {
		t.Errorf("stored episodes wrong: %+v", season.Episodes)
	}
}

func TestRefreshPrunesSeasonsAndEpisodesTMDBNoLongerLists(t *testing.T) {
	var hits atomic.Int32
	status := &sync.Map{}
	s := newUpstreamService(t, &hits, status)
	ctx := context.Background()

	if _, err := s.EnsureSeason(ctx, 1396, 1); err != nil {
		t.Fatal(err)
	}
	var seasons, episodes int64
	s.DB.Model(&domain.Season{}).Count(&seasons)
	s.DB.Model(&domain.Episode{}).Count(&episodes)
	if seasons != 2 || episodes != 2 {
		t.Fatalf("setup: seasons=%d episodes=%d", seasons, episodes)
	}

	// TMDB drops episode 2 of season 1 ...
	status.Store("/tv/1396/season/1", `{"season_number":1,"name":"Season 1","episodes":[{"episode_number":1,"name":"Pilot"}]}`)
	season, err := s.EnsureSeason(ctx, 1396, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(season.Episodes) != 1 {
		t.Errorf("removed episode should be pruned, got %d episodes", len(season.Episodes))
	}

	// ... and season 2 (which gets an episode of its own first).
	s.DB.Create(&domain.Episode{ShowID: season.ShowID, SeasonID: 2, Title: "S2E1", EpisodeNumber: 1})
	ageOut(t, s.DB, &domain.Show{})
	status.Store("/tv/1396", `{"id":1396,"name":"Breaking Bad","seasons":[{"season_number":1,"name":"Season 1"}],"external_ids":{"tvdb_id":81189}}`)
	show, err := s.EnsureShow(ctx, 1396)
	if err != nil {
		t.Fatal(err)
	}
	if len(show.Seasons) != 1 || show.Seasons[0].SeasonNumber != 1 {
		t.Errorf("removed season should be pruned: %+v", show.Seasons)
	}
	var orphaned int64
	s.DB.Model(&domain.Episode{}).Where("season_id = ?", 2).Count(&orphaned)
	if orphaned != 0 {
		t.Errorf("episodes of a pruned season should go too, got %d", orphaned)
	}
}

func TestRefreshWithNoSeasonsOrEpisodesPrunesNothing(t *testing.T) {
	var hits atomic.Int32
	status := &sync.Map{}
	s := newUpstreamService(t, &hits, status)
	ctx := context.Background()

	if _, err := s.EnsureSeason(ctx, 1396, 1); err != nil {
		t.Fatal(err)
	}
	ageOut(t, s.DB, &domain.Show{})
	status.Store("/tv/1396", `{"id":1396,"name":"Breaking Bad","seasons":[],"external_ids":{}}`)
	status.Store("/tv/1396/season/1", `{"season_number":1,"episodes":[]}`)

	if _, err := s.EnsureShow(ctx, 1396); err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnsureSeason(ctx, 1396, 1); err != nil {
		t.Fatal(err)
	}
	var seasons, episodes int64
	s.DB.Model(&domain.Season{}).Count(&seasons)
	s.DB.Model(&domain.Episode{}).Count(&episodes)
	if seasons != 2 || episodes != 2 {
		t.Errorf("an empty TMDB answer must not wipe stored data: seasons=%d episodes=%d", seasons, episodes)
	}
}

func TestConcurrentRequestsDoNotDuplicateOrFail(t *testing.T) {
	var hits atomic.Int32
	s := newTestService(t, &hits)
	// In-memory shared-cache SQLite fails concurrent cross-table access with
	// "table is locked"; a WAL file database behaves like a real server.
	path := filepath.Join(t.TempDir(), "concurrent.db")
	db, err := gorm.Open(sqlite.Open(path+"?_busy_timeout=10000&_journal_mode=WAL&_txlock=immediate"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.Movie{}, &domain.Show{}, &domain.Season{}, &domain.Episode{}, &domain.Genre{}, &domain.Review{}); err != nil {
		t.Fatal(err)
	}
	s.DB = db
	ctx := context.Background()

	var wg sync.WaitGroup
	errs := make(chan error, 24)
	for i := 0; i < 8; i++ {
		wg.Add(3)
		go func() { defer wg.Done(); _, err := s.EnsureMovie(ctx, 603); errs <- err }()
		go func() { defer wg.Done(); _, err := s.EnsureShow(ctx, 1396); errs <- err }()
		go func() { defer wg.Done(); _, err := s.EnsureSeason(ctx, 1396, 1); errs <- err }()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Errorf("concurrent request failed: %v", err)
		}
	}

	counts := map[string]int64{}
	for name, model := range map[string]any{"movies": &domain.Movie{}, "shows": &domain.Show{}, "seasons": &domain.Season{}, "episodes": &domain.Episode{}} {
		var n int64
		s.DB.Model(model).Count(&n)
		counts[name] = n
	}
	if counts["movies"] != 1 || counts["shows"] != 1 || counts["seasons"] != 2 || counts["episodes"] != 2 {
		t.Errorf("duplicate rows created: %v", counts)
	}
	if s.locks.Len() != 0 {
		t.Errorf("idle locks should be released, %d left", s.locks.Len())
	}
}

func TestPruneFailuresAreReported(t *testing.T) {
	const shrunkShow = `{"id":1396,"name":"Breaking Bad","seasons":[{"season_number":1,"name":"Season 1"}],"external_ids":{"tvdb_id":81189}}`
	const shrunkSeason = `{"season_number":1,"episodes":[{"episode_number":1,"name":"Pilot"}]}`

	setup := func(t *testing.T) (*Service, *sync.Map) {
		var hits atomic.Int32
		status := &sync.Map{}
		s := newUpstreamService(t, &hits, status)
		if _, err := s.EnsureSeason(context.Background(), 1396, 1); err != nil {
			t.Fatal(err)
		}
		ageOut(t, s.DB, &domain.Show{})
		status.Store("/tv/1396", shrunkShow)
		status.Store("/tv/1396/season/1", shrunkSeason)
		return s, status
	}

	showCases := []dbFailure{
		{"find stale seasons", "query", "seasons", 2},
		{"delete stale episodes", "delete", "episodes", 0},
		{"delete stale seasons", "delete", "seasons", 0},
	}
	for _, tc := range showCases {
		t.Run("show/"+tc.name, func(t *testing.T) {
			s, _ := setup(t)
			testutil.FailAfter(t, s.DB, tc.op, tc.table, tc.after)
			if _, err := s.EnsureShow(context.Background(), 1396); !errors.Is(err, testutil.ErrInjected) {
				t.Errorf("expected injected error, got %v", err)
			}
		})
	}

	t.Run("season/delete stale episodes", func(t *testing.T) {
		s, _ := setup(t)
		testutil.FailOn(t, s.DB, "delete", "episodes")
		if _, err := s.EnsureSeason(context.Background(), 1396, 1); !errors.Is(err, testutil.ErrInjected) {
			t.Errorf("expected injected error, got %v", err)
		}
	})
}
