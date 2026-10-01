import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import ListPage from './ListPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { WatchlistDetail } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const detail: WatchlistDetail = {
  id: 'l1', owner_id: 'u1', title: 'Friday night', description: 'cozy', privacy: 'private',
  type: 'movie', role: 'owner', item_count: 2,
  items: [
    { id: 'i1', item_type: 'movie', position: 0, notes: 'classic', movie: { id: 1, tmdb_id: 603, title: 'The Matrix', overview: '', poster_path: '/m.jpg', release_date: '1999-03-30T00:00:00Z' } },
    { id: 'i2', item_type: 'show', position: 1, notes: '', show: { id: 2, tmdb_id: 1396, name: 'Breaking Bad', overview: '' } },
  ],
}

function renderPage() {
  return renderWithProviders(
    <Routes><Route path="/lists/:id" element={<ListPage />} /></Routes>,
    { route: '/lists/l1' },
  )
}

describe('ListPage', () => {
  it('shows the list and its items', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue(detail)
    renderPage()
    expect(await screen.findByRole('heading', { name: 'Friday night' })).toBeInTheDocument()
    expect(screen.getByText('cozy')).toBeInTheDocument()
    expect(screen.getByText(/The Matrix/)).toBeInTheDocument()
    expect(screen.getByText('(1999)')).toBeInTheDocument()
    expect(screen.getByText('Breaking Bad')).toBeInTheDocument()
    expect(screen.getByText('classic')).toBeInTheDocument()
    expect(screen.getByRole('img', { name: 'The Matrix' })).toHaveAttribute('src', 'https://image.tmdb.org/t/p/w92/m.jpg')
  })

  it('shows an empty state', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue({ ...detail, items: [], description: '' })
    renderPage()
    expect(await screen.findByText(/nothing here yet/i)).toBeInTheDocument()
  })

  it('removes an item', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue(detail)
    vi.mocked(api.removeItem).mockResolvedValue()
    renderPage()
    await screen.findByText('Breaking Bad')
    vi.mocked(api.getWatchlist).mockResolvedValue({ ...detail, items: [detail.items[0]] })
    await userEvent.click(screen.getByRole('button', { name: 'Remove Breaking Bad' }))
    expect(api.removeItem).toHaveBeenCalledWith('l1', 'i2')
    await screen.findByText('The Matrix', { exact: false })
    expect(screen.queryByText('Breaking Bad')).not.toBeInTheDocument()
  })

  it('shows remove errors', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue(detail)
    vi.mocked(api.removeItem).mockRejectedValue(new ApiError(404, 'not_found', 'Item not found'))
    renderPage()
    await screen.findByText('Breaking Bad')
    await userEvent.click(screen.getByRole('button', { name: 'Remove Breaking Bad' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Item not found')
  })

  it('hides editing controls from viewers', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue({ ...detail, role: 'viewer' })
    renderPage()
    await screen.findByText('Breaking Bad')
    expect(screen.queryByRole('button', { name: /remove/i })).not.toBeInTheDocument()
  })

  it('shows not found for unknown lists', async () => {
    vi.mocked(api.getWatchlist).mockRejectedValue(new ApiError(404, 'not_found', 'Not found'))
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('Not found')
  })

  it('tolerates an item without title data', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue({ ...detail, items: [{ id: 'i9', item_type: 'movie', position: 0, notes: 'orphan' }] })
    renderPage()
    expect(await screen.findByText('orphan')).toBeInTheDocument()
  })

  it('links items to their title pages', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue(detail)
    renderPage()
    expect(await screen.findByRole('link', { name: 'The Matrix' })).toHaveAttribute('href', '/movies/603')
    expect(screen.getByRole('link', { name: 'Breaking Bad' })).toHaveAttribute('href', '/shows/1396')
  })

  it('does not link items that lack a TMDB id', async () => {
    vi.mocked(api.getWatchlist).mockResolvedValue({ ...detail, items: [{ id: 'i3', item_type: 'show', position: 0, notes: '', show: { id: 3, name: 'No Id', overview: '' } }] })
    renderPage()
    expect(await screen.findByText('No Id')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'No Id' })).not.toBeInTheDocument()
  })
})
