import { useState, type FormEvent } from 'react'
import { keepPreviousData, useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  acceptFriendRequest, addFriend, declineFriendRequest, listFriendRequests, listFriends, removeFriend,
} from '../api/endpoints'
import Alert from '../components/Alert'
import Avatar from '../components/Avatar'
import Button from '../components/Button'
import Pager from '../components/Pager'
import PopRow from '../components/PopRow'
import { errorMessage } from '../lib/format'
import { useClampPage } from '../lib/useClampPage'

export default function FriendsPage() {
  const qc = useQueryClient()
  const [who, setWho] = useState('')
  const [friendsPage, setFriendsPage] = useState(1)
  const [requestsPage, setRequestsPage] = useState(1)
  const friends = useQuery({ queryKey: ['friends', friendsPage], queryFn: () => listFriends(friendsPage), placeholderData: keepPreviousData })
  const requests = useQuery({ queryKey: ['friend-requests', requestsPage], queryFn: () => listFriendRequests(requestsPage), placeholderData: keepPreviousData })
  useClampPage(friendsPage, friends.data?.total, friends.data?.per_page, setFriendsPage)
  useClampPage(requestsPage, requests.data?.total, requests.data?.per_page, setRequestsPage)
  const refresh = () => Promise.all([
    qc.invalidateQueries({ queryKey: ['friends'] }),
    qc.invalidateQueries({ queryKey: ['friend-requests'] }),
    qc.invalidateQueries({ queryKey: ['unread'] }),
  ])
  const add = useMutation({ mutationFn: () => addFriend(who.trim()), onSuccess: () => { setWho(''); return refresh() } })
  const accept = useMutation({ mutationFn: (id: string) => acceptFriendRequest(id), onSuccess: refresh })
  const decline = useMutation({ mutationFn: (id: string) => declineFriendRequest(id), onSuccess: refresh })
  const remove = useMutation({ mutationFn: (userId: string) => removeFriend(userId), onSuccess: refresh })
  const actionError = [add, accept, decline, remove].find((m) => m.isError)?.error

  function submit(e: FormEvent) {
    e.preventDefault()
    add.mutate()
  }

  const incoming = requests.data?.incoming ?? []
  const outgoing = requests.data?.outgoing ?? []

  return (
    <section className="flex flex-col gap-8">
      <h1 className="t-display-l page-title">Friends</h1>

      <form onSubmit={submit} className="cn-panel sm:flex-row sm:items-end">
        <label className="cn-field min-w-0 flex-1">
          Username or e-mail
          <input value={who} onChange={(e) => setWho(e.target.value)} autoComplete="off" className="cn-input" />
        </label>
        <Button type="submit" icon="plus" disabled={add.isPending}>Add friend</Button>
      </form>
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}

      <div className="flex flex-col gap-4">
        <h2 className="t-display-m m-0">Your friends</h2>
        {friends.isError && <Alert message={errorMessage(friends.error)} />}
        {friends.data?.friends.length === 0 && <p className="t-body m-0 text-ink-muted">No friends yet.</p>}
        <ul className="m-0 flex list-none flex-col gap-3 p-0">
          {friends.data?.friends.map((f, i) => (
            <PopRow key={f.id} index={i}>
              <Avatar name={f.user.display_name} src={f.user.avatar_url} decorative />
              <span className="t-title min-w-0 flex-1">{f.user.display_name}</span>
              <Button size="s" variant="danger" onClick={() => remove.mutate(f.user.id)}>Remove</Button>
            </PopRow>
          ))}
        </ul>
        {friends.data && <Pager page={friendsPage} total={friends.data.total} perPage={friends.data.per_page} onChange={setFriendsPage} />}
      </div>

      <div className="flex flex-col gap-4">
        <h2 className="t-display-m m-0">Requests</h2>
        {requests.isError && <Alert message={errorMessage(requests.error)} />}
        {requests.data && incoming.length === 0 && outgoing.length === 0 && <p className="t-body m-0 text-ink-muted">No pending requests.</p>}
        <ul className="m-0 flex list-none flex-col gap-3 p-0">
          {incoming.map((r, i) => (
            <PopRow key={r.id} index={i}>
              <Avatar name={r.user.display_name} src={r.user.avatar_url} decorative />
              <span className="t-title min-w-0 flex-1">{r.user.display_name}</span>
              <Button size="s" onClick={() => accept.mutate(r.id)}>Accept</Button>
              <Button size="s" variant="secondary" onClick={() => decline.mutate(r.id)}>Decline</Button>
            </PopRow>
          ))}
          {outgoing.map((r, i) => (
            <PopRow key={r.id} index={incoming.length + i} quiet>
              <Avatar name={r.user.display_name} src={r.user.avatar_url} decorative />
              <span className="min-w-0 flex-1"><span className="t-title">{r.user.display_name}</span> <span className="t-body-s">(waiting)</span></span>
              <Button size="s" variant="secondary" onClick={() => remove.mutate(r.user.id)}>Cancel request</Button>
            </PopRow>
          ))}
        </ul>
        {requests.data && <Pager page={requestsPage} total={requests.data.total} perPage={requests.data.per_page} onChange={setRequestsPage} />}
      </div>
    </section>
  )
}
