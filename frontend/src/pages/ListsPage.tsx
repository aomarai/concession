import { useState, type FormEvent } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { createWatchlist, listWatchlists } from '../api/endpoints'
import type { ListType } from '../api/types'
import Alert from '../components/Alert'
import Button from '../components/Button'
import ListCard from '../components/ListCard'
import Pager from '../components/Pager'
import { errorMessage } from '../lib/format'
import { useClampPage } from '../lib/useClampPage'

export default function ListsPage() {
  const qc = useQueryClient()
  const [page, setPage] = useState(1)
  const lists = useQuery({ queryKey: ['watchlists', page], queryFn: () => listWatchlists(page), placeholderData: keepPreviousData })
  useClampPage(page, lists.data?.total, lists.data?.per_page, setPage)
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
    <section className="flex flex-col gap-8">
      <h1 className="t-display-l page-title">Your lists</h1>

      <form onSubmit={submit} className="cn-panel sm:flex-row sm:flex-wrap sm:items-end">
        <label className="cn-field min-w-0 flex-1">
          Title
          <input value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Friday night horror" className="cn-input" />
        </label>
        <label className="cn-field">
          Type
          <select value={type} onChange={(e) => setType(e.target.value as ListType)} className="cn-select">
            <option value="movie">Movies</option>
            <option value="show">TV shows</option>
          </select>
        </label>
        <Button type="submit" icon="plus" disabled={create.isPending}>Create list</Button>
      </form>
      {create.isError && <Alert message={errorMessage(create.error)} />}

      {lists.isError && <Alert message={errorMessage(lists.error)} />}
      {lists.data?.watchlists.length === 0 && <p className="t-body m-0 text-ink-muted">No lists yet. Create one above.</p>}
      <ul className="cn-listcard-grid">
        {lists.data?.watchlists.map((l, i) => <ListCard key={l.id} list={l} index={i} />)}
      </ul>
      {lists.data && <Pager page={page} total={lists.data.total} perPage={lists.data.per_page} onChange={setPage} />}
    </section>
  )
}
