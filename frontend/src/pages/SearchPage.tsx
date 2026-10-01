import { useState, type FormEvent } from 'react'
import { useMutation, useQuery } from '@tanstack/react-query'
import { addItem, listWatchlists, search } from '../api/endpoints'
import type { ListType, SearchResult, WatchlistSummary } from '../api/types'
import Alert from '../components/Alert'
import Poster from '../components/Poster'
import { errorMessage, year } from '../lib/format'

function AddToList({ result, lists }: { result: SearchResult; lists: WatchlistSummary[] }) {
  const kind: ListType = result.media_type === 'tv' ? 'show' : 'movie'
  const choices = lists.filter((l) => l.type === kind && (l.role === 'owner' || l.role === 'editor'))
  const [listId, setListId] = useState('')
  const add = useMutation({ mutationFn: (id: string) => addItem(id, result.id) })

  if (choices.length === 0) {
    return <p className="text-sm text-zinc-500">No {kind === 'show' ? 'TV show' : 'movie'} list you can edit.</p>
  }
  const target = choices.find((l) => l.id === listId)

  return (
    <div className="space-y-1">
      <div className="flex items-center gap-2">
        <label className="text-sm">
          <span className="sr-only">Add to list</span>
          <select aria-label="Add to list" value={listId} onChange={(e) => { add.reset(); setListId(e.target.value) }} className="rounded bg-zinc-800 px-2 py-1">
            <option value="">Add to list…</option>
            {choices.map((l) => <option key={l.id} value={l.id}>{l.title}</option>)}
          </select>
        </label>
        <button disabled={!target || add.isPending} onClick={() => add.mutate(listId)} className="rounded bg-amber-500 px-3 py-1 text-sm font-medium text-zinc-900 disabled:opacity-50">
          Add
        </button>
      </div>
      {add.isSuccess && target && <p className="text-sm text-green-400">Added to {target.title}</p>}
      {add.isError && <Alert message={errorMessage(add.error)} />}
    </div>
  )
}

export default function SearchPage() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const lists = useQuery({ queryKey: ['watchlists'], queryFn: listWatchlists })
  const results = useQuery({
    queryKey: ['search', query, page],
    queryFn: () => search(query, page),
    enabled: query !== '',
  })

  function submit(e: FormEvent) {
    e.preventDefault()
    setQuery(input.trim())
    setPage(1)
  }

  const data = results.data
  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">Search</h1>
      <form onSubmit={submit} className="flex gap-2">
        <input type="search" value={input} onChange={(e) => setInput(e.target.value)} placeholder="Movies and TV shows" className="flex-1 rounded bg-zinc-800 px-2 py-1" />
        <button type="submit" className="rounded bg-amber-500 px-3 py-1 font-medium text-zinc-900">Search</button>
      </form>

      {results.isError && <Alert message={errorMessage(results.error)} />}
      {data && data.results.length === 0 && <p className="text-zinc-400">No results.</p>}
      <ul className="space-y-3">
        {data?.results.map((r) => (
          <li key={`${r.media_type}-${r.id}`} className="flex gap-3 rounded bg-zinc-900 p-3">
            <Poster path={r.poster_path} title={r.title ?? r.name ?? ''} />
            <div className="flex-1 space-y-1">
              <p className="font-medium">{r.title ?? r.name}</p>
              <p className="text-sm text-zinc-400">{year(r.release_date ?? r.first_air_date)}</p>
              {r.overview && <p className="line-clamp-3 text-sm text-zinc-300">{r.overview}</p>}
              <AddToList result={r} lists={lists.data?.watchlists ?? []} />
            </div>
          </li>
        ))}
      </ul>
      {data && data.total_pages > 1 && (
        <div className="flex gap-2">
          <button disabled={page <= 1} onClick={() => setPage(page - 1)} className="rounded bg-zinc-800 px-3 py-1 disabled:opacity-50">Previous</button>
          <button disabled={page >= data.total_pages} onClick={() => setPage(page + 1)} className="rounded bg-zinc-800 px-3 py-1 disabled:opacity-50">Next</button>
        </div>
      )}
    </section>
  )
}
