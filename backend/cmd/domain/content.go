package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

type Review struct {
	ID             uuid.UUID      `json:"id" gorm:"type:uuid;primaryKey"`
	UserID         uuid.UUID      `json:"user_id" gorm:"type:uuid;not null;index"`
	Rating         uint8          `json:"rating" gorm:"not null"` // e.g. 1-10 or 1-5
	Title          string         `json:"title"`
	Content        string         `json:"content"`
	ReviewableID   uint64         `json:"reviewable_id" gorm:"not null;index"`
	ReviewableType string         `json:"reviewable_type" gorm:"not null;index"` // e.g. "movies" or "shows"
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	User User `json:"user,omitempty" gorm:"foreignKey:UserID"`
}

type Movie struct {
	ID            uint64                      `json:"id" gorm:"primaryKey"`
	TMDBID        int64                       `json:"tmdb_id" gorm:"uniqueIndex;not null"`
	IMDBID        string                      `json:"imdb_id" gorm:"index"`
	Title         string                      `json:"title" gorm:"not null"`
	OriginalTitle string                      `json:"original_title"`
	Overview      string                      `json:"overview"`
	Tagline       string                      `json:"tagline"`
	PosterPath    string                      `json:"poster_path"`
	BackdropPath  string                      `json:"backdrop_path"`
	ReleaseDate   time.Time                   `json:"release_date"`
	Revenue       int64                       `json:"revenue"`
	Budget        int64                       `json:"budget"`
	Runtime       int                         `json:"runtime"`
	Popularity    float32                     `json:"popularity"`
	VoteAverage   float32                     `json:"vote_average"`
	VoteCount     int64                       `json:"vote_count"`
	Actors        datatypes.JSONSlice[string] `json:"actors"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
	DeletedAt     gorm.DeletedAt              `json:"deleted_at" gorm:"index"`

	// Relationships
	Genres  []Genre  `json:"genres,omitempty" gorm:"many2many:movie_genres"`
	Reviews []Review `json:"reviews,omitempty" gorm:"polymorphic:Reviewable;polymorphicValue:movies"`
}

type Show struct {
	ID            uint64                      `json:"id" gorm:"primaryKey"`
	Name          string                      `json:"name" gorm:"not null"`
	Actors        datatypes.JSONSlice[string] `json:"actors"`
	ContentRating string                      `json:"content_rating"`
	IMDBID        string                      `json:"imdb_id" gorm:"index"`
	TVDBID        int64                       `json:"tvdb_id" gorm:"uniqueIndex;not null"`
	Overview      string                      `json:"overview"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
	DeletedAt     gorm.DeletedAt              `json:"deleted_at" gorm:"index"`

	// Relationships
	Seasons []Season `json:"seasons,omitempty" gorm:"foreignKey:ShowID;constraint:OnDelete:CASCADE"`
	Genres  []Genre  `json:"genres,omitempty" gorm:"many2many:show_genres"`
	Reviews []Review `json:"reviews,omitempty" gorm:"polymorphic:Reviewable;polymorphicValue:shows"`
}

type Season struct {
	ID           uint64         `json:"id" gorm:"primaryKey"`
	ShowID       uint64         `json:"show_id" gorm:"not null;index"`
	SeasonNumber int            `json:"season_number" gorm:"not null"`
	Title        string         `json:"title"`
	Overview     string         `json:"overview"`
	PosterPath   string         `json:"poster_path"`
	AirDate      time.Time      `json:"air_date"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `json:"deleted_at" gorm:"index"`

	// Relationships
	Episodes []Episode `json:"episodes,omitempty" gorm:"foreignKey:SeasonID;constraint:OnDelete:CASCADE"`
}

type Episode struct {
	ID            uint64                      `json:"id" gorm:"primaryKey"`
	SeasonID      uint64                      `json:"season_id" gorm:"not null;index"`
	ShowID        uint64                      `json:"show_id" gorm:"not null;index"`
	Title         string                      `json:"title" gorm:"not null"`
	Overview      string                      `json:"overview"`
	EpisodeNumber uint32                      `json:"episode_number" gorm:"not null"`
	SeasonNumber  uint32                      `json:"season_number"`
	AirDate       time.Time                   `json:"air_date"`
	Runtime       int                         `json:"runtime"`
	GuestStars    datatypes.JSONSlice[string] `json:"guest_stars"`
	Writers       datatypes.JSONSlice[string] `json:"writers"`
	Directors     datatypes.JSONSlice[string] `json:"directors"`
	CreatedAt     time.Time                   `json:"created_at"`
	UpdatedAt     time.Time                   `json:"updated_at"`
	DeletedAt     gorm.DeletedAt              `json:"deleted_at" gorm:"index"`
}

type Genre struct {
	ID   int    `json:"id" gorm:"primaryKey"`
	Name string `json:"name" gorm:"not null"`
}
