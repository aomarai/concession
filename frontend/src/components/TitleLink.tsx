import { Link } from 'react-router-dom'
import type { WatchlistItem } from '../api/types'

type Titled = Pick<WatchlistItem, 'movie' | 'show'>

export function itemTitle(it: Titled): string {
  return it.movie?.title ?? it.show?.name ?? ''
}

function titlePath(it: Titled): string | undefined {
  if (it.movie) return `/movies/${it.movie.tmdb_id}`
  return it.show?.tmdb_id ? `/shows/${it.show.tmdb_id}` : undefined
}

// The item's title, linked to its detail page when we know its TMDB id.
export default function TitleLink({ item }: { item: Titled }) {
  const to = titlePath(item)
  const name = itemTitle(item)
  return to ? <Link to={to} className="hover:underline">{name}</Link> : <span>{name}</span>
}
