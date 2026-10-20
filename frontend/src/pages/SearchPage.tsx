import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useMutation, useQuery } from '@tanstack/react-query'
import { addItem, listWatchlists, search } from '../api/endpoints'
import type { ListType, SearchResult, WatchlistSummary } from '../api/types'
import Alert from '../components/Alert'
import Button from '../components/Button'
import Icon from '../components/Icon'
import Pager from '../components/Pager'
import TicketCard, { Fact } from '../components/TicketCard'
import { errorMessage, year } from '../lib/format'

function AddToList({ result, lists }: { result: SearchResult; lists: WatchlistSummary[] }) {
  const kind: ListType = result.media_type === 'tv' ? 'show' : 'movie'
  const choices = lists.filter((l) => l.type === kind && (l.role === 'owner' || l.role === 'editor'))
  const [listId, setListId] = useState('')
  const add = useMutation({ mutationFn: (id: string) => addItem(id, result.id) })

  if (choices.length === 0) {
    return <p className="t-body-s m-0 text-ink-muted">No {kind === 'show' ? 'TV show' : 'movie'} list you can edit.</p>
  }
  const target = choices.find((l) => l.id === listId)

  return (
    <div className="flex w-full flex-col gap-2">
      <div className="flex items-center gap-2">
        <select aria-label="Add to list" value={listId} onChange={(e) => { add.reset(); setListId(e.target.value) }} className="cn-select min-w-0 flex-1">
          <option value="">Add to list…</option>
          {choices.map((l) => <option key={l.id} value={l.id}>{l.title}</option>)}
        </select>
        <Button icon="plus" disabled={!target || add.isPending} onClick={() => add.mutate(listId)}>Add</Button>
      </div>
      {add.isSuccess && target && <p className="cn-success"><Icon name="check" />Added to {target.title}</p>}
      {add.isError && <Alert message={errorMessage(add.error)} />}
    </div>
  )
}

export default function SearchPage() {
  const [input, setInput] = useState('')
  const [query, setQuery] = useState('')
  const [page, setPage] = useState(1)
  const lists = useQuery({ queryKey: ['watchlists'], queryFn: () => listWatchlists(1, 100) })
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
    <section className="flex flex-col gap-8">
      <h1 className="t-display-l page-title">Search</h1>
      <form onSubmit={submit} role="search" className="cn-search">
        <Icon name="search" />
        <input type="search" value={input} onChange={(e) => setInput(e.target.value)} placeholder="Movies and TV shows" aria-label="Search movies and TV shows" />
        <Button type="submit" variant="dark">Search</Button>
      </form>

      {results.isError && <Alert message={errorMessage(results.error)} />}
      {data && data.results.length === 0 && <p className="t-body m-0 text-ink-muted">No results. Try another title.</p>}
      <ul className="cn-ticket-grid">
        {data?.results.map((r, i) => {
          const name = r.title ?? r.name ?? ''
          return (
            <TicketCard
              key={`${r.media_type}-${r.id}`} index={i} name={name} posterPath={r.poster_path}
              title={<Link to={`/${r.media_type === 'tv' ? 'shows' : 'movies'}/${r.id}`} className="link">{name}</Link>}
              meta={<>
                {year(r.release_date ?? r.first_air_date) && <Fact icon="calendar">{year(r.release_date ?? r.first_air_date)}</Fact>}
                {r.media_type === 'tv' ? <Fact icon="tv">TV show</Fact> : <Fact icon="film">Movie</Fact>}
              </>}
              actions={<AddToList result={r} lists={lists.data?.watchlists ?? []} />}
            >
              {r.overview && <p className="t-body-s m-0 line-clamp-3">{r.overview}</p>}
            </TicketCard>
          )
        })}
      </ul>
      {data && <Pager page={page} total={data.total_pages} perPage={1} onChange={setPage} />}
    </section>
  )
}
