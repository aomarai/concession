import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getShared } from '../api/endpoints'
import Alert from '../components/Alert'
import Poster from '../components/Poster'
import TitleLink, { itemTitle } from '../components/TitleLink'
import { errorMessage, year } from '../lib/format'

// A list opened through a share link: always read-only.
export default function SharedListPage() {
  const { token = '' } = useParams()
  const list = useQuery({ queryKey: ['shared', token], queryFn: () => getShared(token) })

  if (list.isError) return <Alert message={errorMessage(list.error)} />
  if (!list.data) return <p className="text-zinc-400">Loading…</p>
  const { data } = list

  return (
    <section className="space-y-4">
      <h1 className="text-2xl font-semibold">{data.title}</h1>
      {data.description && <p className="text-zinc-400">{data.description}</p>}
      {data.items.length === 0 && <p className="text-zinc-400">Nothing on this list yet.</p>}
      <ul className="space-y-3">
        {data.items.map((it) => (
          <li key={it.id} className="flex gap-3 rounded bg-zinc-900 p-3">
            <Poster path={it.movie?.poster_path} title={itemTitle(it)} />
            <div className="space-y-1">
              <p className="font-medium">
                <TitleLink item={it} />{' '}
                {it.movie && <span className="text-zinc-400">({year(it.movie.release_date)})</span>}
              </p>
              {it.notes && <p className="text-sm text-zinc-400">{it.notes}</p>}
            </div>
          </li>
        ))}
      </ul>
    </section>
  )
}
