import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import ListsPage from './ListsPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { WatchlistSummary } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const list: WatchlistSummary = {
  id: 'l1', owner_id: 'u1', title: 'Friday night', description: 'cozy', privacy: 'private',
  type: 'movie', role: 'owner', item_count: 3,
}

describe('ListsPage', () => {
  it('lists watchlists with links', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [list] })
    renderWithProviders(<ListsPage />)
    const link = await screen.findByRole('link', { name: /friday night/i })
    expect(link).toHaveAttribute('href', '/lists/l1')
    expect(screen.getByText(/3 titles/i)).toBeInTheDocument()
  })

  it('singularizes one title', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [{ ...list, item_count: 1 }] })
    renderWithProviders(<ListsPage />)
    expect(await screen.findByText(/1 title\b/i)).toBeInTheDocument()
  })

  it('shows an empty state', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [] })
    renderWithProviders(<ListsPage />)
    expect(await screen.findByText(/no lists yet/i)).toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.listWatchlists).mockRejectedValue(new ApiError(500, 'internal', 'db down'))
    renderWithProviders(<ListsPage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('db down')
  })

  it('creates a list and refreshes', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValueOnce({ watchlists: [] })
    vi.mocked(api.createWatchlist).mockResolvedValue(list)
    renderWithProviders(<ListsPage />)
    await screen.findByText(/no lists yet/i)
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [list] })

    await userEvent.type(screen.getByLabelText('Title'), 'Friday night')
    await userEvent.selectOptions(screen.getByLabelText('Type'), 'show')
    await userEvent.click(screen.getByRole('button', { name: /create list/i }))

    expect(api.createWatchlist).toHaveBeenCalledWith({ title: 'Friday night', type: 'show' })
    expect(await screen.findByRole('link', { name: /friday night/i })).toBeInTheDocument()
    expect(screen.getByLabelText('Title')).toHaveValue('')
  })

  it('shows create errors', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [] })
    vi.mocked(api.createWatchlist).mockRejectedValue(new ApiError(400, 'bad_request', 'Title is required'))
    renderWithProviders(<ListsPage />)
    await screen.findByText(/no lists yet/i)
    await userEvent.type(screen.getByLabelText('Title'), '  ')
    await userEvent.click(screen.getByRole('button', { name: /create list/i }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Title is required')
  })

  it('labels show lists', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [{ ...list, type: 'show' }] })
    renderWithProviders(<ListsPage />)
    expect(await screen.findByText(/TV shows ·/)).toBeInTheDocument()
  })
})
