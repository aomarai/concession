import { request } from './client'
import type { ListType, Movie, Privacy, Review, ReviewInput, ReviewPage, SearchResponse, Show, TitleKind, User, WatchlistDetail, WatchlistItem, WatchlistSummary } from './types'

export const getMe = () => request<User>('/me')
export const logout = () => request<void>('/auth/logout', { method: 'POST' })

export const search = (q: string, page = 1) =>
  request<SearchResponse>(`/search?q=${encodeURIComponent(q)}&page=${page}`)

export const listWatchlists = () => request<{ watchlists: WatchlistSummary[] }>('/watchlists')
export const getWatchlist = (id: string) => request<WatchlistDetail>(`/watchlists/${id}`)
export const createWatchlist = (body: { title: string; description?: string; privacy?: Privacy; type: ListType }) =>
  request<WatchlistSummary>('/watchlists', { method: 'POST', body })
export const deleteWatchlist = (id: string) => request<void>(`/watchlists/${id}`, { method: 'DELETE' })

export const addItem = (listId: string, tmdbId: number) =>
  request<WatchlistItem>(`/watchlists/${listId}/items`, { method: 'POST', body: { tmdb_id: tmdbId } })
export const removeItem = (listId: string, itemId: string) =>
  request<void>(`/watchlists/${listId}/items/${itemId}`, { method: 'DELETE' })

export const getMovie = (tmdbId: number) => request<Movie>(`/movies/${tmdbId}`)
export const getShow = (tmdbId: number) => request<Show>(`/shows/${tmdbId}`)

export const listReviews = (kind: TitleKind, tmdbId: number, page = 1) =>
  request<ReviewPage>(`/${kind}/${tmdbId}/reviews?page=${page}`)
export const createReview = (kind: TitleKind, tmdbId: number, body: ReviewInput) =>
  request<Review>(`/${kind}/${tmdbId}/reviews`, { method: 'POST', body })
export const updateReview = (id: string, body: Partial<ReviewInput>) =>
  request<Review>(`/reviews/${id}`, { method: 'PATCH', body })
export const deleteReview = (id: string) => request<void>(`/reviews/${id}`, { method: 'DELETE' })
