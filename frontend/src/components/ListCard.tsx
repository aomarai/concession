import { Link } from 'react-router-dom'
import type { WatchlistSummary } from '../api/types'
import { usePop } from '../lib/spring'

const PRIVACY = { private: 'Private', shared: 'Shared', public: 'Public' } as const
const ROLE = { owner: undefined, editor: 'You can edit', viewer: 'View only' } as const

// A watchlist in the lists overview. The title links to the list and stretches over the whole card.
export default function ListCard({ list, index = 0 }: { list: WatchlistSummary; index?: number }) {
  const pop = usePop<HTMLLIElement>({ entry: true, delay: Math.min(index, 8) * 40 })
  const count = `${list.item_count} ${list.item_count === 1 ? 'title' : 'titles'}`
  return (
    <li {...pop} className="cn-listcard pop">
      <div className="cn-listcard__top">
        <h2 className="cn-listcard__title"><Link to={`/lists/${list.id}`} className="cn-listcard__link">{list.title}</Link></h2>
        <span className="cn-listcard__count" aria-hidden="true">{list.item_count}</span>
      </div>
      {list.description && <p className="cn-listcard__desc">{list.description}</p>}
      <p className="cn-listcard__tags">
        <span className="cn-tag">{count}</span>
        <span className="cn-tag">{list.type === 'show' ? 'TV shows' : 'Movies'}</span>
        <span className="cn-tag">{PRIVACY[list.privacy]}</span>
        {ROLE[list.role] && <span className="cn-tag">{ROLE[list.role]}</span>}
      </p>
    </li>
  )
}
