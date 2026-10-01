import * as api from './endpoints'
import { ApiError, request } from './client'

vi.mock('./client', async (importOriginal) => ({
  ...(await importOriginal<typeof import('./client')>()),
  request: vi.fn().mockResolvedValue('ok'),
}))

afterEach(() => vi.mocked(request).mockClear())

describe('endpoints', () => {
  it.each([
    ['me', () => api.getMe(), ['/me']],
    ['logout', () => api.logout(), ['/auth/logout', { method: 'POST' }]],
    ['search', () => api.search('the matrix & co', 2), ['/search?q=the%20matrix%20%26%20co&page=2']],
    ['lists', () => api.listWatchlists(), ['/watchlists']],
    ['list', () => api.getWatchlist('l1'), ['/watchlists/l1']],
    [
      'create',
      () => api.createWatchlist({ title: 'T', type: 'movie' }),
      ['/watchlists', { method: 'POST', body: { title: 'T', type: 'movie' } }],
    ],
    ['delete', () => api.deleteWatchlist('l1'), ['/watchlists/l1', { method: 'DELETE' }]],
    [
      'add item',
      () => api.addItem('l1', 603),
      ['/watchlists/l1/items', { method: 'POST', body: { tmdb_id: 603 } }],
    ],
    ['movie', () => api.getMovie(603), ['/movies/603']],
    ['show', () => api.getShow(1396), ['/shows/1396']],
    ['reviews', () => api.listReviews('movies', 603, 2), ['/movies/603/reviews?page=2']],
    [
      'create review',
      () => api.createReview('shows', 1396, { rating: 9 }),
      ['/shows/1396/reviews', { method: 'POST', body: { rating: 9 } }],
    ],
    ['update review', () => api.updateReview('r1', { rating: 4 }), ['/reviews/r1', { method: 'PATCH', body: { rating: 4 } }]],
    ['delete review', () => api.deleteReview('r1'), ['/reviews/r1', { method: 'DELETE' }]],
    [
      'update list',
      () => api.updateWatchlist('l1', { title: 'N', privacy: 'shared' }),
      ['/watchlists/l1', { method: 'PATCH', body: { title: 'N', privacy: 'shared' } }],
    ],
    [
      'reorder',
      () => api.reorderItems('l1', ['b', 'a']),
      ['/watchlists/l1/items/order', { method: 'PUT', body: { item_ids: ['b', 'a'] } }],
    ],
    [
      'item notes',
      () => api.updateItemNotes('l1', 'i1', 'hi'),
      ['/watchlists/l1/items/i1', { method: 'PATCH', body: { notes: 'hi' } }],
    ],
    ['progress', () => api.getProgress('shows', 1396), ['/me/progress/shows/1396']],
    [
      'set progress',
      () => api.setProgress('movies', 603, { status: 'completed' }),
      ['/me/progress/movies/603', { method: 'PUT', body: { status: 'completed' } }],
    ],
    ['clear progress', () => api.clearProgress('movies', 603), ['/me/progress/movies/603', { method: 'DELETE' }]],
    ['members', () => api.listMembers('l1'), ['/watchlists/l1/collaborators']],
    [
      'invite',
      () => api.inviteMember('l1', 'grace', 'editor'),
      ['/watchlists/l1/collaborators', { method: 'POST', body: { user: 'grace', role: 'editor' } }],
    ],
    [
      'set role',
      () => api.setMemberRole('l1', 'u2', 'viewer'),
      ['/watchlists/l1/collaborators/u2', { method: 'PATCH', body: { role: 'viewer' } }],
    ],
    ['remove member', () => api.removeMember('l1', 'u2'), ['/watchlists/l1/collaborators/u2', { method: 'DELETE' }]],
    ['invites', () => api.listInvites(), ['/me/invites']],
    ['accept', () => api.acceptInvite('i1'), ['/invites/i1/accept', { method: 'POST' }]],
    ['decline', () => api.declineInvite('i1'), ['/invites/i1/decline', { method: 'POST' }]],
    ['rotate token', () => api.rotateShareToken('l1'), ['/watchlists/l1/share-token', { method: 'POST' }]],
    ['shared', () => api.getShared('a b'), ['/shared/a%20b']],
    ['notifications', () => api.listNotifications(3), ['/me/notifications?page=3']],
    ['unread count', () => api.getUnreadCount(), ['/me/notifications/unread-count']],
    ['mark read', () => api.markNotificationRead('n1'), ['/me/notifications/n1/read', { method: 'POST' }]],
    ['mark all read', () => api.markAllNotificationsRead(), ['/me/notifications/read-all', { method: 'POST' }]],
    ['friends', () => api.listFriends(), ['/friends']],
    ['friend requests', () => api.listFriendRequests(), ['/friends/requests']],
    ['add friend', () => api.addFriend('grace'), ['/friends', { method: 'POST', body: { user: 'grace' } }]],
    ['accept request', () => api.acceptFriendRequest('f1'), ['/friends/requests/f1/accept', { method: 'POST' }]],
    ['decline request', () => api.declineFriendRequest('f1'), ['/friends/requests/f1/decline', { method: 'POST' }]],
    ['unfriend', () => api.removeFriend('u2'), ['/friends/u2', { method: 'DELETE' }]],
    ['remove item', () => api.removeItem('l1', 'i1'), ['/watchlists/l1/items/i1', { method: 'DELETE' }]],
  ])('%s', async (_name, call, expected) => {
    await call()
    expect(request).toHaveBeenCalledWith(...expected)
  })
})

describe('getProgress', () => {
  it('treats 404 as "not tracked"', async () => {
    vi.mocked(request).mockRejectedValueOnce(new ApiError(404, 'not_found', 'nope'))
    await expect(api.getProgress('movies', 1)).resolves.toBeNull()
  })

  it('rethrows other errors', async () => {
    vi.mocked(request).mockRejectedValueOnce(new ApiError(500, 'internal', 'boom'))
    await expect(api.getProgress('movies', 1)).rejects.toThrow('boom')
  })
})
