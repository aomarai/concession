import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { Route, Routes } from 'react-router-dom'
import SharingPanel from './SharingPanel'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { Members, Role, WatchlistDetail } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const list = (o: Partial<WatchlistDetail> = {}): WatchlistDetail => ({
  id: 'l1', owner_id: 'u1', title: 'Friday', description: '', privacy: 'shared', type: 'movie',
  role: 'owner', item_count: 0, items: [], share_token: 'tok123', ...o,
})
const members: Members = {
  owner: { id: 'u1', display_name: 'Ada L', role: 'owner', status: 'accepted' },
  members: [{ id: 'u2', display_name: 'Grace H', role: 'editor', status: 'accepted' }],
  pending: [{ id: 'u3', display_name: 'Linus T', role: 'viewer', status: 'pending' }],
}

function renderPanel(l = list(), meId = 'u1', loaded: Members | Error = members) {
  vi.mocked(api.getMe).mockResolvedValue({ id: meId, username: 'x', display_name: 'X', avatar_url: '' })
  if (loaded instanceof Error) vi.mocked(api.listMembers).mockRejectedValue(loaded)
  else vi.mocked(api.listMembers).mockResolvedValue(loaded)
  return renderWithProviders(
    <Routes>
      <Route path="/" element={<p>Home</p>} />
      <Route path="/lists/l1" element={<SharingPanel list={l} />} />
    </Routes>,
    { route: '/lists/l1' },
  )
}

describe('SharingPanel as owner', () => {
  it('lists the owner, members and pending invitations', async () => {
    renderPanel()
    expect(await screen.findByText('Grace H')).toBeInTheDocument()
    expect(screen.getByText('Ada L')).toBeInTheDocument()
    expect(screen.getByText('Linus T')).toBeInTheDocument()
    expect(api.listMembers).toHaveBeenCalledWith('l1')
  })

  it('invites someone', async () => {
    vi.mocked(api.inviteMember).mockResolvedValue({})
    renderPanel()
    const box = await screen.findByLabelText('Username or e-mail')
    await userEvent.type(box, 'linus')
    await userEvent.selectOptions(screen.getByLabelText('Role'), 'viewer')
    await userEvent.click(screen.getByRole('button', { name: 'Invite' }))
    expect(api.inviteMember).toHaveBeenCalledWith('l1', 'linus', 'viewer')
    await vi.waitFor(() => expect(box).toHaveValue(''))
  })

  it('shows invite errors', async () => {
    vi.mocked(api.inviteMember).mockRejectedValue(new ApiError(404, 'not_found', 'No user found'))
    renderPanel()
    await userEvent.type(await screen.findByLabelText('Username or e-mail'), 'nobody')
    await userEvent.click(screen.getByRole('button', { name: 'Invite' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('No user found')
  })

  it('changes a role', async () => {
    vi.mocked(api.setMemberRole).mockResolvedValue()
    renderPanel()
    await userEvent.selectOptions(await screen.findByLabelText('Role for Grace H'), 'viewer')
    expect(api.setMemberRole).toHaveBeenCalledWith('l1', 'u2', 'viewer')
  })

  it('removes a member and cancels an invitation', async () => {
    vi.mocked(api.removeMember).mockResolvedValue()
    renderPanel()
    await userEvent.click(await screen.findByRole('button', { name: 'Remove Grace H' }))
    expect(api.removeMember).toHaveBeenLastCalledWith('l1', 'u2')
    await userEvent.click(screen.getByRole('button', { name: 'Cancel invite for Linus T' }))
    expect(api.removeMember).toHaveBeenLastCalledWith('l1', 'u3')
  })

  it('shows errors from member actions', async () => {
    vi.mocked(api.removeMember).mockRejectedValue(new ApiError(404, 'not_found', 'Already removed'))
    renderPanel()
    await userEvent.click(await screen.findByRole('button', { name: 'Remove Grace H' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Already removed')
  })

  it('works without pending invitations', async () => {
    renderPanel(list(), 'u1', { owner: members.owner, members: [] })
    expect(await screen.findByText('Ada L')).toBeInTheDocument()
  })

  it('shows the share link and rotates it', async () => {
    vi.mocked(api.rotateShareToken).mockResolvedValue({ share_token: 'new' })
    renderPanel()
    const link = await screen.findByLabelText('Share link')
    expect(link).toHaveValue(`${window.location.origin}/shared/tok123`)
    await userEvent.click(screen.getByRole('button', { name: 'New link' }))
    expect(api.rotateShareToken).toHaveBeenCalledWith('l1')
  })

  it('explains how to enable the link on private lists', async () => {
    renderPanel(list({ privacy: 'private' }))
    expect(await screen.findByText(/set the list to shared or public/i)).toBeInTheDocument()
    expect(screen.queryByLabelText('Share link')).not.toBeInTheDocument()
  })

  it('shows no link before a token exists', async () => {
    renderPanel(list({ share_token: undefined }))
    await screen.findByText('Grace H')
    expect(screen.queryByLabelText('Share link')).not.toBeInTheDocument()
  })

  it('shows rotate errors', async () => {
    vi.mocked(api.rotateShareToken).mockRejectedValue(new ApiError(500, 'internal', 'rotate failed'))
    renderPanel()
    await userEvent.click(await screen.findByRole('button', { name: 'New link' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('rotate failed')
  })

  it('shows member load errors', async () => {
    renderPanel(list(), 'u1', new ApiError(500, 'internal', 'members broke'))
    expect(await screen.findByText('members broke')).toBeInTheDocument()
  })
})

describe('SharingPanel as a collaborator', () => {
  it.each<Role>(['editor', 'viewer'])('shows %s members read-only and lets them leave', async (role) => {
    vi.mocked(api.removeMember).mockResolvedValue()
    renderPanel(list({ role, share_token: undefined }), 'u2')
    expect(await screen.findByText('Grace H')).toBeInTheDocument()
    expect(screen.queryByLabelText('Username or e-mail')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Role for Grace H')).not.toBeInTheDocument()
    expect(screen.queryByLabelText('Share link')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /remove/i })).not.toBeInTheDocument()
    await userEvent.click(screen.getByRole('button', { name: 'Leave list' }))
    expect(api.removeMember).toHaveBeenCalledWith('l1', 'u2')
    expect(await screen.findByText('Home')).toBeInTheDocument()
  })

  it('does not offer leaving to someone who is only reading a public list', async () => {
    renderPanel(list({ role: 'viewer', share_token: undefined }), 'u9')
    await screen.findByText('Grace H')
    expect(screen.queryByRole('button', { name: 'Leave list' })).not.toBeInTheDocument()
  })

  it('shows leave errors', async () => {
    vi.mocked(api.removeMember).mockRejectedValue(new ApiError(403, 'forbidden', 'Cannot leave'))
    renderPanel(list({ role: 'editor' }), 'u2')
    await userEvent.click(await screen.findByRole('button', { name: 'Leave list' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Cannot leave')
  })
})
