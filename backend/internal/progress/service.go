// Package progress tracks each user's personal watch status for movies and
// shows (plan to watch, watching, completed, dropped) and, for shows, the last
// season and episode reached. It is independent of watchlists.
package progress

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/keyedlock"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Catalog resolves TMDB IDs to stored titles; *catalog.Service implements it.
type Catalog interface {
	EnsureMovie(ctx context.Context, tmdbID int64) (*domain.Movie, error)
	EnsureShow(ctx context.Context, tmdbID int64) (*domain.Show, error)
}

type Service struct {
	DB      *gorm.DB
	Catalog Catalog

	locks keyedlock.Locks
}

func NewService(db *gorm.DB, catalog Catalog) *Service {
	return &Service{DB: db, Catalog: catalog}
}

// View is a progress record together with its title.
type View struct {
	ItemType       domain.ItemType    `json:"item_type"`
	ItemID         uint64             `json:"item_id"`
	Status         domain.WatchStatus `json:"status"`
	LastSeasonNum  uint32             `json:"last_season_num"`
	LastEpisodeNum uint32             `json:"last_episode_num"`
	WatchedAt      time.Time          `json:"watched_at"`
	Movie          *domain.Movie      `json:"movie,omitempty"`
	Show           *domain.Show       `json:"show,omitempty"`
}

type SetInput struct {
	Status  domain.WatchStatus
	Season  uint32 // shows only
	Episode uint32 // shows only; requires Season
}

// ParseKind converts a URL segment ("movies" or "shows") to an ItemType.
func ParseKind(s string) (domain.ItemType, bool) {
	switch s {
	case "movies":
		return domain.ItemTypeMovie, true
	case "shows":
		return domain.ItemTypeShow, true
	}
	return "", false
}

func validStatus(st domain.WatchStatus) bool {
	switch st {
	case domain.StatusPlanToWatch, domain.StatusWatching, domain.StatusCompleted, domain.StatusDropped:
		return true
	}
	return false
}

func validate(kind domain.ItemType, in SetInput) error {
	if !validStatus(in.Status) {
		return svcerr.Invalid("status must be plan_to_watch, watching, completed or dropped")
	}
	if kind == domain.ItemTypeMovie && (in.Season != 0 || in.Episode != 0) {
		return svcerr.Invalid("season and episode only apply to shows")
	}
	if in.Episode > 0 && in.Season == 0 {
		return svcerr.Invalid("episode requires season")
	}
	return nil
}

// Set creates or updates the user's progress for a title, fetching the title
// from TMDB if it is not stored yet.
func (s *Service) Set(ctx context.Context, userID uuid.UUID, kind domain.ItemType, tmdbID int64, in SetInput) (*View, error) {
	if err := validate(kind, in); err != nil {
		return nil, err
	}

	var itemID uint64
	var movie *domain.Movie
	var show *domain.Show
	if kind == domain.ItemTypeMovie {
		m, err := s.Catalog.EnsureMovie(ctx, tmdbID)
		if err != nil {
			return nil, err
		}
		itemID, movie = m.ID, m
	} else {
		sh, err := s.Catalog.EnsureShow(ctx, tmdbID)
		if err != nil {
			return nil, err
		}
		itemID, show = sh.ID, sh
	}

	defer s.locks.Lock(fmt.Sprintf("%s:%s:%d", userID, kind, itemID))()
	db := s.DB.WithContext(ctx)
	var p domain.UserWatchProgress
	err := db.Where("user_id = ? AND item_type = ? AND item_id = ?", userID, kind, itemID).First(&p).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	p.UserID, p.ItemType, p.ItemID = userID, kind, itemID
	p.Status, p.LastSeasonNum, p.LastEpisodeNum = in.Status, in.Season, in.Episode
	p.WatchedAt = time.Now()
	if err := db.Save(&p).Error; err != nil {
		return nil, err
	}
	return &View{
		ItemType: kind, ItemID: itemID, Status: p.Status, LastSeasonNum: p.LastSeasonNum,
		LastEpisodeNum: p.LastEpisodeNum, WatchedAt: p.WatchedAt, Movie: movie, Show: show,
	}, nil
}

// titleID finds the internal ID of an already stored title by TMDB ID.
func (s *Service) titleID(ctx context.Context, kind domain.ItemType, tmdbID int64) (uint64, error) {
	id, err := domain.StoredTitleID(ctx, s.DB, kind, tmdbID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, svcerr.ErrNotFound
	}
	return id, err
}

// Get returns the user's progress for a title. A title that was never tracked
// (or never stored) is ErrNotFound.
func (s *Service) Get(ctx context.Context, userID uuid.UUID, kind domain.ItemType, tmdbID int64) (*View, error) {
	itemID, err := s.titleID(ctx, kind, tmdbID)
	if err != nil {
		return nil, err
	}
	var p domain.UserWatchProgress
	err = s.DB.WithContext(ctx).Where("user_id = ? AND item_type = ? AND item_id = ?", userID, kind, itemID).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, svcerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	views, err := s.attach(ctx, []domain.UserWatchProgress{p})
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// Delete stops tracking a title. Deleting something that is not tracked is
// ErrNotFound.
func (s *Service) Delete(ctx context.Context, userID uuid.UUID, kind domain.ItemType, tmdbID int64) error {
	itemID, err := s.titleID(ctx, kind, tmdbID)
	if err != nil {
		return err
	}
	res := s.DB.WithContext(ctx).Where("user_id = ? AND item_type = ? AND item_id = ?", userID, kind, itemID).
		Delete(&domain.UserWatchProgress{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return svcerr.ErrNotFound
	}
	return nil
}

// Filter narrows List; zero values match everything.
type Filter struct {
	Kind   domain.ItemType
	Status domain.WatchStatus
}

// List returns the user's progress, most recently updated first.
func (s *Service) List(ctx context.Context, userID uuid.UUID, f Filter) ([]View, error) {
	if f.Status != "" && !validStatus(f.Status) {
		return nil, svcerr.Invalid("status must be plan_to_watch, watching, completed or dropped")
	}
	q := s.DB.WithContext(ctx).Where("user_id = ?", userID)
	if f.Kind != "" {
		q = q.Where("item_type = ?", f.Kind)
	}
	if f.Status != "" {
		q = q.Where("status = ?", f.Status)
	}
	var rows []domain.UserWatchProgress
	if err := q.Order("watched_at DESC, id").Find(&rows).Error; err != nil {
		return nil, err
	}
	return s.attach(ctx, rows)
}

// attach loads the movies and shows the rows refer to.
func (s *Service) attach(ctx context.Context, rows []domain.UserWatchProgress) ([]View, error) {
	var movieIDs, showIDs []uint64
	for _, r := range rows {
		if r.ItemType == domain.ItemTypeMovie {
			movieIDs = append(movieIDs, r.ItemID)
		} else {
			showIDs = append(showIDs, r.ItemID)
		}
	}
	movies, shows, err := domain.LoadTitles(ctx, s.DB, movieIDs, showIDs)
	if err != nil {
		return nil, err
	}
	out := make([]View, len(rows))
	for i, r := range rows {
		out[i] = View{
			ItemType: r.ItemType, ItemID: r.ItemID, Status: r.Status, LastSeasonNum: r.LastSeasonNum,
			LastEpisodeNum: r.LastEpisodeNum, WatchedAt: r.WatchedAt,
		}
		if r.ItemType == domain.ItemTypeMovie {
			out[i].Movie = movies[r.ItemID]
		} else {
			out[i].Show = shows[r.ItemID]
		}
	}
	return out, nil
}
