import type { ReactNode } from 'react'
import type { WatchStatus } from '../api/types'
import { usePop } from '../lib/spring'
import Icon, { type IconName } from './Icon'
import Poster from './Poster'
import { RatingChip } from './Rating'
import StatusChip from './StatusChip'

// One fact in a card's meta line, with its icon: a year, a runtime, "TV show".
export function Fact({ icon, children }: { icon: IconName; children: ReactNode }) {
  return <span><Icon name={icon} />{children}</span>
}

// A movie or show as a poster-first card, perforated like a ticket between the image and the details.
// The status and rating float on the poster; round glass `tools` sit at its top right.
// It pops in on entry (staggered by `index`) and on hover.
export default function TicketCard({ title, name, meta, posterPath, status, rating, tools, index = 0, children, actions }: {
  /** The title as shown, usually a link to its page. */
  title: ReactNode
  /** Plain-text title, for the poster's placeholder. */
  name: string
  meta?: ReactNode
  posterPath?: string
  status?: WatchStatus
  rating?: number
  tools?: ReactNode
  index?: number
  children?: ReactNode
  actions?: ReactNode
}) {
  const pop = usePop<HTMLLIElement>({ entry: true, delay: Math.min(index, 8) * 40, tap: false })
  return (
    <li {...pop} className="cn-ticket pop">
      <div className="cn-ticket__media">
        <Poster path={posterPath} title={name} size="fill" decorative />
        {(rating !== undefined || tools) && (
          <div className="cn-ticket__top">
            {rating !== undefined && <RatingChip value={rating} />}
            {tools && <div className="cn-ticket__tools">{tools}</div>}
          </div>
        )}
        {status && <div className="cn-ticket__status"><StatusChip status={status} /></div>}
      </div>
      <div className="cn-ticket__perf" aria-hidden="true">
        <span className="cn-ticket__notch cn-ticket__notch--left" />
        <span className="cn-ticket__notch cn-ticket__notch--right" />
      </div>
      <div className="cn-ticket__body">
        <h3 className="cn-ticket__title">{title}</h3>
        {meta && <p className="cn-ticket__meta">{meta}</p>}
        {children}
        {actions && <div className="cn-ticket__actions">{actions}</div>}
      </div>
    </li>
  )
}
