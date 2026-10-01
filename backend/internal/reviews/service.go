// Package reviews implements ratings and reviews of movies and shows. Reviews
// are public to every signed-in user; only their author can change or delete
// them. Each user can review a title once.
package reviews

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/keyedlock"
	"github.com/aomarai/concession/internal/svcerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	MinRating = 1
	MaxRating = 10

	maxTitleLen   = 200
	maxContentLen = 10000

	DefaultPerPage = 20
	MaxPerPage     = 100
	// MaxPage bounds the page number so (page-1)*perPage cannot overflow into
	// a negative OFFSET, which the database would reject with a 500.
	MaxPage = 1_000_000
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

// Author is the public part of a reviewer's profile (never their email).
type Author struct {
	ID          uuid.UUID `json:"id"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url,omitempty"`
}

// View is a review as returned by the API. Movie or Show is set when the
// review is listed outside its title's own page.
type View struct {
	ID        uuid.UUID             `json:"id"`
	ItemType  domain.ReviewableItem `json:"item_type"`
	ItemID    uint64                `json:"item_id"`
	Rating    int                   `json:"rating"`
	Title     string                `json:"title"`
	Content   string                `json:"content"`
	Author    Author                `json:"author"`
	CreatedAt time.Time             `json:"created_at"`
	UpdatedAt time.Time             `json:"updated_at"`
	Movie     *domain.Movie         `json:"movie,omitempty"`
	Show      *domain.Show          `json:"show,omitempty"`
}

// Summary aggregates a title's ratings. Average is rounded to one decimal and
// is 0 when there are no reviews.
type Summary struct {
	Count   int64   `json:"count"`
	Average float64 `json:"average"`
}

// Page is one page of reviews. Summary is set for per-title listings.
type Page struct {
	Reviews []View   `json:"reviews"`
	Summary *Summary `json:"summary,omitempty"`
	Page    int      `json:"page"`
	PerPage int      `json:"per_page"`
	Total   int64    `json:"total"`
}

type Input struct {
	Rating  int
	Title   string
	Content string
}

type UpdateInput struct {
	Rating  *int
	Title   *string
	Content *string
}

func validRating(r int) error {
	if r < MinRating || r > MaxRating {
		return svcerr.Invalid(fmt.Sprintf("rating must be between %d and %d", MinRating, MaxRating))
	}
	return nil
}

func validText(title, content string) (string, string, error) {
	title, content = strings.TrimSpace(title), strings.TrimSpace(content)
	if utf8.RuneCountInString(title) > maxTitleLen {
		return "", "", svcerr.Invalid("title is too long")
	}
	if utf8.RuneCountInString(content) > maxContentLen {
		return "", "", svcerr.Invalid("review text is too long")
	}
	return title, content, nil
}

// normalizePage applies defaults (0 means "unset") and limits.
func normalizePage(page, perPage int) (int, int, error) {
	if page == 0 {
		page = 1
	}
	if perPage == 0 {
		perPage = DefaultPerPage
	}
	if page < 1 || perPage < 1 {
		return 0, 0, svcerr.Invalid("page and per_page must be positive")
	}
	if page > MaxPage {
		return 0, 0, svcerr.Invalid("page is too large")
	}
	return page, min(perPage, MaxPerPage), nil
}

func itemType(kind domain.ReviewableItem) domain.ItemType { return domain.ItemType(kind) }

// Create adds the user's review of a title, fetching the title from TMDB if it
// is not stored yet. A second review of the same title is ErrDuplicate.
func (s *Service) Create(ctx context.Context, userID uuid.UUID, kind domain.ReviewableItem, tmdbID int64, in Input) (*View, error) {
	if err := validRating(in.Rating); err != nil {
		return nil, err
	}
	title, content, err := validText(in.Title, in.Content)
	if err != nil {
		return nil, err
	}

	var itemID uint64
	if kind == domain.ReviewableMovies {
		m, err := s.Catalog.EnsureMovie(ctx, tmdbID)
		if err != nil {
			return nil, err
		}
		itemID = m.ID
	} else {
		sh, err := s.Catalog.EnsureShow(ctx, tmdbID)
		if err != nil {
			return nil, err
		}
		itemID = sh.ID
	}

	defer s.locks.Lock(fmt.Sprintf("%s:%s:%d", userID, kind, itemID))()
	db := s.DB.WithContext(ctx)
	var existing int64
	if err := db.Model(&domain.Review{}).
		Where("user_id = ? AND reviewable_type = ? AND reviewable_id = ?", userID, kind, itemID).
		Count(&existing).Error; err != nil {
		return nil, err
	}
	if existing > 0 {
		return nil, svcerr.ErrDuplicate
	}

	r := domain.Review{UserID: userID, Rating: uint8(in.Rating), Title: title, Content: content, ReviewableID: itemID, ReviewableType: kind}
	if err := db.Omit("User").Create(&r).Error; err != nil {
		return nil, err
	}
	views, err := s.views(ctx, []domain.Review{r}, false)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

func (s *Service) find(ctx context.Context, id uuid.UUID) (domain.Review, error) {
	var r domain.Review
	err := s.DB.WithContext(ctx).First(&r, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r, svcerr.ErrNotFound
	}
	return r, err
}

// ownReview loads a review the caller is allowed to change.
func (s *Service) ownReview(ctx context.Context, userID, id uuid.UUID) (domain.Review, error) {
	r, err := s.find(ctx, id)
	if err != nil {
		return r, err
	}
	if r.UserID != userID {
		return r, svcerr.ErrForbidden
	}
	return r, nil
}

// Get returns one review with its title attached.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (*View, error) {
	r, err := s.find(ctx, id)
	if err != nil {
		return nil, err
	}
	views, err := s.views(ctx, []domain.Review{r}, true)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// Update changes the caller's own review.
func (s *Service) Update(ctx context.Context, userID, id uuid.UUID, in UpdateInput) (*View, error) {
	r, err := s.ownReview(ctx, userID, id)
	if err != nil {
		return nil, err
	}
	if in.Rating != nil {
		if err := validRating(*in.Rating); err != nil {
			return nil, err
		}
		r.Rating = uint8(*in.Rating)
	}
	title, content := r.Title, r.Content
	if in.Title != nil {
		title = *in.Title
	}
	if in.Content != nil {
		content = *in.Content
	}
	if r.Title, r.Content, err = validText(title, content); err != nil {
		return nil, err
	}
	if err := s.DB.WithContext(ctx).Omit("User").Save(&r).Error; err != nil {
		return nil, err
	}
	views, err := s.views(ctx, []domain.Review{r}, false)
	if err != nil {
		return nil, err
	}
	return &views[0], nil
}

// Delete permanently removes the caller's own review, so they can review the
// title again later.
func (s *Service) Delete(ctx context.Context, userID, id uuid.UUID) error {
	r, err := s.ownReview(ctx, userID, id)
	if err != nil {
		return err
	}
	return s.DB.WithContext(ctx).Unscoped().Delete(&domain.Review{}, "id = ?", r.ID).Error
}

// ListForTitle returns a page of reviews for a stored title with its rating
// summary. A title that was never stored simply has no reviews.
func (s *Service) ListForTitle(ctx context.Context, kind domain.ReviewableItem, tmdbID int64, page, perPage int) (*Page, error) {
	page, perPage, err := normalizePage(page, perPage)
	if err != nil {
		return nil, err
	}
	out := &Page{Reviews: []View{}, Summary: &Summary{}, Page: page, PerPage: perPage}

	itemID, err := domain.StoredTitleID(ctx, s.DB, itemType(kind), tmdbID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}

	db := s.DB.WithContext(ctx)
	where := "reviewable_type = ? AND reviewable_id = ?"
	var agg struct {
		N   int64
		Avg *float64
	}
	if err := db.Model(&domain.Review{}).Select("COUNT(*) AS n, AVG(rating) AS avg").
		Where(where, kind, itemID).Scan(&agg).Error; err != nil {
		return nil, err
	}
	out.Total = agg.N
	out.Summary.Count = agg.N
	if agg.Avg != nil {
		out.Summary.Average = math.Round(*agg.Avg*10) / 10
	}

	var rows []domain.Review
	if err := db.Where(where, kind, itemID).Order("created_at DESC, id").
		Limit(perPage).Offset((page - 1) * perPage).Find(&rows).Error; err != nil {
		return nil, err
	}
	if out.Reviews, err = s.views(ctx, rows, false); err != nil {
		return nil, err
	}
	return out, nil
}

// ListMine returns a page of the caller's own reviews with their titles.
func (s *Service) ListMine(ctx context.Context, userID uuid.UUID, page, perPage int) (*Page, error) {
	page, perPage, err := normalizePage(page, perPage)
	if err != nil {
		return nil, err
	}
	db := s.DB.WithContext(ctx)
	out := &Page{Page: page, PerPage: perPage}
	if err := db.Model(&domain.Review{}).Where("user_id = ?", userID).Count(&out.Total).Error; err != nil {
		return nil, err
	}
	var rows []domain.Review
	if err := db.Where("user_id = ?", userID).Order("created_at DESC, id").
		Limit(perPage).Offset((page - 1) * perPage).Find(&rows).Error; err != nil {
		return nil, err
	}
	if out.Reviews, err = s.views(ctx, rows, true); err != nil {
		return nil, err
	}
	return out, nil
}

// views converts rows to API views, attaching authors and, if withTitles,
// the reviewed movie or show.
func (s *Service) views(ctx context.Context, rows []domain.Review, withTitles bool) ([]View, error) {
	db := s.DB.WithContext(ctx)

	userIDs := make([]uuid.UUID, 0, len(rows))
	var movieIDs, showIDs []uint64
	for _, r := range rows {
		userIDs = append(userIDs, r.UserID)
		if r.ReviewableType == domain.ReviewableMovies {
			movieIDs = append(movieIDs, r.ReviewableID)
		} else {
			showIDs = append(showIDs, r.ReviewableID)
		}
	}
	authors := make(map[uuid.UUID]Author, len(rows))
	if len(userIDs) > 0 {
		var users []domain.User
		if err := db.Select("id", "display_name", "avatar_url").Where("id IN ?", userIDs).Find(&users).Error; err != nil {
			return nil, err
		}
		for _, u := range users {
			authors[u.ID] = Author{ID: u.ID, DisplayName: u.DisplayName, AvatarURL: u.AvatarURL}
		}
	}
	var movies map[uint64]*domain.Movie
	var shows map[uint64]*domain.Show
	if withTitles {
		var err error
		if movies, shows, err = domain.LoadTitles(ctx, s.DB, movieIDs, showIDs); err != nil {
			return nil, err
		}
	}

	out := make([]View, len(rows))
	for i, r := range rows {
		out[i] = View{
			ID: r.ID, ItemType: r.ReviewableType, ItemID: r.ReviewableID, Rating: int(r.Rating),
			Title: r.Title, Content: r.Content, CreatedAt: r.CreatedAt, UpdatedAt: r.UpdatedAt,
			Author: authors[r.UserID],
		}
		if out[i].Author.ID == uuid.Nil {
			out[i].Author.ID = r.UserID // author account no longer exists
		}
		if r.ReviewableType == domain.ReviewableMovies {
			out[i].Movie = movies[r.ReviewableID]
		} else {
			out[i].Show = shows[r.ReviewableID]
		}
	}
	return out, nil
}
