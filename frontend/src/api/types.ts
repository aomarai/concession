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
  share_token?: string
}

export interface Genre {
  id: number
  name: string
}

export interface Movie {
  id: number
  tmdb_id: number
  title: string
  overview: string
  tagline?: string
  poster_path: string
  release_date: string
  runtime?: number
  vote_average?: number
  actors?: string[]
  genres?: Genre[]
}

export interface Season {
  id: number
  season_number: number
  title: string
}

export interface Show {
  id: number
  tmdb_id?: number
  name: string
  overview: string
  actors?: string[]
  genres?: Genre[]
  seasons?: Season[]
}

// Path segment used by the API and the app's routes.
export type TitleKind = 'movies' | 'shows'

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

export interface Author {
  id: string
  display_name: string
  avatar_url?: string
}

export interface Review {
  id: string
  rating: number
  title: string
  content: string
  author: Author
  created_at: string
  updated_at: string
}

export interface ReviewInput {
  rating: number
  title?: string
  content?: string
}

export interface ReviewPage {
  reviews: Review[]
  summary?: { count: number; average: number }
  page: number
  per_page: number
  total: number
}

export type WatchStatus = 'plan_to_watch' | 'watching' | 'completed' | 'dropped'

export interface Progress {
  status: WatchStatus
  last_season_num: number
  last_episode_num: number
}

export interface ProgressInput {
  status: WatchStatus
  last_season_num?: number
  last_episode_num?: number
}

export interface Person {
  id: string
  display_name: string
  avatar_url?: string
}

export type MemberRole = 'editor' | 'viewer'

export interface Member extends Person {
  role: Role
  status: 'pending' | 'accepted'
}

export interface Members {
  owner: Member
  members: Member[]
  pending?: Member[]
}

export interface Invite {
  id: string
  watchlist_id: string
  watchlist_title: string
  role: MemberRole
  invited_by?: Person
}
