import { useState } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteMe, getMe, listMyProgress, listMyReviews, updateMe } from '../api/endpoints'
import type { ProgressEntry, User, WatchStatus } from '../api/types'
import Alert from '../components/Alert'
import TitleLink from '../components/TitleLink'
import { errorMessage } from '../lib/format'

const STATUSES: [WatchStatus, string][] = [
  ['watching', 'Watching'],
  ['plan_to_watch', 'Plan to watch'],
  ['completed', 'Completed'],
  ['dropped', 'Dropped'],
]

function position(e: ProgressEntry): string | undefined {
  return e.item_type === 'show' && (e.last_season_num > 0 || e.last_episode_num > 0)
    ? `S${e.last_season_num} E${e.last_episode_num}`
    : undefined
}

const inputClass = 'rounded bg-zinc-900 px-2 py-1'
const buttonClass = 'rounded bg-zinc-800 px-3 py-1 text-sm disabled:opacity-50'

function EditProfile({ user, onDone }: { user: User; onDone: () => void }) {
  const qc = useQueryClient()
  const [displayName, setDisplayName] = useState(user.display_name)
  const [username, setUsername] = useState(user.username)
  // Only send what changed: a legacy username (the Google e-mail) is not valid
  // to send back unchanged.
  const changes: { display_name?: string; username?: string } = {}
  if (displayName !== user.display_name) changes.display_name = displayName
  if (username !== user.username) changes.username = username
  const save = useMutation({
    mutationFn: () => updateMe(changes),
    onSuccess: async (updated) => {
      // A fetch that started before the save would otherwise land after it with the old profile.
      await qc.cancelQueries({ queryKey: ['me'] })
      qc.setQueryData(['me'], updated)
      onDone()
    },
  })
  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <label className="block text-sm">
        Display name
        <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className={`${inputClass} mt-1 block`} />
      </label>
      <label className="block text-sm">
        Username
        <input value={username} onChange={(e) => setUsername(e.target.value)} className={`${inputClass} mt-1 block`} />
      </label>
      {save.isError && <Alert message={errorMessage(save.error)} />}
      <div className="flex gap-2">
        <button type="submit" disabled={Object.keys(changes).length === 0 || save.isPending} className={buttonClass}>Save</button>
        <button type="button" onClick={onDone} disabled={save.isPending} className={buttonClass}>Cancel</button>
      </div>
    </form>
  )
}

function DeleteAccount({ user, onDone }: { user: User; onDone: () => void }) {
  const qc = useQueryClient()
  const [typed, setTyped] = useState('')
  const remove = useMutation({
    mutationFn: () => deleteMe(typed),
    // The session is gone: refetching "me" now yields 401, which shows the sign-in page.
    onSuccess: () => qc.resetQueries(),
  })
  const t = typed.trim().toLowerCase()
  const matches = t !== '' && (t === user.username.toLowerCase() || t === user.email?.toLowerCase())
  return (
    <form
      className="space-y-2"
      onSubmit={(e) => {
        e.preventDefault()
        remove.mutate()
      }}
    >
      <p className="text-sm text-zinc-300">
        This permanently deletes your account, your lists, reviews, watch progress and friendships. It cannot be undone.
      </p>
      <label className="block text-sm">
        Type your username or e-mail to confirm
        <input value={typed} onChange={(e) => setTyped(e.target.value)} className={`${inputClass} mt-1 block`} />
      </label>
      {remove.isError && <Alert message={errorMessage(remove.error)} />}
      <div className="flex gap-2">
        <button type="submit" disabled={!matches || remove.isPending} className="rounded bg-red-700 px-3 py-1 text-sm disabled:opacity-50">Permanently delete</button>
        <button type="button" onClick={onDone} disabled={remove.isPending} className={buttonClass}>Cancel</button>
      </div>
    </form>
  )
}

function Account() {
  const me = useQuery({ queryKey: ['me'], queryFn: getMe })
  const [mode, setMode] = useState<'view' | 'edit' | 'delete'>('view')
  if (me.isError) return <Alert message={errorMessage(me.error)} />
  if (!me.data) return <p className="text-zinc-400">Loading…</p>
  const u = me.data
  const done = () => setMode('view')
  return (
    <div className="space-y-4">
      <header className="flex items-center gap-4">
        {u.avatar_url && <img src={u.avatar_url} alt={u.display_name} className="h-16 w-16 rounded-full object-cover" />}
        <div>
          <h1 className="text-2xl font-semibold">{u.display_name}</h1>
          <p className="text-zinc-400">@{u.username}</p>
          {u.email && <p className="text-sm text-zinc-500">{u.email}</p>}
        </div>
      </header>
      {mode === 'edit' && <EditProfile user={u} onDone={done} />}
      {mode === 'delete' && <DeleteAccount user={u} onDone={done} />}
      {mode === 'view' && (
        <div className="flex gap-2">
          <button onClick={() => setMode('edit')} className={buttonClass}>Edit profile</button>
          <button onClick={() => setMode('delete')} className={`${buttonClass} text-red-400`}>Delete account</button>
        </div>
      )}
    </div>
  )
}

function Tracked() {
  const progress = useQuery({ queryKey: ['my-progress'], queryFn: listMyProgress })
  if (progress.isError) return <Alert message={errorMessage(progress.error)} />
  if (!progress.data) return <p className="text-zinc-400">Loading…</p>
  const entries = progress.data.progress
  if (entries.length === 0) return <p className="text-zinc-400">Nothing tracked yet. Set a status on a title page.</p>
  return (
    <div className="space-y-4">
      {STATUSES.map(([status, label]) => {
        const group = entries.filter((e) => e.status === status)
        if (group.length === 0) return null
        return (
          <section key={status} className="space-y-1">
            <h3 className="font-medium">{label}</h3>
            <ul className="space-y-1">
              {group.map((e, i) => (
                <li key={i} className="flex gap-2 text-sm">
                  <TitleLink item={e} />
                  <span className="text-zinc-400">{position(e)}</span>
                </li>
              ))}
            </ul>
          </section>
        )
      })}
    </div>
  )
}

function MyReviews() {
  const [page, setPage] = useState(1)
  const reviews = useQuery({ queryKey: ['my-reviews', page], queryFn: () => listMyReviews(page) })
  if (reviews.isError) return <Alert message={errorMessage(reviews.error)} />
  if (!reviews.data) return <p className="text-zinc-400">Loading…</p>
  const { data } = reviews
  const pages = Math.ceil(data.total / data.per_page)
  return (
    <div className="space-y-3">
      {data.reviews.length === 0 && <p className="text-zinc-400">You haven't reviewed anything yet.</p>}
      <ul className="space-y-2">
        {data.reviews.map((r) => (
          <li key={r.id} className="space-y-1 rounded bg-zinc-900 p-3">
            <p className="font-medium">
              <TitleLink item={r} /> <span className="text-zinc-400">{r.rating}/10</span>
            </p>
            {r.title && <p>{r.title}</p>}
            {r.content && <p className="text-sm text-zinc-300">{r.content}</p>}
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="flex gap-2">
          <button disabled={page <= 1} onClick={() => setPage(page - 1)} className="rounded bg-zinc-800 px-3 py-1 text-sm disabled:opacity-50">Previous</button>
          <button disabled={page >= pages} onClick={() => setPage(page + 1)} className="rounded bg-zinc-800 px-3 py-1 text-sm disabled:opacity-50">Next</button>
        </div>
      )}
    </div>
  )
}

export default function ProfilePage() {
  return (
    <div className="space-y-8">
      <Account />
      <section className="space-y-3">
        <h2 className="text-xl font-semibold">Tracking</h2>
        <Tracked />
      </section>
      <section className="space-y-3">
        <h2 className="text-xl font-semibold">Your reviews</h2>
        <MyReviews />
      </section>
    </div>
  )
}
