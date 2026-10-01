import { screen } from '@testing-library/react'
import { Route, Routes } from 'react-router-dom'
import SharedListPage from './SharedListPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { WatchlistDetail } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const detail: WatchlistDetail = {
  id: 'l1', owner_id: 'u1', title: 'Friday night', description: 'cozy', privacy: 'shared', type: 'movie',
  role: 'viewer', item_count: 2,
  items: [
    { id: 'i1', item_type: 'movie', position: 0, notes: 'classic', movie: { id: 1, tmdb_id: 603, title: 'The Matrix', overview: '', poster_path: '', release_date: '1999-03-30T00:00:00Z' } },
    { id: 'i2', item_type: 'show', position: 1, notes: '', show: { id: 2, tmdb_id: 1396, name: 'Breaking Bad', overview: '' } },
  ],
}

function renderPage() {
  return renderWithProviders(
    <Routes><Route path="/shared/:token" element={<SharedListPage />} /></Routes>,
    { route: '/shared/tok' },
  )
}

describe('SharedListPage', () => {
  it('shows the list read-only', async () => {
    vi.mocked(api.getShared).mockResolvedValue(detail)
    renderPage()
    expect(await screen.findByRole('heading', { name: 'Friday night' })).toBeInTheDocument()
    expect(api.getShared).toHaveBeenCalledWith('tok')
    expect(screen.getByText('cozy')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'The Matrix' })).toHaveAttribute('href', '/movies/603')
    expect(screen.getByRole('link', { name: 'Breaking Bad' })).toHaveAttribute('href', '/shows/1396')
    expect(screen.getByText('classic')).toBeInTheDocument()
    expect(screen.queryByRole('button')).not.toBeInTheDocument()
  })

  it('shows an empty list', async () => {
    vi.mocked(api.getShared).mockResolvedValue({ ...detail, items: [], description: '' })
    renderPage()
    expect(await screen.findByText(/nothing on this list/i)).toBeInTheDocument()
  })

  it('shows errors for unknown or disabled links', async () => {
    vi.mocked(api.getShared).mockRejectedValue(new ApiError(404, 'not_found', 'Not found'))
    renderPage()
    expect(await screen.findByRole('alert')).toHaveTextContent('Not found')
  })
})
