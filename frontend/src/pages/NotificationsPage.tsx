import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listNotifications, markAllNotificationsRead, markNotificationRead } from '../api/endpoints'
import Alert from '../components/Alert'
import Badge from '../components/Badge'
import Button from '../components/Button'
import Pager from '../components/Pager'
import PopRow from '../components/PopRow'
import { errorMessage } from '../lib/format'
import { appPath } from '../lib/links'

export default function NotificationsPage() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const list = useQuery({ queryKey: ['notifications', page], queryFn: () => listNotifications(page) })
  const refresh = () => Promise.all([
    qc.invalidateQueries({ queryKey: ['notifications'] }),
    qc.invalidateQueries({ queryKey: ['unread'] }),
  ])
  const markOne = useMutation({ mutationFn: (id: string) => markNotificationRead(id), onSuccess: refresh })
  const markAll = useMutation({ mutationFn: () => markAllNotificationsRead(), onSuccess: refresh })
  const actionError = [markOne, markAll].find((m) => m.isError)?.error

  const data = list.data

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-wrap items-end gap-4">
        <h1 className="t-display-l page-title flex-1">Notifications</h1>
        <Button variant="secondary" icon="check" onClick={() => markAll.mutate()} disabled={!data || data.unread_count === 0 || markAll.isPending}>
          Mark all read
        </Button>
      </div>
      {list.isError && <Alert message={errorMessage(list.error)} />}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
      {data?.notifications.length === 0 && <p className="t-body m-0 text-ink-muted">No notifications.</p>}
      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {data?.notifications.map((n, i) => {
          const to = appPath(n.link_url)
          return (
            <PopRow key={n.id} index={i} quiet={n.is_read}>
              {!n.is_read && <Badge tone="velvet">New</Badge>}
              <span className="t-body min-w-0 flex-1">
                {to ? <Link to={to} className="link">{n.message}</Link> : n.message}
              </span>
              {!n.is_read && <Button size="s" variant="secondary" onClick={() => markOne.mutate(n.id)}>Mark read</Button>}
            </PopRow>
          )
        })}
      </ul>
      {data && <Pager page={page} total={data.total} perPage={data.per_page} onChange={setPage} />}
    </section>
  )
}
