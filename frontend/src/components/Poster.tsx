import { posterUrl } from '../lib/format'

export default function Poster({ path, title }: { path?: string; title: string }) {
  const src = posterUrl(path)
  if (!src) return <div className="h-[138px] w-[92px] shrink-0 rounded bg-zinc-800" aria-hidden="true" />
  return <img src={src} alt={title} className="h-[138px] w-[92px] shrink-0 rounded object-cover" />
}
