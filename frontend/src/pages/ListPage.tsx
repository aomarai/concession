import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  deleteWatchlist, getWatchlist, removeItem, reorderItems, updateItemNotes, updateWatchlist,
} from '../api/endpoints'
import type { Privacy, WatchlistDetail, WatchlistItem } from '../api/types'
import Alert from '../components/Alert'
import Button from '../components/Button'
import SharingPanel from '../components/SharingPanel'
import TicketCard, { Fact } from '../components/TicketCard'
import TitleLink, { itemTitle } from '../components/TitleLink'
import { errorMessage, year } from '../lib/format'
import { useListEvents } from '../lib/useListEvents'

function NotesEditor({ listId, item, onDone }: { listId: string; item: WatchlistItem; onDone: () => void }) {
  const qc = useQueryClient()
  const [notes, setNotes] = useState(item.notes)
  const save = useMutation({
    mutationFn: () => updateItemNotes(listId, item.id, notes),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['watchlist', listId] })
      onDone()
    },
  })
  return (
    <div className="flex flex-col gap-2">
      <textarea aria-label={`Notes for ${itemTitle(item)}`} value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} className="cn-textarea" />
      {save.isError && <Alert message={errorMessage(save.error)} />}
      <div className="flex flex-wrap gap-2">
        <Button size="s" onClick={() => save.mutate()} disabled={save.isPending}>Save notes</Button>
        <Button size="s" variant="quiet" onClick={onDone}>Cancel</Button>
      </div>
    </div>
  )
}

function Settings({ list, onClose }: { list: WatchlistDetail; onClose: () => void }) {
  const qc = useQueryClient()
  const [title, setTitle] = useState(list.title)
  const [description, setDescription] = useState(list.description)
  const [privacy, setPrivacy] = useState<Privacy>(list.privacy)
  const save = useMutation({
    mutationFn: () => updateWatchlist(list.id, { title, description, privacy }),
    onSuccess: async () => {
      await Promise.all([
        qc.invalidateQueries({ queryKey: ['watchlist', list.id] }),
        qc.invalidateQueries({ queryKey: ['watchlists'] }),
      ])
      onClose()
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    save.mutate()
  }

  return (
    <form onSubmit={submit} className="cn-panel">
      <h2 className="t-display-m m-0">List settings</h2>
      <label className="cn-field">
        Title
        <input value={title} onChange={(e) => setTitle(e.target.value)} className="cn-input" />
      </label>
      <label className="cn-field">
        Description
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} className="cn-textarea" />
      </label>
      <label className="cn-field self-start">
        Privacy
        <select value={privacy} onChange={(e) => setPrivacy(e.target.value as Privacy)} className="cn-select">
          <option value="private">Private</option>
          <option value="shared">Shared</option>
          <option value="public">Public</option>
        </select>
      </label>
      {save.isError && <Alert message={errorMessage(save.error)} />}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={save.isPending}>Save settings</Button>
        <Button variant="quiet" onClick={onClose}>Close settings</Button>
      </div>
    </form>
  )
}

export default function ListPage() {
  const { id = '' } = useParams()
  const navigate = useNavigate()
  const qc = useQueryClient()
  useListEvents(id)
  const [editingNotes, setEditingNotes] = useState<string | null>(null)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [confirmingDelete, setConfirmingDelete] = useState(false)

  const list = useQuery({ queryKey: ['watchlist', id], queryFn: () => getWatchlist(id) })
  const refresh = () => qc.invalidateQueries({ queryKey: ['watchlist', id] })
  const remove = useMutation({ mutationFn: (itemId: string) => removeItem(id, itemId), onSuccess: refresh })
  const reorder = useMutation({ mutationFn: (ids: string[]) => reorderItems(id, ids), onSuccess: refresh })
  const destroy = useMutation({
    mutationFn: () => deleteWatchlist(id),
    onSuccess: async () => {
      qc.removeQueries({ queryKey: ['watchlist', id] })
      await qc.invalidateQueries({ queryKey: ['watchlists'] })
      navigate('/')
    },
  })

  if (list.isError) return <Alert message={errorMessage(list.error)} />
  if (!list.data) return <p className="t-body text-ink-muted">Loading…</p>

  const { data } = list
  const canEdit = data.role === 'owner' || data.role === 'editor'
  const actionError = [remove, reorder, destroy].find((m) => m.isError)?.error

  function move(index: number, by: -1 | 1) {
    const ids = data.items.map((it) => it.id)
    ;[ids[index], ids[index + by]] = [ids[index + by], ids[index]]
    reorder.mutate(ids)
  }

  return (
    <section className="flex flex-col gap-8">
      <div className="flex flex-wrap items-end gap-4">
        <div className="flex min-w-0 flex-1 flex-col gap-2">
          <h1 className="t-display-l page-title">{data.title}</h1>
          {data.description && <p className="t-body m-0 text-ink-muted">{data.description}</p>}
        </div>
        {data.role === 'owner' && (
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => setSettingsOpen(true)}>List settings</Button>
            {confirmingDelete ? (
              <>
                <Button variant="danger" onClick={() => destroy.mutate()}>Confirm delete list</Button>
                <Button variant="quiet" onClick={() => setConfirmingDelete(false)}>Keep list</Button>
              </>
            ) : (
              <Button variant="danger" onClick={() => setConfirmingDelete(true)}>Delete list</Button>
            )}
          </div>
        )}
      </div>
      {settingsOpen && <Settings list={data} onClose={() => setSettingsOpen(false)} />}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
      {data.items.length === 0 && <p className="t-body m-0 text-ink-muted">Nothing here yet. Find something on the Search page.</p>}
      <ul className="cn-ticket-grid">
        {data.items.map((it, i) => (
          <TicketCard
            key={it.id} index={i} name={itemTitle(it)} posterPath={it.movie?.poster_path} title={<TitleLink item={it} />}
            meta={it.movie ? <Fact icon="calendar">{year(it.movie.release_date)}</Fact> : <Fact icon="tv">TV show</Fact>}
            tools={canEdit && (
              <>
                <Button size="s" variant="glass" icon="left" onClick={() => move(i, -1)} disabled={i === 0 || reorder.isPending} aria-label={`Move ${itemTitle(it)} up`} />
                <Button size="s" variant="glass" icon="right" onClick={() => move(i, 1)} disabled={i === data.items.length - 1 || reorder.isPending} aria-label={`Move ${itemTitle(it)} down`} />
                <Button size="s" variant="glass" icon="cross" onClick={() => remove.mutate(it.id)} aria-label={`Remove ${itemTitle(it)}`} />
              </>
            )}
            actions={canEdit && editingNotes !== it.id && (
              <Button size="s" variant="secondary" icon="note" onClick={() => setEditingNotes(it.id)} aria-label={`Edit notes for ${itemTitle(it)}`}>
                {it.notes ? 'Edit notes' : 'Add notes'}
              </Button>
            )}
          >
            {editingNotes === it.id
              ? <NotesEditor listId={id} item={it} onDone={() => setEditingNotes(null)} />
              : it.notes && <p className="t-body-s m-0 text-ink-muted">{it.notes}</p>}
          </TicketCard>
        ))}
      </ul>
      <SharingPanel list={data} />
    </section>
  )
}
