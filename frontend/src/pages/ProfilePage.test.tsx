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
  vi.clearAllMocks()
  vi.mocked(api.getMe).mockResolvedValue({ id: 'u1', username: 'ada', email: 'ada@example.com', display_name: 'Ada L', avatar_url: 'https://img.example/a.png' })
  vi.mocked(api.listMyProgress).mockResolvedValue({ progress: [], page: 1, per_page: 20, total: 0 })
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

describe('ProfilePage editing', () => {
  const open = async () => {
    renderWithProviders(<ProfilePage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Edit profile' }))
  }

  it('saves only the fields that changed and shows the result', async () => {
    vi.mocked(api.updateMe).mockResolvedValue({ id: 'u1', username: 'ada', display_name: 'Ada Lovelace', avatar_url: '' })
    await open()
    expect(screen.getByLabelText('Display name')).toHaveValue('Ada L')
    expect(screen.getByLabelText('Username')).toHaveValue('ada')
    expect(screen.getByRole('button', { name: 'Save' })).toBeDisabled()
    await userEvent.clear(screen.getByLabelText('Display name'))
    await userEvent.type(screen.getByLabelText('Display name'), 'Ada Lovelace')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(api.updateMe).toHaveBeenCalledWith({ display_name: 'Ada Lovelace' })
    expect(await screen.findByRole('heading', { name: 'Ada Lovelace' })).toBeInTheDocument()
    expect(screen.queryByLabelText('Display name')).not.toBeInTheDocument()
  })

  it('can change the username', async () => {
    vi.mocked(api.updateMe).mockResolvedValue({ id: 'u1', username: 'countess', display_name: 'Ada L', avatar_url: '' })
    await open()
    await userEvent.clear(screen.getByLabelText('Username'))
    await userEvent.type(screen.getByLabelText('Username'), 'countess')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(api.updateMe).toHaveBeenCalledWith({ username: 'countess' })
    expect(await screen.findByText('@countess')).toBeInTheDocument()
  })

  it('shows why a save failed and keeps the form open', async () => {
    vi.mocked(api.updateMe).mockRejectedValue(new ApiError(409, 'conflict', 'That username is already taken'))
    await open()
    await userEvent.type(screen.getByLabelText('Username'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByText('That username is already taken')).toBeInTheDocument()
    expect(screen.getByLabelText('Username')).toBeInTheDocument()
  })

  it('cannot be cancelled while a save is in flight', async () => {
    vi.mocked(api.updateMe).mockReturnValue(new Promise(() => {}))
    await open()
    await userEvent.type(screen.getByLabelText('Username'), 'x')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('is not overwritten by a profile fetch that started before the save', async () => {
    let finishStale!: (u: { id: string; username: string; display_name: string; avatar_url: string }) => void
    const { client } = renderWithProviders(<ProfilePage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Edit profile' }))
    vi.mocked(api.getMe).mockReturnValue(new Promise((resolve) => (finishStale = resolve)))
    void client.refetchQueries({ queryKey: ['me'] })
    vi.mocked(api.updateMe).mockResolvedValue({ id: 'u1', username: 'ada', display_name: 'Ada Lovelace', avatar_url: '' })
    await userEvent.clear(screen.getByLabelText('Display name'))
    await userEvent.type(screen.getByLabelText('Display name'), 'Ada Lovelace')
    await userEvent.click(screen.getByRole('button', { name: 'Save' }))
    expect(await screen.findByRole('heading', { name: 'Ada Lovelace' })).toBeInTheDocument()
    finishStale({ id: 'u1', username: 'ada', display_name: 'Ada L', avatar_url: '' })
    await new Promise((r) => setTimeout(r, 20))
    expect(screen.getByRole('heading', { name: 'Ada Lovelace' })).toBeInTheDocument()
  })

  it('cancels without saving', async () => {
    await open()
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByLabelText('Display name')).not.toBeInTheDocument()
    expect(api.updateMe).not.toHaveBeenCalled()
  })
})

describe('ProfilePage account deletion', () => {
  const open = async () => {
    const view = renderWithProviders(<ProfilePage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Delete account' }))
    return view
  }

  it('needs the username typed before it will delete', async () => {
    await open()
    const confirm = screen.getByRole('button', { name: 'Permanently delete' })
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/type your username/i), 'ad')
    expect(confirm).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/type your username/i), 'A')
    expect(confirm).toBeEnabled()
  })

  it('rejects a wrong value as long as the username', async () => {
    await open()
    await userEvent.type(screen.getByLabelText(/type your username/i), 'xyz')
    expect(screen.getByRole('button', { name: 'Permanently delete' })).toBeDisabled()
    await userEvent.type(screen.getByLabelText(/type your username/i), '{Enter}')
    expect(api.deleteMe).not.toHaveBeenCalled()
  })

  it('cannot be cancelled while the deletion is in flight', async () => {
    vi.mocked(api.deleteMe).mockReturnValue(new Promise(() => {}))
    await open()
    await userEvent.type(screen.getByLabelText(/type your username/i), 'ada')
    await userEvent.click(screen.getByRole('button', { name: 'Permanently delete' }))
    expect(screen.getByRole('button', { name: 'Cancel' })).toBeDisabled()
  })

  it('accepts the e-mail address too, then signs out', async () => {
    vi.mocked(api.deleteMe).mockResolvedValue(undefined)
    const { client } = await open()
    const reset = vi.spyOn(client, 'resetQueries')
    await userEvent.type(screen.getByLabelText(/type your username/i), 'ADA@example.com')
    await userEvent.click(screen.getByRole('button', { name: 'Permanently delete' }))
    expect(api.deleteMe).toHaveBeenCalledWith('ADA@example.com')
    await vi.waitFor(() => expect(reset).toHaveBeenCalled())
  })

  it('shows why deleting failed', async () => {
    vi.mocked(api.deleteMe).mockRejectedValue(new ApiError(500, 'internal_error', 'delete broke'))
    await open()
    await userEvent.type(screen.getByLabelText(/type your username/i), 'ada')
    await userEvent.click(screen.getByRole('button', { name: 'Permanently delete' }))
    expect(await screen.findByText('delete broke')).toBeInTheDocument()
  })

  it('can be cancelled', async () => {
    await open()
    await userEvent.click(screen.getByRole('button', { name: 'Cancel' }))
    expect(screen.queryByLabelText(/type your username/i)).not.toBeInTheDocument()
    expect(api.deleteMe).not.toHaveBeenCalled()
  })
})

describe('ProfilePage tracked titles', () => {
  const tracked = (progress: ProgressEntry[], o: { page?: number; total?: number } = {}) =>
    vi.mocked(api.listMyProgress).mockResolvedValue({ progress, page: o.page ?? 1, per_page: 20, total: o.total ?? progress.length })

  it('lists titles with their status, links and show progress', async () => {
    tracked([
      entry({}),
      entry({ item_type: 'show', status: 'watching', movie: undefined, show: bb, last_season_num: 2, last_episode_num: 5 }),
      entry({ status: 'completed' }),
      entry({ item_type: 'show', status: 'plan_to_watch', movie: undefined, show: bb }),
    ])
    renderWithProviders(<ProfilePage />)
    const section = (await screen.findByRole('heading', { name: 'Tracking' })).closest('section')!
    await within(section).findAllByRole('listitem')
    const rows = within(section).getAllByRole('listitem')
    expect(rows).toHaveLength(4)
    expect(within(rows[0]).getByRole('link', { name: 'The Matrix' })).toHaveAttribute('href', '/movies/603')
    expect(within(rows[0]).getByText('Watching')).toBeInTheDocument()
    expect(within(rows[1]).getByRole('link', { name: 'Breaking Bad' })).toHaveAttribute('href', '/shows/1396')
    expect(within(rows[1]).getByText('S2 E5')).toBeInTheDocument()
    expect(within(rows[2]).getByText('Completed')).toBeInTheDocument()
    expect(within(rows[3]).getByText('Plan to watch')).toBeInTheDocument()
    expect(within(rows[3]).queryByText(/^S\d/)).not.toBeInTheDocument()
    expect(api.listMyProgress).toHaveBeenCalledWith(1, undefined)
  })

  it('filters by status and goes back to the first page', async () => {
    tracked([entry({})], { total: 45 })
    renderWithProviders(<ProfilePage />)
    const section = (await screen.findByRole('heading', { name: 'Tracking' })).closest('section')!
    await within(section).findByText('The Matrix')
    await userEvent.click(within(section).getByRole('button', { name: 'Next' }))
    expect(api.listMyProgress).toHaveBeenLastCalledWith(2, undefined)
    await userEvent.click(within(section).getByRole('radio', { name: 'Completed' }))
    expect(api.listMyProgress).toHaveBeenLastCalledWith(1, 'completed')
    expect(within(section).getByRole('radio', { name: 'Completed' })).toHaveAttribute('aria-checked', 'true')
    await userEvent.click(within(section).getByRole('radio', { name: 'All' }))
    expect(api.listMyProgress).toHaveBeenLastCalledWith(1, undefined)
  })

  it('pages through titles', async () => {
    tracked([entry({})], { total: 45 })
    renderWithProviders(<ProfilePage />)
    const section = (await screen.findByRole('heading', { name: 'Tracking' })).closest('section')!
    await within(section).findByText('The Matrix')
    expect(within(section).getByText('Page 1 of 3')).toBeInTheDocument()
    expect(within(section).getByRole('button', { name: 'Previous' })).toBeDisabled()
    await userEvent.click(within(section).getByRole('button', { name: 'Next' }))
    expect(api.listMyProgress).toHaveBeenLastCalledWith(2, undefined)
    await within(section).findByText('The Matrix')
    await userEvent.click(within(section).getByRole('button', { name: 'Next' }))
    await within(section).findByText('Page 3 of 3')
    expect(api.listMyProgress).toHaveBeenLastCalledWith(3, undefined)
    expect(within(section).getByRole('button', { name: 'Next' })).toBeDisabled()
    await userEvent.click(within(section).getByRole('button', { name: 'Previous' }))
    expect(api.listMyProgress).toHaveBeenLastCalledWith(2, undefined)
  })

  it('shows an empty state', async () => {
    renderWithProviders(<ProfilePage />)
    expect(await screen.findByText(/nothing tracked yet/i)).toBeInTheDocument()
  })

  it('says so when a status has nothing', async () => {
    renderWithProviders(<ProfilePage />)
    await screen.findByText(/nothing tracked yet/i)
    await userEvent.click(screen.getByRole('radio', { name: 'Dropped' }))
    expect(await screen.findByText(/nothing is marked as dropped/i)).toBeInTheDocument()
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
    expect(await screen.findByRole('img', { name: 'Rated 9 out of 10' })).toBeInTheDocument()
    expect(screen.getByText('Great')).toBeInTheDocument()
    expect(screen.getByText('Loved it')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'Rated 4 out of 10' })).toBeInTheDocument()
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
