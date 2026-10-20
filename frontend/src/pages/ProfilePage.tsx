import { useState } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { deleteMe, getMe, listMyProgress, listMyReviews, updateMe } from '../api/endpoints'
import type { ProgressEntry, User, WatchStatus } from '../api/types'
import Alert from '../components/Alert'
import Avatar from '../components/Avatar'
import Button from '../components/Button'
import Pager from '../components/Pager'
import PopRow from '../components/PopRow'
import { RatingDisplay } from '../components/Rating'
import SegmentedControl from '../components/SegmentedControl'
import StatusChip from '../components/StatusChip'
import TitleLink from '../components/TitleLink'
import { errorMessage } from '../lib/format'
import { STATUSES, STATUS_LABEL } from '../lib/status'
import { useClampPage } from '../lib/useClampPage'

function position(e: ProgressEntry): string | undefined {
  return e.item_type === 'show' && (e.last_season_num > 0 || e.last_episode_num > 0)
    ? `S${e.last_season_num} E${e.last_episode_num}`
    : undefined
}

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
      className="cn-panel max-w-xl"
      onSubmit={(e) => {
        e.preventDefault()
        save.mutate()
      }}
    >
      <label className="cn-field">
        Display name
        <input value={displayName} onChange={(e) => setDisplayName(e.target.value)} className="cn-input" />
      </label>
      <label className="cn-field">
        Username
        <input value={username} onChange={(e) => setUsername(e.target.value)} autoCapitalize="none" autoCorrect="off" className="cn-input" />
      </label>
      {save.isError && <Alert message={errorMessage(save.error)} />}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={Object.keys(changes).length === 0 || save.isPending}>Save</Button>
        <Button variant="quiet" onClick={onDone} disabled={save.isPending}>Cancel</Button>
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
      className="cn-panel max-w-xl"
      onSubmit={(e) => {
        e.preventDefault()
        remove.mutate()
      }}
    >
      <p className="t-body m-0">
        This permanently deletes your account, your lists, reviews, watch progress and friendships. It cannot be undone.
      </p>
      <label className="cn-field">
        Type your username or e-mail to confirm
        <input value={typed} onChange={(e) => setTyped(e.target.value)} autoCapitalize="none" autoCorrect="off" className="cn-input" />
      </label>
      {remove.isError && <Alert message={errorMessage(remove.error)} />}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" variant="danger" disabled={!matches || remove.isPending}>Permanently delete</Button>
        <Button variant="quiet" onClick={onDone} disabled={remove.isPending}>Cancel</Button>
      </div>
    </form>
  )
}

function Account() {
  const me = useQuery({ queryKey: ['me'], queryFn: getMe })
  const [mode, setMode] = useState<'view' | 'edit' | 'delete'>('view')
  if (me.isError) return <Alert message={errorMessage(me.error)} />
  if (!me.data) return <p className="t-body text-ink-muted">Loading…</p>
  const u = me.data
  const done = () => setMode('view')
  return (
    <div className="flex flex-col gap-6">
      <header className="flex items-center gap-5">
        <Avatar name={u.display_name} src={u.avatar_url} size="l" />
        <div className="flex min-w-0 flex-col gap-1">
          <h1 className="t-display-l page-title">{u.display_name}</h1>
          <p className="t-body m-0 text-ink-muted">@{u.username}</p>
          {u.email && <p className="t-body-s m-0 text-ink-muted">{u.email}</p>}
        </div>
      </header>
      {mode === 'edit' && <EditProfile user={u} onDone={done} />}
      {mode === 'delete' && <DeleteAccount user={u} onDone={done} />}
      {mode === 'view' && (
        <div className="flex flex-wrap gap-2">
          <Button variant="secondary" onClick={() => setMode('edit')}>Edit profile</Button>
          <Button variant="danger" onClick={() => setMode('delete')}>Delete account</Button>
        </div>
      )}
    </div>
  )
}

type Filter = WatchStatus | 'all'
const FILTERS = [{ value: 'all' as Filter, label: 'All' }, ...STATUSES]

function Tracked() {
  const [page, setPage] = useState(1)
  const [filter, setFilter] = useState<Filter>('all')
  const status = filter === 'all' ? undefined : filter
  const progress = useQuery({
    queryKey: ['my-progress', page, status],
    queryFn: () => listMyProgress(page, status),
    placeholderData: keepPreviousData,
  })
  useClampPage(page, progress.data?.total, progress.data?.per_page, setPage)
  const choose = (f: Filter) => {
    setFilter(f)
    setPage(1)
  }
  let body
  if (progress.isError) body = <Alert message={errorMessage(progress.error)} />
  else if (!progress.data) body = <p className="t-body text-ink-muted">Loading…</p>
  else if (progress.data.progress.length === 0) {
    body = (
      <p className="t-body m-0 text-ink-muted">
        {status ? `Nothing is marked as ${STATUS_LABEL[status].toLowerCase()}.` : 'Nothing tracked yet. Set a status on a title page.'}
      </p>
    )
  } else {
    const { data } = progress
    body = (
      <>
        <ul className="m-0 flex list-none flex-col gap-3 p-0">
          {data.progress.map((e, i) => (
            <PopRow key={i} index={i}>
              <span className="t-title min-w-0 flex-1"><TitleLink item={e} /></span>
              {position(e) && <span className="t-numeral-s text-ink-muted">{position(e)}</span>}
              <StatusChip status={e.status} />
            </PopRow>
          ))}
        </ul>
        <Pager page={page} total={data.total} perPage={data.per_page} onChange={setPage} />
      </>
    )
  }
  return (
    <div className="flex flex-col gap-4">
      <SegmentedControl label="Filter by status" segments={FILTERS} value={filter} onChange={choose} />
      {body}
    </div>
  )
}

function MyReviews() {
  const [page, setPage] = useState(1)
  const reviews = useQuery({ queryKey: ['my-reviews', page], queryFn: () => listMyReviews(page) })
  useClampPage(page, reviews.data?.total, reviews.data?.per_page, setPage)
  if (reviews.isError) return <Alert message={errorMessage(reviews.error)} />
  if (!reviews.data) return <p className="t-body text-ink-muted">Loading…</p>
  const { data } = reviews
  return (
    <div className="flex flex-col gap-4">
      {data.reviews.length === 0 && <p className="t-body m-0 text-ink-muted">You haven't reviewed anything yet.</p>}
      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {data.reviews.map((r, i) => (
          <PopRow key={r.id} index={i} className="flex-col items-stretch gap-2">
            <div className="flex flex-wrap items-center gap-3">
              <span className="t-title"><TitleLink item={r} /></span>
              <RatingDisplay value={r.rating} />
            </div>
            {r.title && <p className="t-label m-0">{r.title}</p>}
            {r.content && <p className="t-body m-0 max-w-[70ch]">{r.content}</p>}
          </PopRow>
        ))}
      </ul>
      <Pager page={page} total={data.total} perPage={data.per_page} onChange={setPage} />
    </div>
  )
}

export default function ProfilePage() {
  return (
    <div className="flex flex-col gap-10">
      <Account />
      <section className="flex flex-col gap-4">
        <h2 className="t-display-m m-0">Tracking</h2>
        <Tracked />
      </section>
      <section className="flex flex-col gap-4">
        <h2 className="t-display-m m-0">Your reviews</h2>
        <MyReviews />
      </section>
    </div>
  )
}
