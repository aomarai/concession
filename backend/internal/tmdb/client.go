// Package tmdb is a small client for The Movie Database v3 API, authenticated
// with a v4 "API Read Access Token" bearer token.
package tmdb

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"
)

const DefaultBaseURL = "https://api.themoviedb.org/3"

// ErrNotFound is returned when TMDB responds 404 for a title.
var ErrNotFound = errors.New("tmdb: not found")

type cacheEntry struct {
	body    []byte
	expires time.Time
}

// Client is safe for concurrent use.
type Client struct {
	baseURL  string
	token    string
	http     *http.Client
	cacheTTL time.Duration

	mu    sync.Mutex
	cache map[string]cacheEntry
}

type Option func(*Client)

// WithBaseURL overrides the API base URL (used by tests).
func WithBaseURL(u string) Option { return func(c *Client) { c.baseURL = u } }

// WithHTTPClient overrides the HTTP client.
func WithHTTPClient(h *http.Client) Option { return func(c *Client) { c.http = h } }

// WithCacheTTL sets how long successful GET responses are cached. Zero disables caching.
func WithCacheTTL(d time.Duration) Option { return func(c *Client) { c.cacheTTL = d } }

func NewClient(token string, opts ...Option) *Client {
	c := &Client{
		baseURL:  DefaultBaseURL,
		token:    token,
		http:     &http.Client{Timeout: 10 * time.Second},
		cacheTTL: 10 * time.Minute,
		cache:    make(map[string]cacheEntry),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

func (c *Client) get(ctx context.Context, path string, q url.Values, out any) error {
	u := c.baseURL + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}

	if body, ok := c.cached(u); ok {
		return json.Unmarshal(body, out)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("tmdb request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return fmt.Errorf("tmdb read body: %w", err)
	}
	switch {
	case resp.StatusCode == http.StatusNotFound:
		return ErrNotFound
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("tmdb: unexpected status %d", resp.StatusCode)
	}

	c.store(u, body)
	return json.Unmarshal(body, out)
}

func (c *Client) cached(key string) ([]byte, bool) {
	if c.cacheTTL <= 0 {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	e, ok := c.cache[key]
	if !ok || time.Now().After(e.expires) {
		delete(c.cache, key)
		return nil, false
	}
	return e.body, true
}

func (c *Client) store(key string, body []byte) {
	if c.cacheTTL <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = cacheEntry{body: body, expires: time.Now().Add(c.cacheTTL)}
}

// SearchMulti searches movies and TV shows (people are filtered out).
func (c *Client) SearchMulti(ctx context.Context, query string, page int) (*SearchResponse, error) {
	if page < 1 {
		page = 1
	}
	var raw SearchResponse
	q := url.Values{"query": {query}, "page": {strconv.Itoa(page)}, "include_adult": {"false"}}
	if err := c.get(ctx, "/search/multi", q, &raw); err != nil {
		return nil, err
	}
	kept := raw.Results[:0]
	for _, r := range raw.Results {
		if r.MediaType == "movie" || r.MediaType == "tv" {
			kept = append(kept, r)
		}
	}
	raw.Results = kept
	return &raw, nil
}

func (c *Client) GetMovie(ctx context.Context, id int64) (*Movie, error) {
	var m Movie
	err := c.get(ctx, "/movie/"+strconv.FormatInt(id, 10), url.Values{"append_to_response": {"credits"}}, &m)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) GetShow(ctx context.Context, id int64) (*Show, error) {
	var s Show
	err := c.get(ctx, "/tv/"+strconv.FormatInt(id, 10),
		url.Values{"append_to_response": {"external_ids,content_ratings,credits"}}, &s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (c *Client) GetSeason(ctx context.Context, showID int64, seasonNumber int) (*Season, error) {
	var s Season
	err := c.get(ctx, fmt.Sprintf("/tv/%d/season/%d", showID, seasonNumber), nil, &s)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// Genres returns the combined movie and TV genre lists.
func (c *Client) Genres(ctx context.Context) ([]Genre, error) {
	seen := map[int]bool{}
	var out []Genre
	for _, kind := range []string{"movie", "tv"} {
		var resp struct {
			Genres []Genre `json:"genres"`
		}
		if err := c.get(ctx, "/genre/"+kind+"/list", nil, &resp); err != nil {
			return nil, err
		}
		for _, g := range resp.Genres {
			if !seen[g.ID] {
				seen[g.ID] = true
				out = append(out, g)
			}
		}
	}
	return out, nil
}
