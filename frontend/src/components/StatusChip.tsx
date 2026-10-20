import type { WatchStatus } from '../api/types'
import { STATUSES } from '../lib/status'
import Icon from './Icon'

// Where someone is with a title: a fill, an icon and a word, so colour is never the only signal.
export default function StatusChip({ status }: { status: WatchStatus }) {
  const s = STATUSES.find((x) => x.value === status)
  if (!s) return null
  return <span className={`cn-chip cn-status--${s.value}`}><Icon name={s.icon} />{s.label}</span>
}
