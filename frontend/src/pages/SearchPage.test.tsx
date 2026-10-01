import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import SearchPage from './SearchPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { SearchResponse, WatchlistSummary } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const lists: WatchlistSummary[] = [
  { id: 'm1', owner_id: 'u', title: 'Movies', description: '', privacy: 'private', type: 'movie', role: 'owner', item_count: 0 },
  { id: 's1', owner_id: 'u', title: 'Shows', description: '', privacy: 'private', type: 'show', role: 'editor', item_count: 0 },
  { id: 'v1', owner_id: 'u', title: 'Read only', description: '', privacy: 'public', type: 'movie', role: 'viewer', item_count: 0 },
]

const results: SearchResponse = {
  page: 1, total_pages: 1, total_results: 2,
  results: [
    { id: 603, media_type: 'movie', title: 'The Matrix', overview: 'Neo', poster_path: '/m.jpg', release_date: '1999-03-31' },
    { id: 1396, media_type: 'tv', name: 'Breaking Bad', overview: '', poster_path: '', first_air_date: '2008-01-20' },
  ],
}

beforeEach(() => {
  vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: lists })
})

async function doSearch(q = 'matrix') {
  await userEvent.type(screen.getByRole('searchbox'), q)
  await userEvent.click(screen.getByRole('button', { name: 'Search' }))
}

describe('SearchPage', () => {
  it('searches and shows results', async () => {
    vi.mocked(api.search).mockResolvedValue(results)
    renderWithProviders(<SearchPage />)
    await doSearch()
    expect(api.search).toHaveBeenCalledWith('matrix', 1)
    expect(await screen.findByText('The Matrix')).toBeInTheDocument()
    expect(screen.getByText('Breaking Bad')).toBeInTheDocument()
    expect(screen.getByText('1999')).toBeInTheDocument()
    expect(screen.getByText('Neo')).toBeInTheDocument()
  })

  it('does nothing for a blank query', async () => {
    renderWithProviders(<SearchPage />)
    await doSearch('   ')
    expect(api.search).not.toHaveBeenCalled()
  })

  it('shows when there are no results', async () => {
    vi.mocked(api.search).mockResolvedValue({ page: 1, total_pages: 0, total_results: 0, results: [] })
    renderWithProviders(<SearchPage />)
    await doSearch('zzz')
    expect(await screen.findByText(/no results/i)).toBeInTheDocument()
  })

  it('shows search errors', async () => {
    vi.mocked(api.search).mockRejectedValue(new ApiError(502, 'upstream_error', 'TMDB is down'))
    renderWithProviders(<SearchPage />)
    await doSearch()
    expect(await screen.findByRole('alert')).toHaveTextContent('TMDB is down')
  })

  it('pages through results', async () => {
    vi.mocked(api.search).mockResolvedValue({ ...results, total_pages: 2 })
    renderWithProviders(<SearchPage />)
    await doSearch()
    await screen.findByText('The Matrix')
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(api.search).toHaveBeenLastCalledWith('matrix', 2)
    await screen.findByText('The Matrix')
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Previous' }))
    expect(api.search).toHaveBeenLastCalledWith('matrix', 1)
  })

  it('adds a movie to a matching list the user can edit', async () => {
    vi.mocked(api.search).mockResolvedValue(results)
    vi.mocked(api.addItem).mockResolvedValue({} as never)
    renderWithProviders(<SearchPage />)
    await doSearch()
    const card = (await screen.findByText('The Matrix')).closest('li')!
    const select = within(card).getByLabelText('Add to list')
    expect(within(select).queryByRole('option', { name: 'Shows' })).not.toBeInTheDocument()
    expect(within(select).queryByRole('option', { name: 'Read only' })).not.toBeInTheDocument()
    await userEvent.selectOptions(select, 'm1')
    await userEvent.click(within(card).getByRole('button', { name: 'Add' }))
    expect(api.addItem).toHaveBeenCalledWith('m1', 603)
    expect(await within(card).findByText(/added to movies/i)).toBeInTheDocument()
  })

  it('adds a show to a show list', async () => {
    vi.mocked(api.search).mockResolvedValue(results)
    vi.mocked(api.addItem).mockResolvedValue({} as never)
    renderWithProviders(<SearchPage />)
    await doSearch()
    const card = (await screen.findByText('Breaking Bad')).closest('li')!
    await userEvent.selectOptions(within(card).getByLabelText('Add to list'), 's1')
    await userEvent.click(within(card).getByRole('button', { name: 'Add' }))
    expect(api.addItem).toHaveBeenCalledWith('s1', 1396)
  })

  it('shows add errors (e.g. duplicates)', async () => {
    vi.mocked(api.search).mockResolvedValue(results)
    vi.mocked(api.addItem).mockRejectedValue(new ApiError(409, 'conflict', 'Already on this list'))
    renderWithProviders(<SearchPage />)
    await doSearch()
    const card = (await screen.findByText('The Matrix')).closest('li')!
    await userEvent.selectOptions(within(card).getByLabelText('Add to list'), 'm1')
    await userEvent.click(within(card).getByRole('button', { name: 'Add' }))
    expect(await within(card).findByRole('alert')).toHaveTextContent('Already on this list')
  })

  it('hides the add control when no list fits', async () => {
    vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [] })
    vi.mocked(api.search).mockResolvedValue(results)
    renderWithProviders(<SearchPage />)
    await doSearch()
    const card = (await screen.findByText('The Matrix')).closest('li')!
    expect(await within(card).findByText(/no movie list/i)).toBeInTheDocument()
  })

  it('copes with untitled results while lists are still loading', async () => {
    vi.mocked(api.listWatchlists).mockReturnValue(new Promise(() => {}))
    vi.mocked(api.search).mockResolvedValue({ page: 1, total_pages: 1, total_results: 1, results: [{ id: 5, media_type: 'movie', overview: '', poster_path: '' }] })
    renderWithProviders(<SearchPage />)
    await doSearch()
    expect(await screen.findByText(/no movie list/i)).toBeInTheDocument()
  })
})
