import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import App from './App'
import { ApiError } from './api/client'
import * as api from './api/endpoints'
import { renderWithProviders } from './test/utils'

vi.mock('./api/endpoints')

const me = { id: 'u1', username: 'ada', display_name: 'Ada L', avatar_url: '' }

beforeEach(() => {
  vi.mocked(api.listWatchlists).mockResolvedValue({ watchlists: [] })
})

describe('App auth gate', () => {
  it('shows the sign-in page when signed out', async () => {
    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'unauthorized', 'no'))
    renderWithProviders(<App />)
    const link = await screen.findByRole('link', { name: /sign in with google/i })
    expect(link).toHaveAttribute('href', '/api/v1/auth/google/login')
  })

  it('shows an error with retry on other failures', async () => {
    vi.mocked(api.getMe).mockRejectedValueOnce(new ApiError(500, 'internal', 'boom'))
    renderWithProviders(<App />)
    expect(await screen.findByText('boom')).toBeInTheDocument()
    vi.mocked(api.getMe).mockResolvedValue(me)
    await userEvent.click(screen.getByRole('button', { name: /try again/i }))
    expect(await screen.findByText('Ada L')).toBeInTheDocument()
  })

  it('shows a generic message for non-API errors', async () => {
    vi.mocked(api.getMe).mockRejectedValue(new Error('network'))
    renderWithProviders(<App />)
    expect(await screen.findByText(/something went wrong/i)).toBeInTheDocument()
  })

  it('shows the app shell when signed in and signs out', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    vi.mocked(api.logout).mockResolvedValue()
    renderWithProviders(<App />)
    expect(await screen.findByText('Ada L')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Lists' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Search' })).toBeInTheDocument()

    vi.mocked(api.getMe).mockRejectedValue(new ApiError(401, 'unauthorized', 'no'))
    await userEvent.click(screen.getByRole('button', { name: /sign out/i }))
    await waitFor(() => expect(api.logout).toHaveBeenCalled())
    expect(await screen.findByRole('link', { name: /sign in with google/i })).toBeInTheDocument()
  })

  it('shows a not-found page for unknown routes', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    renderWithProviders(<App />, { route: '/nope' })
    expect(await screen.findByText(/page not found/i)).toBeInTheDocument()
  })
})
