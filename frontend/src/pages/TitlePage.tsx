import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  createReview, deleteReview, getMe, getMovie, getShow, listReviews, updateReview,
} from '../api/endpoints'
import type { Review, ReviewInput, TitleKind } from '../api/types'
import Alert from '../components/Alert'
import Poster from '../components/Poster'
import { errorMessage, year } from '../lib/format'

interface TitleView {
  name: string
  poster?: string
  meta: string
  tagline?: string
  overview: string
  genres?: string
  cast?: string
  seasons?: number
}

const joinNames = (xs?: { name: string }[]) => (xs?.length ? xs.map((x) => x.name).join(', ') : undefined)

async function loadTitle(kind: TitleKind, tmdbId: number): Promise<TitleView> {
  if (kind === 'movies') {
    const m = await getMovie(tmdbId)
    return {
      name: m.title,
      poster: m.poster_path,
      meta: [year(m.release_date), m.runtime ? `${m.runtime} min` : ''].filter(Boolean).join(' · '),
      tagline: m.tagline,
      overview: m.overview,
      genres: joinNames(m.genres),
      cast: m.actors?.join(', '),
    }
  }
  const s = await getShow(tmdbId)
  return {
    name: s.name,
    meta: '',
    overview: s.overview,
    genres: joinNames(s.genres),
    cast: s.actors?.join(', '),
    seasons: s.seasons?.length,
  }
}

function ReviewForm({
  initial, submitLabel, pending, error, onSubmit, onCancel,
}: {
  initial?: Review
  submitLabel: string
  pending: boolean
  error?: string
  onSubmit: (input: ReviewInput) => void
  onCancel?: () => void
}) {
  const [rating, setRating] = useState(initial ? String(initial.rating) : '')
  const [headline, setHeadline] = useState(initial?.title ?? '')
  const [content, setContent] = useState(initial?.content ?? '')
  const [missing, setMissing] = useState(false)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!rating) return setMissing(true)
    setMissing(false)
    onSubmit({ rating: Number(rating), title: headline, content })
  }

  return (
    <form onSubmit={submit} className="space-y-2">
      <label className="flex flex-col text-sm">
        Rating
        <select value={rating} onChange={(e) => setRating(e.target.value)} className="w-32 rounded bg-zinc-800 px-2 py-1">
          <option value="">Choose…</option>
          {Array.from({ length: 10 }, (_, i) => i + 1).map((n) => <option key={n} value={n}>{n}</option>)}
        </select>
      </label>
      <label className="flex flex-col text-sm">
        Headline
        <input value={headline} onChange={(e) => setHeadline(e.target.value)} className="rounded bg-zinc-800 px-2 py-1" />
      </label>
      <label className="flex flex-col text-sm">
        Review
        <textarea value={content} onChange={(e) => setContent(e.target.value)} rows={3} className="rounded bg-zinc-800 px-2 py-1" />
      </label>
      {missing && <Alert message="Choose a rating." />}
      {error && <Alert message={error} />}
      <div className="flex gap-2">
        <button type="submit" disabled={pending} className="rounded bg-amber-500 px-3 py-1 font-medium text-zinc-900">{submitLabel}</button>
        {onCancel && <button type="button" onClick={onCancel} className="rounded bg-zinc-800 px-3 py-1">Cancel</button>}
      </div>
    </form>
  )
}

function Reviews({ kind, tmdbId }: { kind: TitleKind; tmdbId: number }) {
  const qc = useQueryClient()
  const me = useQuery({ queryKey: ['me'], queryFn: getMe })
  const [page, setPage] = useState(1)
  const [editing, setEditing] = useState(false)
  const [confirming, setConfirming] = useState(false)
  const reviews = useQuery({ queryKey: ['reviews', kind, tmdbId, page], queryFn: () => listReviews(kind, tmdbId, page) })
  const refresh = () => qc.invalidateQueries({ queryKey: ['reviews', kind, tmdbId] })

  const create = useMutation({ mutationFn: (b: ReviewInput) => createReview(kind, tmdbId, b), onSuccess: refresh })
  const update = useMutation({
    mutationFn: (v: { id: string; body: ReviewInput }) => updateReview(v.id, v.body),
    onSuccess: () => { setEditing(false); return refresh() },
  })
  const remove = useMutation({
    mutationFn: (id: string) => deleteReview(id),
    onSuccess: () => { setConfirming(false); return refresh() },
  })

  if (reviews.isError) return <Alert message={errorMessage(reviews.error)} />
  const data = reviews.data
  if (!data) return <p className="text-zinc-400">Loading reviews…</p>

  const mine = data.reviews.find((r) => r.author.id === me.data?.id)
  const pages = Math.ceil(data.total / data.per_page)
  const count = data.summary?.count ?? 0

  return (
    <section className="space-y-4">
      <h2 className="text-xl font-semibold">Reviews</h2>
      {data.summary && count > 0 && (
        <p className="text-zinc-400">{`${data.summary.average} average · ${count} ${count === 1 ? 'review' : 'reviews'}`}</p>
      )}
      {!mine && (
        <ReviewForm submitLabel="Post review" pending={create.isPending} error={create.isError ? errorMessage(create.error) : undefined} onSubmit={(b) => create.mutate(b)} />
      )}
      {data.reviews.length === 0 && <p className="text-zinc-400">No reviews yet.</p>}
      <ul className="space-y-3">
        {data.reviews.map((r) => (
          <li key={r.id} className="space-y-1 rounded bg-zinc-900 p-3">
            {r === mine && editing ? (
              <ReviewForm
                initial={r} submitLabel="Save" pending={update.isPending}
                error={update.isError ? errorMessage(update.error) : undefined}
                onSubmit={(body) => update.mutate({ id: r.id, body })} onCancel={() => setEditing(false)}
              />
            ) : (
              <>
                <p>
                  <strong>{r.author.display_name}</strong>{' '}
                  {r === mine && <span className="text-amber-400">Your review</span>}{' '}
                  <span className="text-zinc-400">{r.rating}/10</span>
                </p>
                {r.title && <p className="font-medium">{r.title}</p>}
                {r.content && <p className="text-sm text-zinc-300">{r.content}</p>}
                {r === mine && (
                  <div className="flex gap-3 text-sm">
                    <button onClick={() => setEditing(true)} className="hover:underline">Edit</button>
                    {confirming ? (
                      <>
                        <button onClick={() => remove.mutate(r.id)} className="text-red-400 hover:underline">Confirm delete</button>
                        <button onClick={() => setConfirming(false)} className="hover:underline">Keep</button>
                      </>
                    ) : (
                      <button onClick={() => setConfirming(true)} className="text-red-400 hover:underline">Delete</button>
                    )}
                  </div>
                )}
                {r === mine && remove.isError && <Alert message={errorMessage(remove.error)} />}
              </>
            )}
          </li>
        ))}
      </ul>
      {pages > 1 && (
        <div className="flex gap-2">
          <button disabled={page <= 1} onClick={() => setPage(page - 1)} className="rounded bg-zinc-800 px-3 py-1 disabled:opacity-50">Previous</button>
          <button disabled={page >= pages} onClick={() => setPage(page + 1)} className="rounded bg-zinc-800 px-3 py-1 disabled:opacity-50">Next</button>
        </div>
      )}
    </section>
  )
}

export default function TitlePage({ kind }: { kind: TitleKind }) {
  const tmdbId = Number(useParams().tmdbId)
  const title = useQuery({ queryKey: ['title', kind, tmdbId], queryFn: () => loadTitle(kind, tmdbId) })

  if (title.isError) return <Alert message={errorMessage(title.error)} />
  if (!title.data) return <p className="text-zinc-400">Loading…</p>
  const t = title.data

  return (
    <article className="space-y-8">
      <div className="flex gap-4">
        <Poster path={t.poster} title={t.name} />
        <div className="space-y-2">
          <h1 className="text-2xl font-semibold">{t.name}</h1>
          {t.meta && <p className="text-zinc-400">{t.meta}</p>}
          {t.seasons !== undefined && <p className="text-zinc-400">{`${t.seasons} ${t.seasons === 1 ? 'season' : 'seasons'}`}</p>}
          {t.tagline && <p className="italic text-zinc-400">{t.tagline}</p>}
          {t.genres && <p className="text-sm text-amber-400">{t.genres}</p>}
          {t.overview && <p>{t.overview}</p>}
          {t.cast && <p className="text-sm text-zinc-400">Cast: {t.cast}</p>}
        </div>
      </div>
      <Reviews kind={kind} tmdbId={tmdbId} />
    </article>
  )
}
