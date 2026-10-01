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
  vi.mocked(api.getUnreadCount).mockResolvedValue({ unread_count: 0 })
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

  it('has an invites page', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    vi.mocked(api.listInvites).mockResolvedValue({ invites: [] })
    renderWithProviders(<App />, { route: '/invites' })
    expect(await screen.findByText(/no pending invitations/i)).toBeInTheDocument()
    expect(screen.getByRole('link', { name: 'Invites' })).toBeInTheDocument()
  })

  it('has a shared list page', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    vi.mocked(api.getShared).mockResolvedValue({
      id: 'l1', owner_id: 'u2', title: 'Shared one', description: '', privacy: 'shared', type: 'movie', role: 'viewer', item_count: 0, items: [],
    })
    renderWithProviders(<App />, { route: '/shared/tok' })
    expect(await screen.findByRole('heading', { name: 'Shared one' })).toBeInTheDocument()
  })

  it('shows the unread notification count in the nav', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    vi.mocked(api.getUnreadCount).mockResolvedValue({ unread_count: 3 })
    renderWithProviders(<App />)
    expect(await screen.findByLabelText('3 unread')).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /Notifications/ })).toBeInTheDocument()
  })

  it('shows no badge when everything is read', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    renderWithProviders(<App />)
    await screen.findByText('Ada L')
    expect(screen.queryByLabelText(/unread/)).not.toBeInTheDocument()
  })

  it('has notifications and friends pages', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    vi.mocked(api.listNotifications).mockResolvedValue({ notifications: [], unread_count: 0, page: 1, per_page: 20, total: 0 })
    vi.mocked(api.listFriends).mockResolvedValue({ friends: [] })
    vi.mocked(api.listFriendRequests).mockResolvedValue({ incoming: [], outgoing: [] })
    renderWithProviders(<App />, { route: '/notifications' })
    expect(await screen.findByText(/no notifications/i)).toBeInTheDocument()
    await userEvent.click(screen.getByRole('link', { name: 'Friends' }))
    expect(await screen.findByText(/no friends yet/i)).toBeInTheDocument()
  })

  it('links your name to the profile page', async () => {
    vi.mocked(api.getMe).mockResolvedValue(me)
    vi.mocked(api.listMyProgress).mockResolvedValue({ progress: [] })
    vi.mocked(api.listMyReviews).mockResolvedValue({ reviews: [], page: 1, per_page: 20, total: 0 })
    renderWithProviders(<App />)
    await userEvent.click(await screen.findByRole('link', { name: 'Ada L' }))
    expect(await screen.findByText(/nothing tracked yet/i)).toBeInTheDocument()
  })
})
