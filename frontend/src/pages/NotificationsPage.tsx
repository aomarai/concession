import { useState } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { listNotifications, markAllNotificationsRead, markNotificationRead } from '../api/endpoints'
import Alert from '../components/Alert'
import { errorMessage } from '../lib/format'
import { appPath } from '../lib/links'

const button = 'rounded bg-zinc-800 px-3 py-1 text-sm disabled:opacity-50'

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
  const pages = data ? Math.ceil(data.total / data.per_page) : 0

  return (
    <section className="space-y-4">
      <div className="flex items-center">
        <h1 className="text-2xl font-semibold">Notifications</h1>
        <button onClick={() => markAll.mutate()} disabled={!data || data.unread_count === 0 || markAll.isPending} className={`${button} ml-auto`}>
          Mark all read
        </button>
      </div>
      {list.isError && <Alert message={errorMessage(list.error)} />}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
      {data?.notifications.length === 0 && <p className="text-zinc-400">No notifications.</p>}
      <ul className="space-y-2">
        {data?.notifications.map((n) => {
          const to = appPath(n.link_url)
          return (
            <li key={n.id} className={`flex items-center gap-3 rounded p-3 ${n.is_read ? 'bg-zinc-900 text-zinc-400' : 'bg-zinc-800'}`}>
              {!n.is_read && <span className="rounded bg-amber-500 px-1.5 text-xs font-medium text-zinc-900">New</span>}
              <span className="flex-1">
                {to ? <Link to={to} className="hover:underline">{n.message}</Link> : n.message}
              </span>
              {!n.is_read && <button onClick={() => markOne.mutate(n.id)} className={button}>Mark read</button>}
            </li>
          )
        })}
      </ul>
      {pages > 1 && (
        <div className="flex gap-2">
          <button disabled={page <= 1} onClick={() => setPage(page - 1)} className={button}>Previous</button>
          <button disabled={page >= pages} onClick={() => setPage(page + 1)} className={button}>Next</button>
        </div>
      )}
    </section>
  )
}
