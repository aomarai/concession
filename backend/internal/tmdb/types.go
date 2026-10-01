package tmdb

type Genre struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type SearchResponse struct {
	Page         int            `json:"page"`
	TotalPages   int            `json:"total_pages"`
	TotalResults int            `json:"total_results"`
	Results      []SearchResult `json:"results"`
}

// SearchResult covers both movie ("title", "release_date") and tv ("name",
// "first_air_date") results; MediaType is "movie" or "tv".
type SearchResult struct {
	ID           int64   `json:"id"`
	MediaType    string  `json:"media_type"`
	Title        string  `json:"title,omitempty"`
	Name         string  `json:"name,omitempty"`
	Overview     string  `json:"overview"`
	PosterPath   string  `json:"poster_path"`
	ReleaseDate  string  `json:"release_date,omitempty"`
	FirstAirDate string  `json:"first_air_date,omitempty"`
	VoteAverage  float64 `json:"vote_average"`
	Popularity   float64 `json:"popularity"`
}

type Credits struct {
	Cast []struct {
		Name string `json:"name"`
	} `json:"cast"`
}

type Movie struct {
	ID            int64   `json:"id"`
	IMDBID        string  `json:"imdb_id"`
	Title         string  `json:"title"`
	OriginalTitle string  `json:"original_title"`
	Overview      string  `json:"overview"`
	Tagline       string  `json:"tagline"`
	PosterPath    string  `json:"poster_path"`
	BackdropPath  string  `json:"backdrop_path"`
	ReleaseDate   string  `json:"release_date"`
	Revenue       int64   `json:"revenue"`
	Budget        int64   `json:"budget"`
	Runtime       int     `json:"runtime"`
	Popularity    float32 `json:"popularity"`
	VoteAverage   float32 `json:"vote_average"`
	VoteCount     int64   `json:"vote_count"`
	Genres        []Genre `json:"genres"`
	Credits       Credits `json:"credits"`
}

type SeasonSummary struct {
	SeasonNumber int    `json:"season_number"`
	Name         string `json:"name"`
	Overview     string `json:"overview"`
	PosterPath   string `json:"poster_path"`
	AirDate      string `json:"air_date"`
}

type Show struct {
	ID          int64           `json:"id"`
	Name        string          `json:"name"`
	Overview    string          `json:"overview"`
	Genres      []Genre         `json:"genres"`
	Seasons     []SeasonSummary `json:"seasons"`
	Credits     Credits         `json:"credits"`
	ExternalIDs struct {
		IMDBID string `json:"imdb_id"`
		TVDBID *int64 `json:"tvdb_id"`
	} `json:"external_ids"`
	ContentRatings struct {
		Results []struct {
			Country string `json:"iso_3166_1"`
			Rating  string `json:"rating"`
		} `json:"results"`
	} `json:"content_ratings"`
}

// ContentRating returns the US rating when present, else the first available.
func (s *Show) ContentRating() string {
	for _, r := range s.ContentRatings.Results {
		if r.Country == "US" {
			return r.Rating
		}
	}
	if len(s.ContentRatings.Results) > 0 {
		return s.ContentRatings.Results[0].Rating
	}
	return ""
}

type Person struct {
	Name string `json:"name"`
	Job  string `json:"job"`
}

type Episode struct {
	EpisodeNumber int      `json:"episode_number"`
	SeasonNumber  int      `json:"season_number"`
	Name          string   `json:"name"`
	Overview      string   `json:"overview"`
	AirDate       string   `json:"air_date"`
	Runtime       int      `json:"runtime"`
	GuestStars    []Person `json:"guest_stars"`
	Crew          []Person `json:"crew"`
}

type Season struct {
	SeasonNumber int       `json:"season_number"`
	Name         string    `json:"name"`
	Overview     string    `json:"overview"`
	PosterPath   string    `json:"poster_path"`
	AirDate      string    `json:"air_date"`
	Episodes     []Episode `json:"episodes"`
}
