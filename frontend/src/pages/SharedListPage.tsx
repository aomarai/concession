import { useParams } from 'react-router-dom'
import { useQuery } from '@tanstack/react-query'
import { getShared } from '../api/endpoints'
import Alert from '../components/Alert'
import TicketCard, { Fact } from '../components/TicketCard'
import TitleLink, { itemTitle } from '../components/TitleLink'
import { errorMessage, year } from '../lib/format'

// A list opened through a share link: always read-only.
export default function SharedListPage() {
  const { token = '' } = useParams()
  const list = useQuery({ queryKey: ['shared', token], queryFn: () => getShared(token) })

  if (list.isError) return <Alert message={errorMessage(list.error)} />
  if (!list.data) return <p className="t-body text-ink-muted">Loading…</p>
  const { data } = list

  return (
    <section className="flex flex-col gap-8">
      <div className="flex flex-col gap-2">
        <h1 className="t-display-l page-title">{data.title}</h1>
        {data.description && <p className="t-body m-0 text-ink-muted">{data.description}</p>}
      </div>
      {data.items.length === 0 && <p className="t-body m-0 text-ink-muted">Nothing on this list yet.</p>}
      <ul className="cn-ticket-grid">
        {data.items.map((it, i) => (
          <TicketCard
            key={it.id} index={i} name={itemTitle(it)} posterPath={it.movie?.poster_path} title={<TitleLink item={it} />}
            meta={it.movie ? <Fact icon="calendar">{year(it.movie.release_date)}</Fact> : <Fact icon="tv">TV show</Fact>}
          >
            {it.notes && <p className="t-body-s m-0 text-ink-muted">{it.notes}</p>}
          </TicketCard>
        ))}
      </ul>
    </section>
  )
}
