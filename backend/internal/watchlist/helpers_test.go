package watchlist

import (
	"context"
	"fmt"
	"testing"

	"github.com/aomarai/concession/internal/domain"
	"github.com/aomarai/concession/internal/testutil"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func models() []any {
	return []any{
		&domain.User{}, &domain.Movie{}, &domain.Show{}, &domain.Genre{}, &domain.Review{},
		&domain.Watchlist{}, &domain.WatchlistItem{}, &domain.Collaborator{},
	}
}

// fakeCatalog stores a placeholder title per TMDB ID, or fails with err.
type fakeCatalog struct {
	db  *gorm.DB
	err error
}

func (f *fakeCatalog) EnsureMovie(_ context.Context, id int64) (*domain.Movie, error) {
	if f.err != nil {
		return nil, f.err
	}
	m := domain.Movie{TMDBID: id, Title: fmt.Sprintf("Movie %d", id)}
	return &m, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&m).Error
}

func (f *fakeCatalog) EnsureShow(_ context.Context, id int64) (*domain.Show, error) {
	if f.err != nil {
		return nil, f.err
	}
	tv := id
	s := domain.Show{TMDBID: &id, TVDBID: &tv, Name: fmt.Sprintf("Show %d", id)}
	return &s, f.db.Where("tmdb_id = ?", id).FirstOrCreate(&s).Error
}

type env struct {
	svc   *Service
	db    *gorm.DB
	cat   *fakeCatalog
	owner uuid.UUID
}

func newEnv(t *testing.T) *env {
	t.Helper()
	db := testutil.NewDB(t, models()...)
	cat := &fakeCatalog{db: db}
	return &env{svc: NewService(db, cat), db: db, cat: cat, owner: uuid.New()}
}

func (e *env) newList(t *testing.T, typ domain.WatchlistType) uuid.UUID {
	t.Helper()
	w, err := e.svc.Create(context.Background(), e.owner, CreateInput{Title: "List", Type: typ})
	if err != nil {
		t.Fatal(err)
	}
	return w.ID
}

// member makes a new user a collaborator with the given role.
func (e *env) member(t *testing.T, listID uuid.UUID, role domain.CollaboratorRole) uuid.UUID {
	t.Helper()
	u := uuid.New()
	if err := e.db.Create(&domain.Collaborator{UserID: u, WatchlistID: listID, Role: role}).Error; err != nil {
		t.Fatal(err)
	}
	return u
}

func (e *env) addMovies(t *testing.T, listID uuid.UUID, tmdbIDs ...int64) []uuid.UUID {
	t.Helper()
	var ids []uuid.UUID
	for _, id := range tmdbIDs {
		it, err := e.svc.AddItem(context.Background(), e.owner, listID, id, "")
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, it.ID)
	}
	return ids
}

func (e *env) order(t *testing.T, listID uuid.UUID) []uuid.UUID {
	t.Helper()
	d, err := e.svc.Get(context.Background(), e.owner, listID)
	if err != nil {
		t.Fatal(err)
	}
	var ids []uuid.UUID
	for _, it := range d.Items {
		ids = append(ids, it.ID)
	}
	return ids
}
