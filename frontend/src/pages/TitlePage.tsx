import { useState, type FormEvent } from 'react'
import { useParams } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  clearProgress, createReview, deleteReview, getMe, getMovie, getProgress, getShow, listReviews, setProgress, updateReview,
} from '../api/endpoints'
import type { Progress, Review, ReviewInput, TitleKind, WatchStatus } from '../api/types'
import Alert from '../components/Alert'
import Badge from '../components/Badge'
import Button from '../components/Button'
import Pager from '../components/Pager'
import PopRow from '../components/PopRow'
import Poster from '../components/Poster'
import { RatingDisplay, RatingInput } from '../components/Rating'
import SegmentedControl from '../components/SegmentedControl'
import Icon from '../components/Icon'
import { errorMessage, year } from '../lib/format'
import { STATUSES } from '../lib/status'

interface TitleView {
  name: string
  poster?: string
  meta: string
  tagline?: string
  overview: string
  genres?: string[]
  cast?: string
  seasons?: number
}

const names = (xs?: { name: string }[]) => (xs?.length ? xs.map((x) => x.name) : undefined)

async function loadTitle(kind: TitleKind, tmdbId: number): Promise<TitleView> {
  if (kind === 'movies') {
    const m = await getMovie(tmdbId)
    return {
      name: m.title,
      poster: m.poster_path,
      meta: [year(m.release_date), m.runtime ? `${m.runtime} min` : ''].filter(Boolean).join(', '),
      tagline: m.tagline,
      overview: m.overview,
      genres: names(m.genres),
      cast: m.actors?.join(', '),
    }
  }
  const s = await getShow(tmdbId)
  return {
    name: s.name,
    meta: '',
    overview: s.overview,
    genres: names(s.genres),
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
  const [rating, setRating] = useState(initial?.rating ?? 0)
  const [headline, setHeadline] = useState(initial?.title ?? '')
  const [content, setContent] = useState(initial?.content ?? '')
  const [missing, setMissing] = useState(false)

  function submit(e: FormEvent) {
    e.preventDefault()
    if (!rating) return setMissing(true)
    setMissing(false)
    onSubmit({ rating, title: headline, content })
  }

  return (
    <form onSubmit={submit} className="flex w-full flex-col gap-4">
      <div className="cn-field">
        <span>Your rating</span>
        <RatingInput label="Rating" value={rating} onChange={setRating} />
      </div>
      <label className="cn-field">
        Headline
        <input value={headline} onChange={(e) => setHeadline(e.target.value)} className="cn-input" />
      </label>
      <label className="cn-field">
        Review
        <textarea value={content} onChange={(e) => setContent(e.target.value)} rows={3} className="cn-textarea" />
      </label>
      {missing && <Alert message="Choose a rating." />}
      {error && <Alert message={error} />}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={pending}>{submitLabel}</Button>
        {onCancel && <Button variant="quiet" onClick={onCancel}>Cancel</Button>}
      </div>
    </form>
  )
}

type Tracking = WatchStatus | 'none'
const TRACKING = [{ value: 'none' as Tracking, label: 'Not tracking' }, ...STATUSES]

function ProgressControls({ kind, tmdbId, progress }: { kind: TitleKind; tmdbId: number; progress: Progress | null }) {
  const qc = useQueryClient()
  const [season, setSeason] = useState(String(progress?.last_season_num ?? 0))
  const [episode, setEpisode] = useState(String(progress?.last_episode_num ?? 0))
  const refresh = () => qc.invalidateQueries({ queryKey: ['progress', kind, tmdbId] })
  const save = useMutation({
    mutationFn: (input: { status: WatchStatus; last_season_num?: number; last_episode_num?: number }) => setProgress(kind, tmdbId, input),
    onSuccess: refresh,
  })
  const clear = useMutation({ mutationFn: () => clearProgress(kind, tmdbId), onSuccess: refresh })
  const [invalid, setInvalid] = useState(false)
  const busy = save.isPending || clear.isPending
  const error = [save, clear].find((m) => m.isError)?.error

  function saveProgress(status: WatchStatus) {
    const s = season === '' ? 0 : Number(season)
    const e = episode === '' ? 0 : Number(episode)
    if (!Number.isInteger(s) || !Number.isInteger(e) || s < 0 || e < 0) return setInvalid(true)
    setInvalid(false)
    save.mutate({ status, last_season_num: s, last_episode_num: e })
  }

  // The control only reports a change, so "none" here means a tracked title is being cleared.
  function changeStatus(value: Tracking) {
    if (value === 'none') return clear.mutate()
    save.mutate(kind === 'shows' && progress
      ? { status: value, last_season_num: progress.last_season_num, last_episode_num: progress.last_episode_num }
      : { status: value })
  }

  return (
    <section className="cn-panel">
      <h2 className="t-display-m m-0">Your status</h2>
      <SegmentedControl label="Your status" segments={TRACKING} value={progress?.status ?? 'none'} onChange={changeStatus} disabled={busy} />
      {kind === 'shows' && progress && (
        <div className="flex flex-wrap items-end gap-3">
          <label className="cn-field">
            Season
            <input type="number" inputMode="numeric" min={0} value={season} onChange={(e) => setSeason(e.target.value)} className="cn-input w-24" />
          </label>
          <label className="cn-field">
            Episode
            <input type="number" inputMode="numeric" min={0} value={episode} onChange={(e) => setEpisode(e.target.value)} className="cn-input w-24" />
          </label>
          <Button variant="secondary" onClick={() => saveProgress(progress.status)} disabled={busy}>Save progress</Button>
        </div>
      )}
      {invalid && <Alert message="Season and episode must be whole numbers, 0 or more." />}
      {error !== undefined && <Alert message={errorMessage(error)} />}
    </section>
  )
}

function ProgressPanel({ kind, tmdbId }: { kind: TitleKind; tmdbId: number }) {
  const progress = useQuery({ queryKey: ['progress', kind, tmdbId], queryFn: () => getProgress(kind, tmdbId) })
  if (progress.isError) return <Alert message={errorMessage(progress.error)} />
  if (progress.data === undefined) return <p className="t-body text-ink-muted">Loading…</p>
  const p = progress.data
  return <ProgressControls key={`${p?.status}-${p?.last_season_num}-${p?.last_episode_num}`} kind={kind} tmdbId={tmdbId} progress={p} />
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
  if (!data) return <p className="t-body text-ink-muted">Loading reviews…</p>

  const mine = data.reviews.find((r) => r.author.id === me.data?.id)
  const count = data.summary?.count ?? 0

  return (
    <section className="flex flex-col gap-6">
      <div className="flex flex-wrap items-baseline gap-x-4 gap-y-1">
        <h2 className="t-display-m m-0">Reviews</h2>
        {data.summary && count > 0 && (
          <p className="t-body m-0 text-ink-muted">{`${data.summary.average} average from ${count} ${count === 1 ? 'review' : 'reviews'}`}</p>
        )}
      </div>
      {!mine && (
        <div className="cn-panel">
          <ReviewForm submitLabel="Post review" pending={create.isPending} error={create.isError ? errorMessage(create.error) : undefined} onSubmit={(b) => create.mutate(b)} />
        </div>
      )}
      {data.reviews.length === 0 && <p className="t-body m-0 text-ink-muted">No reviews yet.</p>}
      <ul className="m-0 flex list-none flex-col gap-3 p-0">
        {data.reviews.map((r, i) => (
          <PopRow key={r.id} index={i} className="flex-col items-stretch gap-2">
            {r === mine && editing ? (
              <ReviewForm
                initial={r} submitLabel="Save" pending={update.isPending}
                error={update.isError ? errorMessage(update.error) : undefined}
                onSubmit={(body) => update.mutate({ id: r.id, body })} onCancel={() => setEditing(false)}
              />
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-3">
                  <strong className="t-title">{r.author.display_name}</strong>
                  {r === mine && <Badge tone="velvet">Your review</Badge>}
                  <RatingDisplay value={r.rating} />
                </div>
                {r.title && <p className="t-label m-0">{r.title}</p>}
                {r.content && <p className="t-body m-0 max-w-[70ch]">{r.content}</p>}
                {r === mine && (
                  <div className="flex flex-wrap gap-2">
                    <Button size="s" variant="secondary" onClick={() => setEditing(true)}>Edit</Button>
                    {confirming ? (
                      <>
                        <Button size="s" variant="danger" onClick={() => remove.mutate(r.id)}>Confirm delete</Button>
                        <Button size="s" variant="quiet" onClick={() => setConfirming(false)}>Keep</Button>
                      </>
                    ) : (
                      <Button size="s" variant="danger" onClick={() => setConfirming(true)}>Delete</Button>
                    )}
                  </div>
                )}
                {r === mine && remove.isError && <Alert message={errorMessage(remove.error)} />}
              </>
            )}
          </PopRow>
        ))}
      </ul>
      <Pager page={page} total={data.total} perPage={data.per_page} onChange={setPage} />
    </section>
  )
}

export default function TitlePage({ kind }: { kind: TitleKind }) {
  const tmdbId = Number(useParams().tmdbId)
  const title = useQuery({ queryKey: ['title', kind, tmdbId], queryFn: () => loadTitle(kind, tmdbId) })

  if (title.isError) return <Alert message={errorMessage(title.error)} />
  if (!title.data) return <p className="t-body text-ink-muted">Loading…</p>
  const t = title.data

  return (
    <article className="flex flex-col gap-10">
      <div className="flex flex-col gap-8 sm:flex-row sm:items-end">
        <Poster path={t.poster} title={t.name} size="l" />
        <div className="flex min-w-0 flex-col gap-4 sm:pb-2">
          <h1 className="t-display-l page-title">{t.name}</h1>
          {t.meta && <p className="t-body m-0 text-ink-muted">{t.meta}</p>}
          {t.seasons !== undefined && <p className="t-body m-0 text-ink-muted">{`${t.seasons} ${t.seasons === 1 ? 'season' : 'seasons'}`}</p>}
          {t.tagline && <p className="t-body m-0 italic text-ink-muted">{t.tagline}</p>}
          {t.genres && (
            <p className="m-0 flex flex-wrap gap-2">
              {t.genres.map((g, i) => (
                <span key={g} className={i % 2 ? 'cn-feature cn-feature--outline' : 'cn-feature'}><span className="cn-feature__dot"><Icon name="film" /></span>{g}</span>
              ))}
            </p>
          )}
          {t.overview && <p className="t-body m-0 max-w-[70ch]">{t.overview}</p>}
          {t.cast && <p className="t-body-s m-0 max-w-[70ch] text-ink-muted">Cast: {t.cast}</p>}
        </div>
      </div>
      <ProgressPanel kind={kind} tmdbId={tmdbId} />
      <Reviews kind={kind} tmdbId={tmdbId} />
    </article>
  )
}
