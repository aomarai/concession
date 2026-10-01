import { request } from './client'
import type { ListType, Privacy, SearchResponse, User, WatchlistDetail, WatchlistItem, WatchlistSummary } from './types'

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
