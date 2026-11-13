import type { ReactNode } from 'react'
import { Link, NavLink, Route, Routes } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from './api/client'
import { getMe, getUnreadCount, logout } from './api/endpoints'
import Avatar from './components/Avatar'
import Badge from './components/Badge'
import Brand from './components/Brand'
import Button from './components/Button'
import FriendsPage from './pages/FriendsPage'
import InvitesPage from './pages/InvitesPage'
import ListPage from './pages/ListPage'
import ListsPage from './pages/ListsPage'
import NotificationsPage from './pages/NotificationsPage'
import ProfilePage from './pages/ProfilePage'
import SearchPage from './pages/SearchPage'
import SharedListPage from './pages/SharedListPage'
import SignInPage from './pages/SignInPage'
import TitlePage from './pages/TitlePage'
import { errorMessage } from './lib/format'
import { usePop } from './lib/spring'

function NavItem({ to, end, children }: { to: string; end?: boolean; children: ReactNode }) {
  const pop = usePop<HTMLAnchorElement>()
  return <NavLink {...pop} to={to} end={end} className="cn-nav__link pop">{children}</NavLink>
}

function UnreadBadge() {
  const unread = useQuery({ queryKey: ['unread'], queryFn: getUnreadCount, refetchInterval: 60_000 })
  const n = unread.data?.unread_count ?? 0
  if (n === 0) return null
  return <Badge label={`${n} unread`}>{n > 99 ? '99+' : n}</Badge>
}

export default function App() {
  const qc = useQueryClient()
  const me = useQuery({ queryKey: ['me'], queryFn: getMe, retry: false })
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: () => qc.resetQueries(),
  })

  if (me.isPending) return <p className="t-body p-8 text-ink-muted">Loading…</p>
  if (me.isError) {
    if (me.error instanceof ApiError && me.error.status === 401) return <SignInPage />
    return (
      <main className="flex flex-col items-start gap-3 p-8">
        <p role="alert" className="t-body m-0">{errorMessage(me.error)}</p>
        <Button variant="secondary" onClick={() => me.refetch()}>Try again</Button>
      </main>
    )
  }

  return (
    <>
      <header className="cn-nav">
        <Link to="/" className="cn-nav__brand"><Brand /></Link>
        <nav aria-label="Main" className="cn-nav__main">
          <ul className="cn-nav__links">
            <li><NavItem to="/" end>Lists</NavItem></li>
            <li><NavItem to="/search">Search</NavItem></li>
            <li><NavItem to="/invites">Invites</NavItem></li>
            <li><NavItem to="/friends">Friends</NavItem></li>
            <li><NavItem to="/notifications">Notifications<UnreadBadge /></NavItem></li>
          </ul>
        </nav>
        <div className="cn-nav__me">
          <NavItem to="/profile">
            <Avatar name={me.data.display_name} src={me.data.avatar_url} decorative />
            <span>{me.data.display_name}</span>
          </NavItem>
          <Button variant="quiet" size="s" onClick={() => signOut.mutate()}>Sign out</Button>
        </div>
      </header>
      <main className="mx-auto max-w-[1280px] px-4 pt-8 pb-16 sm:px-8">
        <Routes>
          <Route path="/" element={<ListsPage />} />
          <Route path="/lists/:id" element={<ListPage />} />
          <Route path="/search" element={<SearchPage />} />
          <Route path="/invites" element={<InvitesPage />} />
          <Route path="/friends" element={<FriendsPage />} />
          <Route path="/profile" element={<ProfilePage />} />
          <Route path="/notifications" element={<NotificationsPage />} />
          <Route path="/shared/:token" element={<SharedListPage />} />
          <Route path="/movies/:tmdbId" element={<TitlePage kind="movies" />} />
          <Route path="/shows/:tmdbId" element={<TitlePage kind="shows" />} />
          <Route path="*" element={<p className="t-body text-ink-muted">Page not found.</p>} />
        </Routes>
      </main>
    </>
  )
}
