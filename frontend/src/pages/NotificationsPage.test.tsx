import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import NotificationsPage from './NotificationsPage'
import { ApiError } from '../api/client'
import * as api from '../api/endpoints'
import type { AppNotification, NotificationPage } from '../api/types'
import { renderWithProviders } from '../test/utils'

vi.mock('../api/endpoints')

const note = (o: Partial<AppNotification> = {}): AppNotification => ({
  id: 'n1', type: 'watchlist_invite', message: 'Grace H invited you to "Friday"', is_read: false,
  link_url: '/invites', actor: { id: 'u2', display_name: 'Grace H' }, created_at: '2026-01-01T00:00:00Z', ...o,
})
const page = (notifications: AppNotification[], o: Partial<NotificationPage> = {}): NotificationPage => ({
  notifications, unread_count: notifications.filter((n) => !n.is_read).length, page: 1, per_page: 20, total: notifications.length, ...o,
})

describe('NotificationsPage', () => {
  it('lists notifications, linking where the destination is known', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([
      note(),
      note({ id: 'n2', type: 'item_added', message: 'Ada added a title', link_url: '/watchlists/abc-123', is_read: true }),
      note({ id: 'n3', message: 'No link here', link_url: 'https://evil.example' }),
    ]))
    renderWithProviders(<NotificationsPage />)
    expect(await screen.findByRole('link', { name: /invited you to "Friday"/ })).toHaveAttribute('href', '/invites')
    expect(screen.getByRole('link', { name: 'Ada added a title' })).toHaveAttribute('href', '/lists/abc-123')
    expect(screen.getByText('No link here')).toBeInTheDocument()
    expect(screen.queryByRole('link', { name: 'No link here' })).not.toBeInTheDocument()
    expect(api.listNotifications).toHaveBeenCalledWith(1)
  })

  it('marks unread ones and only unread ones as read', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([note(), note({ id: 'n2', is_read: true, message: 'Already seen' })]))
    vi.mocked(api.markNotificationRead).mockResolvedValue()
    renderWithProviders(<NotificationsPage />)
    await screen.findByRole('link', { name: /invited you/ })
    expect(screen.getAllByRole('button', { name: 'Mark read' })).toHaveLength(1)
    await userEvent.click(screen.getByRole('button', { name: 'Mark read' }))
    expect(api.markNotificationRead).toHaveBeenCalledWith('n1')
  })

  it('marks everything read', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([note()]))
    vi.mocked(api.markAllNotificationsRead).mockResolvedValue({ marked: 1 })
    renderWithProviders(<NotificationsPage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Mark all read' }))
    expect(api.markAllNotificationsRead).toHaveBeenCalled()
  })

  it('disables "mark all" when nothing is unread', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([note({ is_read: true })]))
    renderWithProviders(<NotificationsPage />)
    expect(await screen.findByRole('button', { name: 'Mark all read' })).toBeDisabled()
  })

  it('shows an empty state', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([]))
    renderWithProviders(<NotificationsPage />)
    expect(await screen.findByText(/no notifications/i)).toBeInTheDocument()
  })

  it('shows load and action errors', async () => {
    vi.mocked(api.listNotifications).mockRejectedValueOnce(new ApiError(500, 'internal', 'inbox broke'))
    const { unmount } = renderWithProviders(<NotificationsPage />)
    expect(await screen.findByRole('alert')).toHaveTextContent('inbox broke')
    unmount()

    vi.mocked(api.listNotifications).mockResolvedValue(page([note()]))
    vi.mocked(api.markNotificationRead).mockRejectedValue(new ApiError(404, 'not_found', 'Gone'))
    renderWithProviders(<NotificationsPage />)
    await userEvent.click(await screen.findByRole('button', { name: 'Mark read' }))
    expect(await screen.findByRole('alert')).toHaveTextContent('Gone')
  })

  it('pages through notifications', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([note()], { total: 45 }))
    renderWithProviders(<NotificationsPage />)
    await screen.findByRole('link', { name: /invited you/ })
    expect(screen.getByRole('button', { name: 'Previous' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    expect(api.listNotifications).toHaveBeenLastCalledWith(2)
    await screen.findByRole('link', { name: /invited you/ })
    await userEvent.click(screen.getByRole('button', { name: 'Next' }))
    await screen.findByRole('link', { name: /invited you/ })
    expect(screen.getByRole('button', { name: 'Next' })).toBeDisabled()
    await userEvent.click(screen.getByRole('button', { name: 'Previous' }))
    expect(api.listNotifications).toHaveBeenLastCalledWith(2)
  })

  it('marks unread notifications visibly', async () => {
    vi.mocked(api.listNotifications).mockResolvedValue(page([note(), note({ id: 'n2', is_read: true, message: 'Old one', link_url: undefined })]))
    renderWithProviders(<NotificationsPage />)
    const unread = (await screen.findByRole('link', { name: /invited you/ })).closest('li')!
    expect(within(unread).getByText('New')).toBeInTheDocument()
    const read = screen.getByText('Old one').closest('li')!
    expect(within(read).queryByText('New')).not.toBeInTheDocument()
  })
})
