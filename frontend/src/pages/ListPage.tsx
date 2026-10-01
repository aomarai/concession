import { Link, useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getWatchlist, removeItem } from '../api/endpoints'
import type { WatchlistItem } from '../api/types'
import Alert from '../components/Alert'
import Poster from '../components/Poster'
import { errorMessage, year } from '../lib/format'

function itemTitle(it: WatchlistItem): string {
  return it.movie?.title ?? it.show?.name ?? ''
}

function titlePath(it: WatchlistItem): string | undefined {
  if (it.movie) return `/movies/${it.movie.tmdb_id}`
  return it.show?.tmdb_id ? `/shows/${it.show.tmdb_id}` : undefined
}

function TitleLink({ item }: { item: WatchlistItem }) {
  const to = titlePath(item)
  const name = itemTitle(item)
  return to ? <Link to={to} className="hover:underline">{name}</Link> : <span>{name}</span>
}

export default function ListPage() {
  const { id = '' } = useParams()
  const qc = useQueryClient()
  const list = useQuery({ queryKey: ['watchlist', id], queryFn: () => getWatchlist(id) })
  const remove = useMutation({
    mutationFn: (itemId: string) => removeItem(id, itemId),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['watchlist', id] }),
  })

  if (list.isError) return <Alert message={errorMessage(list.error)} />
  if (!list.data) return <p className="text-zinc-400">Loading…</p>

  const { data } = list
  const canEdit = data.role === 'owner' || data.role === 'editor'

  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">{data.title}</h1>
      {data.description && <p className="text-zinc-400">{data.description}</p>}
      {remove.isError && <Alert message={errorMessage(remove.error)} />}
      {data.items.length === 0 && <p className="text-zinc-400">Nothing here yet. Find something on the Search page.</p>}
      <ul className="space-y-3">
        {data.items.map((it) => (
          <li key={it.id} className="flex gap-3 rounded bg-zinc-900 p-3">
            <Poster path={it.movie?.poster_path} title={itemTitle(it)} />
            <div className="flex-1">
              <p className="font-medium">
                <TitleLink item={it} />{' '}
                {it.movie && <span className="text-zinc-400">({year(it.movie.release_date)})</span>}
              </p>
              {it.notes && <p className="text-sm text-zinc-400">{it.notes}</p>}
            </div>
            {canEdit && (
              <button
                onClick={() => remove.mutate(it.id)}
                aria-label={`Remove ${itemTitle(it)}`}
                className="self-start text-sm text-red-400 hover:underline"
              >
                Remove
              </button>
            )}
          </li>
        ))}
      </ul>
    </section>
  )
}
