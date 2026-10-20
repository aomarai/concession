import { posterUrl } from '../lib/format'

// TMDB image widths that stay sharp on 2x screens at each poster size.
const WIDTH = { s: 'w185', m: 'w185', l: 'w500', fill: 'w500' } as const

// A title's poster at 2:3, or a velvet card carrying the title's initial when TMDB has none.
// `fill` stretches to its container's width (the poster at the top of a ticket card).
export default function Poster({ path, title, size = 'm', decorative = false }: { path?: string; title: string; size?: 's' | 'm' | 'l' | 'fill'; decorative?: boolean }) {
  const cls = size === 'm' ? 'cn-poster' : `cn-poster cn-poster--${size}`
  const src = posterUrl(path, WIDTH[size])
  if (src) return <img src={src} alt={decorative ? '' : title} loading="lazy" className={cls} />
  const initial = title.replace(/^(the|a|an)\s+/i, '').charAt(0).toUpperCase() || '?'
  return decorative
    ? <div className={`${cls} cn-poster--empty`} aria-hidden="true">{initial}</div>
    : <div className={`${cls} cn-poster--empty`} role="img" aria-label={`${title} (no poster)`}>{initial}</div>
}
