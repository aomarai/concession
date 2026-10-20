import type { WatchStatus } from '../api/types'
import type { IconName } from '../components/Icon'

// Watch statuses in display order, with the words and icons the design system pairs with them.
export const STATUSES: { value: WatchStatus; label: string; icon: IconName }[] = [
  { value: 'plan_to_watch', label: 'Plan to watch', icon: 'bookmark' },
  { value: 'watching', label: 'Watching', icon: 'play' },
  { value: 'completed', label: 'Completed', icon: 'check' },
  { value: 'dropped', label: 'Dropped', icon: 'cross' },
]

export const STATUS_LABEL = Object.fromEntries(STATUSES.map((s) => [s.value, s.label])) as Record<WatchStatus, string>
