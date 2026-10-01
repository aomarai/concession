// Package catalog fetches titles from TMDB and persists them locally so the
// rest of the app (watchlists, reviews) can reference stable internal IDs.
package catalog

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/tmdb"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// freshFor is how long a locally stored title is served without re-fetching.
const freshFor = 24 * time.Hour

const maxActors = 10

type Service struct {
	DB   *gorm.DB
	TMDB *tmdb.Client
}

func NewService(db *gorm.DB, client *tmdb.Client) *Service {
	return &Service{DB: db, TMDB: client}
}

func parseDate(s string) time.Time {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return time.Time{}
	}
	return t
}

func actors(c tmdb.Credits) []string {
	out := make([]string, 0, maxActors)
	for _, a := range c.Cast {
		if len(out) == maxActors {
			break
		}
		out = append(out, a.Name)
	}
	return out
}

func toGenres(in []tmdb.Genre) []domain.Genre {
	out := make([]domain.Genre, len(in))
	for i, g := range in {
		out[i] = domain.Genre{ID: g.ID, Name: g.Name}
	}
	return out
}

func upsertGenres(tx *gorm.DB, genres []domain.Genre) error {
	if len(genres) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{"name"}),
	}).Create(&genres).Error
}

// Search proxies TMDB multi-search (movies and shows only).
func (s *Service) Search(ctx context.Context, query string, page int) (*tmdb.SearchResponse, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return &tmdb.SearchResponse{Results: []tmdb.SearchResult{}}, nil
	}
	return s.TMDB.SearchMulti(ctx, query, page)
}

// SyncGenres stores the current TMDB movie and TV genre lists.
func (s *Service) SyncGenres(ctx context.Context) error {
	genres, err := s.TMDB.Genres(ctx)
	if err != nil {
		return err
	}
	return upsertGenres(s.DB.WithContext(ctx), toGenres(genres))
}

// EnsureMovie returns the movie with the given TMDB ID, fetching and storing
// it if it is missing or stale.
func (s *Service) EnsureMovie(ctx context.Context, tmdbID int64) (*domain.Movie, error) {
	db := s.DB.WithContext(ctx)

	var existing domain.Movie
	err := db.Preload("Genres").Where("tmdb_id = ?", tmdbID).First(&existing).Error
	switch {
	case err == nil && time.Since(existing.UpdatedAt) < freshFor:
		return &existing, nil
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	remote, err := s.TMDB.GetMovie(ctx, tmdbID)
	if err != nil {
		if existing.ID != 0 && !errors.Is(err, tmdb.ErrNotFound) {
			return &existing, nil // serve stale data if TMDB is unavailable
		}
		return nil, err
	}

	movie := existing // keeps ID/CreatedAt on refresh
	movie.TMDBID = remote.ID
	movie.IMDBID = remote.IMDBID
	movie.Title = remote.Title
	movie.OriginalTitle = remote.OriginalTitle
	movie.Overview = remote.Overview
	movie.Tagline = remote.Tagline
	movie.PosterPath = remote.PosterPath
	movie.BackdropPath = remote.BackdropPath
	movie.ReleaseDate = parseDate(remote.ReleaseDate)
	movie.Revenue = remote.Revenue
	movie.Budget = remote.Budget
	movie.Runtime = remote.Runtime
	movie.Popularity = remote.Popularity
	movie.VoteAverage = remote.VoteAverage
	movie.VoteCount = remote.VoteCount
	movie.Actors = actors(remote.Credits)
	genres := toGenres(remote.Genres)
	movie.Genres = nil

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := upsertGenres(tx, genres); err != nil {
			return err
		}
		movie.UpdatedAt = time.Now()
		if err := tx.Omit("Genres", "Reviews").Save(&movie).Error; err != nil {
			return err
		}
		return tx.Model(&movie).Association("Genres").Replace(genres)
	})
	if err != nil {
		return nil, err
	}
	movie.Genres = genres
	return &movie, nil
}

// EnsureShow returns the show with the given TMDB ID, fetching and storing it
// (with season summaries) if it is missing or stale. The TVDB ID is taken from
// TMDB's external IDs.
func (s *Service) EnsureShow(ctx context.Context, tmdbID int64) (*domain.Show, error) {
	db := s.DB.WithContext(ctx)

	var existing domain.Show
	err := db.Preload("Genres").Preload("Seasons", func(d *gorm.DB) *gorm.DB {
		return d.Order("season_number")
	}).Where("tmdb_id = ?", tmdbID).First(&existing).Error
	switch {
	case err == nil && time.Since(existing.UpdatedAt) < freshFor:
		return &existing, nil
	case err != nil && !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, err
	}

	remote, err := s.TMDB.GetShow(ctx, tmdbID)
	if err != nil {
		if existing.ID != 0 && !errors.Is(err, tmdb.ErrNotFound) {
			return &existing, nil
		}
		return nil, err
	}

	show := existing
	id := remote.ID
	show.TMDBID = &id
	show.TVDBID = remote.ExternalIDs.TVDBID
	show.IMDBID = remote.ExternalIDs.IMDBID
	show.Name = remote.Name
	show.Overview = remote.Overview
	show.ContentRating = remote.ContentRating()
	show.Actors = actors(remote.Credits)
	genres := toGenres(remote.Genres)
	show.Genres = nil
	show.Seasons = nil

	err = db.Transaction(func(tx *gorm.DB) error {
		if err := upsertGenres(tx, genres); err != nil {
			return err
		}
		show.UpdatedAt = time.Now()
		if err := tx.Omit("Genres", "Seasons", "Reviews").Save(&show).Error; err != nil {
			return err
		}
		if err := tx.Model(&show).Association("Genres").Replace(genres); err != nil {
			return err
		}
		for _, rs := range remote.Seasons {
			if _, err := upsertSeason(tx, show.ID, rs.SeasonNumber, rs.Name, rs.Overview, rs.PosterPath, rs.AirDate); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var out domain.Show
	if err := db.Preload("Genres").Preload("Seasons", func(d *gorm.DB) *gorm.DB {
		return d.Order("season_number")
	}).First(&out, show.ID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

func upsertSeason(tx *gorm.DB, showID uint64, number int, title, overview, poster, airDate string) (domain.Season, error) {
	var season domain.Season
	err := tx.Where("show_id = ? AND season_number = ?", showID, number).First(&season).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return season, err
	}
	season.ShowID = showID
	season.SeasonNumber = number
	season.Title = title
	season.Overview = overview
	season.PosterPath = poster
	season.AirDate = parseDate(airDate)
	return season, tx.Omit("Episodes").Save(&season).Error
}

// EnsureSeason returns one season of a show (identified by the show's TMDB ID)
// with its episodes, refreshing episodes from TMDB.
func (s *Service) EnsureSeason(ctx context.Context, showTMDBID int64, number int) (*domain.Season, error) {
	show, err := s.EnsureShow(ctx, showTMDBID)
	if err != nil {
		return nil, err
	}
	remote, err := s.TMDB.GetSeason(ctx, showTMDBID, number)
	if err != nil {
		return nil, err
	}

	db := s.DB.WithContext(ctx)
	var seasonID uint64
	err = db.Transaction(func(tx *gorm.DB) error {
		season, err := upsertSeason(tx, show.ID, remote.SeasonNumber, remote.Name, remote.Overview, remote.PosterPath, remote.AirDate)
		if err != nil {
			return err
		}
		seasonID = season.ID

		for _, re := range remote.Episodes {
			var ep domain.Episode
			err := tx.Where("season_id = ? AND episode_number = ?", season.ID, re.EpisodeNumber).First(&ep).Error
			if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			ep.SeasonID = season.ID
			ep.ShowID = show.ID
			ep.EpisodeNumber = uint32(re.EpisodeNumber)
			ep.SeasonNumber = uint32(re.SeasonNumber)
			ep.Title = re.Name
			ep.Overview = re.Overview
			ep.AirDate = parseDate(re.AirDate)
			ep.Runtime = re.Runtime
			ep.GuestStars = names(re.GuestStars, "")
			ep.Writers = names(re.Crew, "Writer")
			ep.Directors = names(re.Crew, "Director")
			if err := tx.Save(&ep).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	var out domain.Season
	if err := db.Preload("Episodes", func(d *gorm.DB) *gorm.DB {
		return d.Order("episode_number")
	}).First(&out, seasonID).Error; err != nil {
		return nil, err
	}
	return &out, nil
}

// names returns the names of people with the given job ("" matches everyone).
func names(people []tmdb.Person, job string) []string {
	out := []string{}
	for _, p := range people {
		if job == "" || p.Job == job {
			out = append(out, p.Name)
		}
	}
	return out
}
