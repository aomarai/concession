import { ApiError, request } from './client'
import type { FriendEntry, FriendRequests, Invite, NotificationPage, ListType, MemberRole, Members, Movie, Privacy, Progress, ProgressEntry, ProgressInput, Review, ReviewInput, ReviewPage, SearchResponse, Show, TitleKind, User, WatchlistDetail, WatchlistItem, WatchlistSummary } from './types'

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

export const updateWatchlist = (id: string, body: { title?: string; description?: string; privacy?: Privacy }) =>
  request<WatchlistSummary>(`/watchlists/${id}`, { method: 'PATCH', body })
export const reorderItems = (listId: string, itemIds: string[]) =>
  request<void>(`/watchlists/${listId}/items/order`, { method: 'PUT', body: { item_ids: itemIds } })
export const updateItemNotes = (listId: string, itemId: string, notes: string) =>
  request<void>(`/watchlists/${listId}/items/${itemId}`, { method: 'PATCH', body: { notes } })

// A title nobody is tracking is "not found" to the API; for the UI it is simply null.
export async function getProgress(kind: TitleKind, tmdbId: number): Promise<Progress | null> {
  try {
    return await request<Progress>(`/me/progress/${kind}/${tmdbId}`)
  } catch (err) {
    if (err instanceof ApiError && err.status === 404) return null
    throw err
  }
}
export const setProgress = (kind: TitleKind, tmdbId: number, body: ProgressInput) =>
  request<Progress>(`/me/progress/${kind}/${tmdbId}`, { method: 'PUT', body })
export const clearProgress = (kind: TitleKind, tmdbId: number) =>
  request<void>(`/me/progress/${kind}/${tmdbId}`, { method: 'DELETE' })

export const listMembers = (listId: string) => request<Members>(`/watchlists/${listId}/collaborators`)
export const inviteMember = (listId: string, user: string, role: MemberRole) =>
  request<unknown>(`/watchlists/${listId}/collaborators`, { method: 'POST', body: { user, role } })
export const setMemberRole = (listId: string, userId: string, role: MemberRole) =>
  request<void>(`/watchlists/${listId}/collaborators/${userId}`, { method: 'PATCH', body: { role } })
export const removeMember = (listId: string, userId: string) =>
  request<void>(`/watchlists/${listId}/collaborators/${userId}`, { method: 'DELETE' })

export const listInvites = () => request<{ invites: Invite[] }>('/me/invites')
export const acceptInvite = (id: string) => request<void>(`/invites/${id}/accept`, { method: 'POST' })
export const declineInvite = (id: string) => request<void>(`/invites/${id}/decline`, { method: 'POST' })

export const rotateShareToken = (listId: string) =>
  request<{ share_token: string }>(`/watchlists/${listId}/share-token`, { method: 'POST' })
export const getShared = (token: string) => request<WatchlistDetail>(`/shared/${encodeURIComponent(token)}`)

export const listNotifications = (page = 1) => request<NotificationPage>(`/me/notifications?page=${page}`)
export const getUnreadCount = () => request<{ unread_count: number }>('/me/notifications/unread-count')
export const markNotificationRead = (id: string) => request<void>(`/me/notifications/${id}/read`, { method: 'POST' })
export const markAllNotificationsRead = () => request<{ marked: number }>('/me/notifications/read-all', { method: 'POST' })

export const listFriends = () => request<{ friends: FriendEntry[] }>('/friends')
export const listFriendRequests = () => request<FriendRequests>('/friends/requests')
export const addFriend = (user: string) => request<FriendEntry>('/friends', { method: 'POST', body: { user } })
export const acceptFriendRequest = (id: string) => request<void>(`/friends/requests/${id}/accept`, { method: 'POST' })
export const declineFriendRequest = (id: string) => request<void>(`/friends/requests/${id}/decline`, { method: 'POST' })
export const removeFriend = (userId: string) => request<void>(`/friends/${userId}`, { method: 'DELETE' })

export const listMyProgress = () => request<{ progress: ProgressEntry[] }>('/me/progress')
export const listMyReviews = (page = 1) => request<ReviewPage>(`/me/reviews?page=${page}`)
