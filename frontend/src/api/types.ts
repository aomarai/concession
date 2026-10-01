export interface User {
  id: string
  username: string
  display_name: string
  avatar_url: string
}

export type ListType = 'movie' | 'show'
export type Privacy = 'private' | 'shared' | 'public'
export type Role = 'owner' | 'editor' | 'viewer'

export interface WatchlistSummary {
  id: string
  owner_id: string
  title: string
  description: string
  privacy: Privacy
  type: ListType
  role: Role
  item_count: number
}

export interface Movie {
  id: number
  tmdb_id: number
  title: string
  overview: string
  poster_path: string
  release_date: string
}

export interface Show {
  id: number
  tmdb_id?: number
  name: string
  overview: string
}

export interface WatchlistItem {
  id: string
  item_type: ListType
  position: number
  notes: string
  movie?: Movie
  show?: Show
}

export interface WatchlistDetail extends WatchlistSummary {
  items: WatchlistItem[]
}

// A TMDB search hit: movies carry "title", shows carry "name".
export interface SearchResult {
  id: number
  media_type: 'movie' | 'tv'
  title?: string
  name?: string
  overview: string
  poster_path: string
  release_date?: string
  first_air_date?: string
}

export interface SearchResponse {
  page: number
  total_pages: number
  total_results: number
  results: SearchResult[]
}
