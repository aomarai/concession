import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ProfilePage from './ProfilePage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { ProgressEntry, Review } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const matrix = { id: 1, tmdb_id: 603, title: 'The Matrix', overview: '', poster_path: '', release_date: '1999-03-30T00:00:00Z' }
const bb = { id: 2, tmdb_id: 1396, name: 'Breaking Bad', overview: '' }

const entry = (o: Partial<ProgressEntry>): ProgressEntry => ({
  item_type: 'movie', status: 'watching', last_season_num: 0, last_episode_num: 0, movie: matrix, ...o,
})
const review = (o: Partial<Review> = {}): Review => ({
  id: 'r1', rating: 9, title: 'Great', content: 'Loved it', author: { id: 'u1', display_name: 'Ada L' },
  created_at: '2026-01-01T00:00:00Z', updated_at: '2026-01-01T00:00:00Z', movie: matrix, ...o,
})

beforeEach(() => {
  vi.mocked(api.getMe).mockResolvedValue({ id: 'u1', username: 'ada', email: 'ada@example.com', display_name: 'Ada L', avatar_url: 'https://img.example/a.png' })
  vi.mocked(api.listMyProgress).mockResolvedValue({ progress: [] })
  vi.mocked(api.listMyReviews).mockResolvedValue({ reviews: [], page: 1, per_page: 20, total: 0 })
})

describe('ProfilePage account', () => {
  it('shows who you are', async () => {
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByRole('heading', { name: 'Ada L' })).toBeInTheDocument()
    expect(screen.getByText('@ada')).toBeInTheDocument()
    expect(screen.getByText('ada@example.com')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Ada L' })).toHaveAttribute('src', 'https://img.example/a.png')
  })

  it('works without an avatar or e-mail', async () => {
    vi.mocked(api.getMe).mockResolvedValue({ id: 'u1', username: 'ada', display_name: 'Ada L', avatar_url: '' })
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByRole('heading', { name: 'Ada L' })).toBeInTheDocument()
    expect(screen.queryByRole('img')).not.toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(500, 'internal', 'me broke'))
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText('me broke')).toBeInTheDocument()
  })
})

describe('ProfilePage tracked titles', () => {
  it('groups titles by status with links and show progress', async () => {
    vi.mocked(api.listMyProgress).mockResolvedValue({
      progress: [
        entry({}),
        entry({ item_type: 'show', status: 'watching', movie: undefined, show: bb, last_season_num: 2, last_episode_num: 5 }),
        entry({ status: 'completed' }),
        entry({ item_type: 'show', status: 'plan_to_watch', movie: undefined, show: bb }),
      ],
    })
    renderWithProviders(<ProfilePage />)
    const watching = (await screen.findByRole('heading', { name: 'Watching' })).closest('section')!
    expect(within(watching).getByRole('link', { name: 'The Matrix' })).toHaveAttribute('href', '/movies/603')
    expect(within(watching).getByRole('link', { name: 'Breaking Bad' })).toHaveAttribute('href', '/shows/1396')
    expect(within(watching).getByText('S2 E5')).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Completed' })).toBeInTheDocument()
    const plan = screen.getByRole('heading', { name: 'Plan to watch' }).closest('section')!
    expect(within(plan).queryByText(/^S\d/)).not.toBeInTheDocument()
    expect(screen.queryByRole('heading', { name: 'Dropped' })).not.toBeInTheDocument()
  })

  it('shows an empty state', async () => {
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText(/nothing tracked yet/i)).toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.listMyProgress).mockRejectedValue(new ApiError(500, 'internal', 'progress broke'))
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText('progress broke')).toBeInTheDocument()
  })
})

describe('ProfilePage reviews', () => {
  it('lists your reviews', async () => {
    vi.mocked(api.listMyReviews).mockResolvedValue({
      reviews: [review(), review({ id: 'r2', rating: 4, title: '', content: '', movie: undefined, show: bb })],
      page: 1, per_page: 20, total: 2,
    })
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText('9/10')).toBeInTheDocument()
    expect(screen.getByText('Great')).toBeInTheDocument()
    expect(screen.getByText('Loved it')).toBeInTheDocument()
    expect(screen.getByText('4/10')).toBeInTheDocument()
    const reviews = screen.getByRole('heading', { name: 'Your reviews' }).closest('section')!
    expect(within(reviews).getByRole('link', { name: 'Breaking Bad' })).toHaveAttribute('href', '/shows/1396')
  })

  it('shows an empty state', async () => {
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText(/haven.t reviewed anything yet/i)).toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.listMyReviews).mockRejectedValue(new ApiError(500, 'internal', 'reviews broke'))
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText('reviews broke')).toBeInTheDocument()
  })

  it('pages through reviews', async () => {
    vi.mocked(api.listMyReviews).mockResolvedValue({ reviews: [review()], page: 1, per_page: 20, total: 45 })
    renderWithProviders(<ProfilePage />)
    await screen.findByText('Great')
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(api.listMyReviews).toHaveBeenLastCalledWith(2)
    await screen.findByText('Great')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByText('Great')
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Previous' }))
    expect(api.listMyReviews).toHaveBeenLastCalledWith(2)
  })
})
