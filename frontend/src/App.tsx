import { Link, NavLink, Route, Routes } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ApiError } from './api/client'
import { getMe, getUnreadCount, logout } from './api/endpoints'
import FriendsPage from './pages/FriendsPage'
import InvitesPage from './pages/InvitesPage'
import ListPage from './pages/ListPage'
import ListsPage from './pages/ListsPage'
import NotificationsPage from './pages/NotificationsPage'
import SearchPage from './pages/SearchPage'
import SharedListPage from './pages/SharedListPage'
import TitlePage from './pages/TitlePage'
import { errorMessage } from './lib/format'

function SignIn() {
  return (
    <main className="mx-auto max-w-sm space-y-4 p-8 text-center">
      <h1 className="text-3xl font-semibold">Concession</h1>
      <p className="text-zinc-400">Movie and TV watchlists to share with friends.</p>
      <a href="/api/v1/auth/google/login" className="inline-block rounded bg-amber-500 px-4 py-2 font-medium text-zinc-900">
        Sign in with Google
      </a>
    </main>
  )
}

const navClass = ({ isActive }: { isActive: boolean }) => (isActive ? 'text-amber-400' : 'hover:text-amber-300')

function UnreadBadge() {
  const unread = useQuery({ queryKey: ['unread'], queryFn: getUnreadCount, refetchInterval: 60_000 })
  const n = unread.data?.unread_count ?? 0
  if (n === 0) return null
  return <span aria-label={`${n} unread`} className="ml-1 rounded-full bg-amber-500 px-1.5 text-xs font-medium text-zinc-900">{n}</span>
}

export default function App() {
  const qc = useQueryClient()
  const me = useQuery({ queryKey: ['me'], queryFn: getMe, retry: false })
  const signOut = useMutation({
    mutationFn: logout,
    onSuccess: () => qc.resetQueries(),
  })

  if (me.isPending) return <p className="p-8 text-zinc-400">Loading…</p>
  if (me.isError) {
    if (me.error instanceof ApiError && me.error.status === 401) return <SignIn />
    return (
      <main className="space-y-3 p-8">
        <p role="alert">{errorMessage(me.error)}</p>
        <button onClick={() => me.refetch()} className="rounded bg-zinc-800 px-3 py-1">Try again</button>
      </main>
    )
  }

  return (
    <div className="mx-auto max-w-4xl p-4">
      <header className="mb-6 flex items-center gap-4 border-b border-zinc-800 pb-3">
        <Link to="/" className="text-lg font-semibold">Concession</Link>
        <nav className="flex gap-4">
          <NavLink to="/" end className={navClass}>Lists</NavLink>
          <NavLink to="/search" className={navClass}>Search</NavLink>
          <NavLink to="/invites" className={navClass}>Invites</NavLink>
          <NavLink to="/friends" className={navClass}>Friends</NavLink>
          <NavLink to="/notifications" className={navClass}>Notifications<UnreadBadge /></NavLink>
        </nav>
        <span className="ml-auto text-sm text-zinc-400">{me.data.display_name}</span>
        <button onClick={() => signOut.mutate()} className="text-sm hover:underline">Sign out</button>
      </header>
      <Routes>
        <Route path="/" element={<ListsPage />} />
        <Route path="/lists/:id" element={<ListPage />} />
        <Route path="/search" element={<SearchPage />} />
        <Route path="/invites" element={<InvitesPage />} />
        <Route path="/friends" element={<FriendsPage />} />
        <Route path="/notifications" element={<NotificationsPage />} />
        <Route path="/shared/:token" element={<SharedListPage />} />
        <Route path="/movies/:tmdbId" element={<TitlePage kind="movies" />} />
        <Route path="/shows/:tmdbId" element={<TitlePage kind="shows" />} />
        <Route path="*" element={<p>Page not found.</p>} />
      </Routes>
    </div>
  )
}
