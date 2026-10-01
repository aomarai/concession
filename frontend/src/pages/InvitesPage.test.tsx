import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import InvitesPage from './InvitesPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { Invite } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const invite = (o: Partial<Invite> = {}): Invite => ({
  id: 'i1', watchlist_id: 'l1', watchlist_title: 'Friday night', role: 'editor',
  invited_by: { id: 'u2', display_name: 'Grace H' }, ...o,
})

describe('InvitesPage', () => {
  it('lists invitations', async () => {
    vi.mocked(api.listInvites).mockResolvedValue({ invites: [invite()] })
    renderWithProviders(<InvitesPage />)
    expect(await screen.findByText('Friday night')).toBeInTheDocument()
    expect(screen.getByText(/Grace H invited you as editor/)).toBeInTheDocument()
  })

  it('copes with an unknown inviter', async () => {
    vi.mocked(api.listInvites).mockResolvedValue({ invites: [invite({ invited_by: undefined, role: 'viewer' })] })
    renderWithProviders(<InvitesPage />)
    expect(await screen.findByText(/Invited as viewer/)).toBeInTheDocument()
  })

  it('shows an empty state', async () => {
    vi.mocked(api.listInvites).mockResolvedValue({ invites: [] })
    renderWithProviders(<InvitesPage />)
    expect(await screen.findByText(/no pending invitations/i)).toBeInTheDocument()
  })

  it('shows load errors', async () => {
    vi.mocked(api.listInvites).mockRejectedValue(new ApiError(500, 'internal', 'invites broke'))
    renderWithProviders(<InvitesPage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('invites broke')
  })

  it('accepts and links to the list', async () => {
    vi.mocked(api.listInvites).mockResolvedValueOnce({ invites: [invite()] })
    vi.mocked(api.acceptInvite).mockResolvedValue()
    renderWithProviders(<InvitesPage />)
    const card = (await screen.findByText('Friday night')).closest('li')!
    vi.mocked(api.listInvites).mockResolvedValue({ invites: [] })
    await userEvent.click(within(card).getByRole('button', { name: 'Accept' }))
    expect(api.acceptInvite).toHaveBeenCalledWith('i1')
    expect(await screen.findByText(/no pending invitations/i)).toBeInTheDocument()
  })

  it('declines', async () => {
    vi.mocked(api.listInvites).mockResolvedValueOnce({ invites: [invite()] })
    vi.mocked(api.declineInvite).mockResolvedValue()
    renderWithProviders(<InvitesPage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Decline' }))
    expect(api.declineInvite).toHaveBeenCalledWith('i1')
  })

  it('shows action errors', async () => {
    vi.mocked(api.listInvites).mockResolvedValue({ invites: [invite()] })
    vi.mocked(api.acceptInvite).mockRejectedValue(new ApiError(404, 'not_found', 'Invitation withdrawn'))
    renderWithProviders(<InvitesPage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Accept' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Invitation withdrawn')
  })
})
