import type { ReactElement } from 'react'
import type { WatchlistPage, WatchlistSummary } from '../api/types'
import { render } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { MemoryRouter } from 'react-router-dom'

export function renderWithProviders(ui: ReactElement, { route = '/' } = {}) {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } })
  const result = render(
    <QueryClientProvider client={client}>
      <MemoryRouter initialEntries={[route]}>{ui}</MemoryRouter>
    </QueryClientProvider>,
  )
  return { ...result, client }
}

// A watchlists response for mocks: one page by default.
export const watchlistPage = (watchlists: WatchlistSummary[], o: { page?: number; total?: number } = {}): WatchlistPage => ({
  watchlists, page: o.page ?? 1, per_page: 20, total: o.total ?? watchlists.length,
})
