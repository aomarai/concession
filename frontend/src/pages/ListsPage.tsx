import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createWatchlist, listWatchlists } from '../api/endpoints'
import type { ListType } from '../api/types'
import Alert from '../components/Alert'
import { errorMessage } from '../lib/format'

export default function ListsPage() {
  const qc = useQueryClient()
  const lists = useQuery({ queryKey: ['watchlists'], queryFn: listWatchlists })
  const [title, setTitle] = useState('')
  const [type, setType] = useState<ListType>('movie')
  const create = useMutation({
    mutationFn: (body: Parameters<typeof createWatchlist>[0]) => createWatchlist(body),
    onSuccess: () => {
      setTitle('')
      return qc.invalidateQueries({ queryKey: ['watchlists'] })
    },
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    create.mutate({ title: title.trim(), type })
  }

  return (
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold">Your lists</h1>

      <form onSubmit={submit} className="flex flex-wrap items-end gap-3">
        <label className="flex flex-col text-sm">
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} className="rounded bg-zinc-800 px-2 py-1" />
        </label>
        <label className="flex flex-col text-sm">
          Type
          <select value={type} onChange={(e) => setType(e.target.value as ListType)} className="rounded bg-zinc-800 px-2 py-1">
            <option value="movie">Movies</option>
            <option value="show">TV shows</option>
          </select>
        </label>
        <button type="submit" disabled={create.isPending} className="rounded bg-amber-500 px-3 py-1 font-medium text-zinc-900">
          Create list
        </button>
      </form>
      {create.isError && <Alert message={errorMessage(create.error)} />}

      {lists.isError && <Alert message={errorMessage(lists.error)} />}
      {lists.data?.watchlists.length === 0 && <p className="text-zinc-400">No lists yet. Create one above.</p>}
      <ul className="grid gap-3 sm:grid-cols-2">
        {lists.data?.watchlists.map((l) => (
          <li key={l.id} className="rounded bg-zinc-900 p-4">
            <Link to={`/lists/${l.id}`} className="text-lg font-medium text-amber-400 hover:underline">
              {l.title}
            </Link>
            <p className="text-sm text-zinc-400">
              {l.item_count} {l.item_count === 1 ? 'title' : 'titles'} · {l.type === 'movie' ? 'Movies' : 'TV shows'} · {l.privacy}
            </p>
          </li>
        ))}
      </ul>
    </section>
  )
}
