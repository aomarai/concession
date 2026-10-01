import * as api from './endpoints'
import { request } from './client'

vi.mock('./client', () => ({ request: vi.fn().mockResolvedValue('ok') }))

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
    ['remove item', () => api.removeItem('l1', 'i1'), ['/watchlists/l1/items/i1', { method: 'DELETE' }]],
  ])('%s', async (_name, call, expected) => {
    await call()
    expect(request).toHaveBeenCalledWith(...expected)
  })
})
