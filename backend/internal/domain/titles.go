package domain

import (
	"context"

	"gorm.io/gorm"
)

// StoredTitleID returns the internal ID of an already stored movie or show
// with the given TMDB ID, or gorm.ErrRecordNotFound. It never calls TMDB.
func StoredTitleID(ctx context.Context, db *gorm.DB, kind ItemType, tmdbID int64) (uint64, error) {
	var id uint64
	q := db.WithContext(ctx)
	if kind == ItemTypeMovie {
		q = q.Model(&Movie{})
	} else {
		q = q.Model(&Show{})
	}
	err := q.Select("id").Where("tmdb_id = ?", tmdbID).Take(&id).Error
	return id, err
}

// LoadTitles fetches the movies and shows with the given internal IDs, keyed
// by ID. Empty ID lists skip their query.
func LoadTitles(ctx context.Context, db *gorm.DB, movieIDs, showIDs []uint64) (map[uint64]*Movie, map[uint64]*Show, error) {
	db = db.WithContext(ctx)
	movies := make(map[uint64]*Movie, len(movieIDs))
	shows := make(map[uint64]*Show, len(showIDs))
	if len(movieIDs) > 0 {
		var ms []Movie
		if err := db.Where("id IN ?", movieIDs).Find(&ms).Error; err != nil {
			return nil, nil, err
		}
		for i := range ms {
			movies[ms[i].ID] = &ms[i]
		}
	}
	if len(showIDs) > 0 {
		var ss []Show
		if err := db.Where("id IN ?", showIDs).Find(&ss).Error; err != nil {
			return nil, nil, err
		}
		for i := range ss {
			shows[ss[i].ID] = &ss[i]
		}
	}
	return movies, shows, nil
}
