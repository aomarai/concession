import type { ReactNode } from 'react'

// A count or a one-word flag on a pill. Hide it rather than show "0".
export default function Badge({ tone = 'butter', label, children }: { tone?: 'butter' | 'velvet' | 'neutral'; label?: string; children: ReactNode }) {
  return <span className={tone === 'butter' ? 'cn-badge' : `cn-badge cn-badge--${tone}`} aria-label={label}>{children}</span>
}
