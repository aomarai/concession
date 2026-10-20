import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import FriendsPage from './FriendsPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { FriendEntry } from '../api/types'
import { friendPage, renderWithProviders, requestPage } from '../test/utils'

vi.mock('../api/endpoints')

const entry = (id: string, userId: string, name: string, status: FriendEntry['status'] = 'accepted'): FriendEntry => ({
  id, user: { id: userId, display_name: name }, status, created_at: '2026-01-01T00:00:00Z',
})

beforeEach(() => {
  vi.mocked(api.listFriends).mockResolvedValue(friendPage([entry('f1', 'u2', 'Grace H')]))
  vi.mocked(api.listFriendRequests).mockResolvedValue(
    requestPage([entry('f2', 'u3', 'Linus T', 'pending')], [entry('f3', 'u4', 'Margaret H', 'pending')]),
  )
})

describe('FriendsPage', () => {
  it('steps back when the page it is on has emptied', async () => {
    // Page 2 exists when page 1 is fetched, but by the time it is opened the friends there are gone.
    vi.mocked(api.listFriends).mockImplementation((page = 1) =>
      Promise.resolve(page === 1 ? friendPage([entry('f1', 'u2', 'Grace H')], { total: 45 }) : friendPage([], { page, total: 20 })),
    )
    renderWithProviders(<FriendsPage />)
    await screen.findByText('Grace H')
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await vi.waitFor(() => expect(api.listFriends).toHaveBeenCalledWith(2))
    await vi.waitFor(() => expect(api.listFriends).toHaveBeenLastCalledWith(1))
    expect(await screen.findByText('Grace H')).toBeInTheDocument()
  })

  it('pages through friends', async () => {
    vi.mocked(api.listFriends).mockImplementation((page = 1) =>
      Promise.resolve(friendPage([entry(`f${page}`, `u${page}`, `Friend ${page}`)], { page, total: 45 })),
    )
    renderWithProviders(<FriendsPage />)
    const section = (await screen.findByRole('heading', { name: 'Your friends' })).closest('div')!
    await within(section).findByText('Friend 1')
    expect(within(section).getByText('Page 1 of 3')).toBeInTheDocument()
    await userEvent.click(within(section).getByRole('button', { name: 'Next' }))
    expect(await within(section).findByText('Friend 2')).toBeInTheDocument()
    expect(api.listFriends).toHaveBeenLastCalledWith(2)
    await userEvent.click(within(section).getByRole('button', { name: 'Previous' }))
    expect(await within(section).findByText('Friend 1')).toBeInTheDocument()
  })

  it('pages through requests', async () => {
    vi.mocked(api.listFriendRequests).mockImplementation((page = 1) =>
      Promise.resolve(requestPage([entry(`r${page}`, `u${page}`, `Asker ${page}`, 'pending')], [], { page, total: 45 })),
    )
    renderWithProviders(<FriendsPage />)
    const section = (await screen.findByRole('heading', { name: 'Requests' })).closest('div')!
    await within(section).findByText('Asker 1')
    expect(within(section).getByText('Page 1 of 3')).toBeInTheDocument()
    await userEvent.click(within(section).getByRole('button', { name: 'Next' }))
    expect(await within(section).findByText('Asker 2')).toBeInTheDocument()
    expect(api.listFriendRequests).toHaveBeenLastCalledWith(2)
  })

  it('shows friends and requests', async () => {
    renderWithProviders(<FriendsPage />)
    expect(await screen.findByText('Grace H')).toBeInTheDocument()
    expect(await screen.findByText('Linus T')).toBeInTheDocument()
    expect(await screen.findByText('Margaret H')).toBeInTheDocument()
  })

  it('shows empty states', async () => {
    vi.mocked(api.listFriends).mockResolvedValue(friendPage([]))
    vi.mocked(api.listFriendRequests).mockResolvedValue(requestPage([], []))
    renderWithProviders(<FriendsPage />)
    expect(await screen.findByText(/no friends yet/i)).toBeInTheDocument()
    expect(await screen.findByText(/no pending requests/i)).toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.listFriends).mockRejectedValue(new ApiError(500, 'internal', 'friends broke'))
    renderWithProviders(<FriendsPage />)
    expect(await screen.findByText('friends broke')).toBeInTheDocument()
  })

  it('shows request load errors', async () => {
    vi.mocked(api.listFriendRequests).mockRejectedValue(new ApiError(500, 'internal', 'requests broke'))
    renderWithProviders(<FriendsPage />)
    expect(await screen.findByText('requests broke')).toBeInTheDocument()
  })

  it('sends a friend request', async () => {
    vi.mocked(api.addFriend).mockResolvedValue(entry('f9', 'u9', 'New'))
    renderWithProviders(<FriendsPage />)
    const box = await screen.findByLabelText('Username or e-mail')
    await userEvent.type(box, '  new  ')
    await userEvent.click(screen.getByRole('button', { name: 'Add friend' }))
    expect(api.addFriend).toHaveBeenCalledWith('new')
    await vi.waitFor(() => expect(box).toHaveValue(''))
  })

  it('shows errors when adding a friend', async () => {
    vi.mocked(api.addFriend).mockRejectedValue(new ApiError(404, 'not_found', 'No user found'))
    renderWithProviders(<FriendsPage />)
    await userEvent.type(await screen.findByLabelText('Username or e-mail'), 'nobody')
    await userEvent.click(screen.getByRole('button', { name: 'Add friend' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('No user found')
  })

  it('accepts and declines incoming requests', async () => {
    vi.mocked(api.acceptFriendRequest).mockResolvedValue()
    vi.mocked(api.declineFriendRequest).mockResolvedValue()
    renderWithProviders(<FriendsPage />)
    const row = (await screen.findByText('Linus T')).closest('li')!
    await userEvent.click(within(row).getByRole('button', { name: 'Accept' }))
    expect(api.acceptFriendRequest).toHaveBeenCalledWith('f2')
    await userEvent.click(within(row).getByRole('button', { name: 'Decline' }))
    expect(api.declineFriendRequest).toHaveBeenCalledWith('f2')
  })

  it('cancels an outgoing request and unfriends', async () => {
    vi.mocked(api.removeFriend).mockResolvedValue()
    renderWithProviders(<FriendsPage />)
    const out = (await screen.findByText('Margaret H')).closest('li')!
    await userEvent.click(within(out).getByRole('button', { name: 'Cancel request' }))
    expect(api.removeFriend).toHaveBeenLastCalledWith('u4')
    const friend = screen.getByText('Grace H').closest('li')!
    await userEvent.click(within(friend).getByRole('button', { name: 'Remove' }))
    expect(api.removeFriend).toHaveBeenLastCalledWith('u2')
  })

  it('shows action errors', async () => {
    vi.mocked(api.acceptFriendRequest).mockRejectedValue(new ApiError(404, 'not_found', 'Request withdrawn'))
    renderWithProviders(<FriendsPage />)
    const row = (await screen.findByText('Linus T')).closest('li')!
    await userEvent.click(within(row).getByRole('button', { name: 'Accept' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Request withdrawn')
  })
})
