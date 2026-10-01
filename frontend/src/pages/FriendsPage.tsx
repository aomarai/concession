import { useState, type FormEvent } from 'react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import {
  acceptFriendRequest, addFriend, declineFriendRequest, listFriendRequests, listFriends, removeFriend,
} from '../api/endpoints'
import Alert from '../components/Alert'
import { errorMessage } from '../lib/format'

const button = 'rounded bg-zinc-800 px-2 py-1 text-sm'

export default function FriendsPage() {
  const qc = useQueryClient()
  const [who, setWho] = useState('')
  const friends = useQuery({ queryKey: ['friends'], queryFn: listFriends })
  const requests = useQuery({ queryKey: ['friend-requests'], queryFn: listFriendRequests })
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
    <section className="space-y-6">
      <h1 className="text-2xl font-semibold">Friends</h1>

      <form onSubmit={submit} className="flex items-end gap-2">
        <label className="flex flex-col text-sm">
          Username or e-mail
          <input value={who} onChange={(e) => setWho(e.target.value)} className="rounded bg-zinc-800 px-2 py-1" />
        </label>
        <button type="submit" disabled={add.isPending} className="rounded bg-amber-500 px-3 py-1 text-sm font-medium text-zinc-900">Add friend</button>
      </form>
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}

      <div className="space-y-2">
        <h2 className="text-lg font-semibold">Your friends</h2>
        {friends.isError && <Alert message={errorMessage(friends.error)} />}
        {friends.data?.friends.length === 0 && <p className="text-zinc-400">No friends yet.</p>}
        <ul className="space-y-2">
          {friends.data?.friends.map((f) => (
            <li key={f.id} className="flex items-center gap-3 rounded bg-zinc-900 p-3">
              <span className="flex-1">{f.user.display_name}</span>
              <button onClick={() => remove.mutate(f.user.id)} className={`${button} text-red-400`}>Remove</button>
            </li>
          ))}
        </ul>
      </div>

      <div className="space-y-2">
        <h2 className="text-lg font-semibold">Requests</h2>
        {requests.isError && <Alert message={errorMessage(requests.error)} />}
        {requests.data && incoming.length === 0 && outgoing.length === 0 && <p className="text-zinc-400">No pending requests.</p>}
        <ul className="space-y-2">
          {incoming.map((r) => (
            <li key={r.id} className="flex items-center gap-3 rounded bg-zinc-900 p-3">
              <span className="flex-1">{r.user.display_name}</span>
              <button onClick={() => accept.mutate(r.id)} className="rounded bg-amber-500 px-2 py-1 text-sm font-medium text-zinc-900">Accept</button>
              <button onClick={() => decline.mutate(r.id)} className={button}>Decline</button>
            </li>
          ))}
          {outgoing.map((r) => (
            <li key={r.id} className="flex items-center gap-3 rounded bg-zinc-900 p-3 text-zinc-400">
              <span className="flex-1"><span>{r.user.display_name}</span> <span className="text-sm">(waiting)</span></span>
              <button onClick={() => remove.mutate(r.user.id)} className={button}>Cancel request</button>
            </li>
          ))}
        </ul>
      </div>
    </section>
  )
}
