import type { ReactNode } from 'react'
import { usePop } from '../lib/spring'

// A raised row in a list (invites, friends, notifications, reviews): pops in on entry, staggered, and on hover.
export default function PopRow({ index = 0, quiet = false, className, children }: { index?: number; quiet?: boolean; className?: string; children: ReactNode }) {
  const pop = usePop<HTMLLIElement>({ entry: true, delay: Math.min(index, 8) * 40, tap: false })
  return (
    <li {...pop} className={['cn-row pop', quiet && 'cn-row--quiet', className].filter(Boolean).join(' ')}>
      {children}
    </li>
  )
}
