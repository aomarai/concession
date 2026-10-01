import { useState, type FormEvent } from 'react'
import { useNavigate, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  deleteWatchlist, getWatchlist, removeItem, reorderItems, updateItemNotes, updateWatchlist,
} from '../api/endpoints'
import type { Privacy, WatchlistDetail, WatchlistItem } from '../api/types'
import Alert from '../components/Alert'
import Poster from '../components/Poster'
import SharingPanel from '../components/SharingPanel'
import TitleLink, { itemTitle } from '../components/TitleLink'
import { errorMessage, year } from '../lib/format'
import { useListEvents } from '../lib/useListEvents'

const button = 'rounded bg-zinc-800 px-2 py-1 text-sm disabled:opacity-40'

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
    <div className="space-y-2">
      <label className="flex flex-col text-sm">
        <span className="sr-only">Notes for {itemTitle(item)}</span>
        <textarea aria-label={`Notes for ${itemTitle(item)}`} value={notes} onChange={(e) => setNotes(e.target.value)} rows={2} className="rounded bg-zinc-800 px-2 py-1" />
      </label>
      {save.isError && <Alert message={errorMessage(save.error)} />}
      <div className="flex gap-2">
        <button onClick={() => save.mutate()} disabled={save.isPending} className="rounded bg-amber-500 px-3 py-1 text-sm font-medium text-zinc-900">Save notes</button>
        <button onClick={onDone} className={button}>Cancel</button>
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
    <form onSubmit={submit} className="space-y-2 rounded bg-zinc-900 p-3">
      <label className="flex flex-col text-sm">
        Title
        <input value={title} onChange={(e) => setTitle(e.target.value)} className="rounded bg-zinc-800 px-2 py-1" />
      </label>
      <label className="flex flex-col text-sm">
        Description
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} rows={2} className="rounded bg-zinc-800 px-2 py-1" />
      </label>
      <label className="flex flex-col text-sm">
        Privacy
        <select value={privacy} onChange={(e) => setPrivacy(e.target.value as Privacy)} className="w-40 rounded bg-zinc-800 px-2 py-1">
          <option value="private">Private</option>
          <option value="shared">Shared</option>
          <option value="public">Public</option>
        </select>
      </label>
      {save.isError && <Alert message={errorMessage(save.error)} />}
      <div className="flex gap-2">
        <button type="submit" disabled={save.isPending} className="rounded bg-amber-500 px-3 py-1 text-sm font-medium text-zinc-900">Save settings</button>
        <button type="button" onClick={onClose} className={button}>Close settings</button>
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
  if (!list.data) return <p className="text-zinc-400">Loading…</p>

  const { data } = list
  const canEdit = data.role === 'owner' || data.role === 'editor'
  const actionError = [remove, reorder, destroy].find((m) => m.isError)?.error

  function move(index: number, by: -1 | 1) {
    const ids = data.items.map((it) => it.id)
    ;[ids[index], ids[index + by]] = [ids[index + by], ids[index]]
    reorder.mutate(ids)
  }

  return (
    <section className="space-y-4">
      <div className="flex items-start gap-3">
        <h1 className="text-2xl font-semibold">{data.title}</h1>
        {data.role === 'owner' && (
          <div className="ml-auto flex gap-2">
            <button onClick={() => setSettingsOpen(true)} className={button}>List settings</button>
            {confirmingDelete ? (
              <>
                <button onClick={() => destroy.mutate()} className="rounded bg-red-900 px-2 py-1 text-sm">Confirm delete list</button>
                <button onClick={() => setConfirmingDelete(false)} className={button}>Keep list</button>
              </>
            ) : (
              <button onClick={() => setConfirmingDelete(true)} className={`${button} text-red-400`}>Delete list</button>
            )}
          </div>
        )}
      </div>
      {settingsOpen && <Settings list={data} onClose={() => setSettingsOpen(false)} />}
      {data.description && <p className="text-zinc-400">{data.description}</p>}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
      {data.items.length === 0 && <p className="text-zinc-400">Nothing here yet. Find something on the Search page.</p>}
      <ul className="space-y-3">
        {data.items.map((it, i) => (
          <li key={it.id} className="flex gap-3 rounded bg-zinc-900 p-3">
            <Poster path={it.movie?.poster_path} title={itemTitle(it)} />
            <div className="flex-1 space-y-1">
              <p className="font-medium">
                <TitleLink item={it} />{' '}
                {it.movie && <span className="text-zinc-400">({year(it.movie.release_date)})</span>}
              </p>
              {editingNotes === it.id ? (
                <NotesEditor listId={id} item={it} onDone={() => setEditingNotes(null)} />
              ) : (
                <>
                  {it.notes && <p className="text-sm text-zinc-400">{it.notes}</p>}
                  {canEdit && (
                    <button onClick={() => setEditingNotes(it.id)} aria-label={`Edit notes for ${itemTitle(it)}`} className="text-sm text-amber-400 hover:underline">
                      {it.notes ? 'Edit notes' : 'Add notes'}
                    </button>
                  )}
                </>
              )}
            </div>
            {canEdit && (
              <div className="flex flex-col items-end gap-1">
                <div className="flex gap-1">
                  <button onClick={() => move(i, -1)} disabled={i === 0 || reorder.isPending} aria-label={`Move ${itemTitle(it)} up`} className={button}>↑</button>
                  <button onClick={() => move(i, 1)} disabled={i === data.items.length - 1 || reorder.isPending} aria-label={`Move ${itemTitle(it)} down`} className={button}>↓</button>
                </div>
                <button onClick={() => remove.mutate(it.id)} aria-label={`Remove ${itemTitle(it)}`} className="text-sm text-red-400 hover:underline">
                  Remove
                </button>
              </div>
            )}
          </li>
        ))}
      </ul>
      <SharingPanel list={data} />
    </section>
  )
}
