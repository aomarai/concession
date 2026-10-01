import { useState, type FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getMe, inviteMember, listMembers, removeMember, rotateShareToken, setMemberRole } from '../api/endpoints'
import type { Member, MemberRole, WatchlistDetail } from '../api/types'
import { errorMessage } from '../lib/format'
import Alert from './Alert'

const button = 'rounded bg-zinc-800 px-2 py-1 text-sm disabled:opacity-40'

export default function SharingPanel({ list }: { list: WatchlistDetail }) {
  const qc = useQueryClient()
  const navigate = useNavigate()
  const me = useQuery({ queryKey: ['me'], queryFn: getMe })
  const members = useQuery({ queryKey: ['members', list.id], queryFn: () => listMembers(list.id) })
  const [who, setWho] = useState('')
  const [role, setRole] = useState<MemberRole>('editor')
  const refresh = () => qc.invalidateQueries({ queryKey: ['members', list.id] })

  const invite = useMutation({
    mutationFn: () => inviteMember(list.id, who.trim(), role),
    onSuccess: () => { setWho(''); return refresh() },
  })
  const changeRole = useMutation({
    mutationFn: (v: { userId: string; role: MemberRole }) => setMemberRole(list.id, v.userId, v.role),
    onSuccess: refresh,
  })
  const remove = useMutation({ mutationFn: (userId: string) => removeMember(list.id, userId), onSuccess: refresh })
  const leave = useMutation({
    mutationFn: (userId: string) => removeMember(list.id, userId),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ['watchlists'] })
      navigate('/')
    },
  })
  const rotate = useMutation({
    mutationFn: () => rotateShareToken(list.id),
    onSuccess: () => qc.invalidateQueries({ queryKey: ['watchlist', list.id] }),
  })

  if (members.isError) return <Alert message={errorMessage(members.error)} />
  if (!members.data) return <p className="text-zinc-400">Loading…</p>

  const isOwner = list.role === 'owner'
  const myId = me.data?.id
  const iAmMember = members.data.members.some((m) => m.id === myId)
  const actionError = [invite, changeRole, remove, leave, rotate].find((m) => m.isError)?.error

  function submit(e: FormEvent) {
    e.preventDefault()
    invite.mutate()
  }

  const row = (m: Member, pending = false) => (
    <li key={m.id} className="flex items-center gap-2 text-sm">
      <span>{m.display_name}</span>
      {isOwner ? (
        <>
          <select
            aria-label={`Role for ${m.display_name}`} value={m.role}
            onChange={(e) => changeRole.mutate({ userId: m.id, role: e.target.value as MemberRole })}
            className="rounded bg-zinc-800 px-1 py-0.5"
          >
            <option value="editor">Editor</option>
            <option value="viewer">Viewer</option>
          </select>
          <button onClick={() => remove.mutate(m.id)} aria-label={pending ? `Cancel invite for ${m.display_name}` : `Remove ${m.display_name}`} className="text-red-400 hover:underline">
            {pending ? 'Cancel invite' : 'Remove'}
          </button>
        </>
      ) : (
        <span className="text-zinc-400">{m.role}</span>
      )}
    </li>
  )

  return (
    <section className="space-y-3 rounded bg-zinc-900 p-3">
      <h2 className="text-lg font-semibold">Sharing</h2>
      <p className="text-sm text-zinc-400">Owner: <span className="text-zinc-100">{members.data.owner.display_name}</span></p>
      <ul className="space-y-1">{members.data.members.map((m) => row(m))}</ul>
      {isOwner && members.data.pending && members.data.pending.length > 0 && (
        <>
          <h3 className="text-sm font-medium text-zinc-400">Pending invitations</h3>
          <ul className="space-y-1">{members.data.pending.map((m) => row(m, true))}</ul>
        </>
      )}
      {isOwner && (
        <form onSubmit={submit} className="flex flex-wrap items-end gap-2">
          <label className="flex flex-col text-sm">
            Username or e-mail
            <input value={who} onChange={(e) => setWho(e.target.value)} className="rounded bg-zinc-800 px-2 py-1" />
          </label>
          <label className="flex flex-col text-sm">
            Role
            <select value={role} onChange={(e) => setRole(e.target.value as MemberRole)} className="rounded bg-zinc-800 px-2 py-1">
              <option value="editor">Editor</option>
              <option value="viewer">Viewer</option>
            </select>
          </label>
          <button type="submit" disabled={invite.isPending} className="rounded bg-amber-500 px-3 py-1 text-sm font-medium text-zinc-900">Invite</button>
        </form>
      )}
      {isOwner && list.privacy === 'private' && (
        <p className="text-sm text-zinc-400">Set the list to Shared or Public in List settings to get a share link.</p>
      )}
      {isOwner && list.privacy !== 'private' && list.share_token && (
        <div className="flex items-end gap-2">
          <label className="flex flex-1 flex-col text-sm">
            Share link
            <input readOnly value={`${window.location.origin}/shared/${list.share_token}`} className="rounded bg-zinc-800 px-2 py-1" />
          </label>
          <button onClick={() => rotate.mutate()} disabled={rotate.isPending} className={button}>New link</button>
        </div>
      )}
      {!isOwner && iAmMember && myId && (
        <button onClick={() => leave.mutate(myId)} disabled={leave.isPending} className={`${button} text-red-400`}>Leave list</button>
      )}
      {actionError !== undefined && <Alert message={errorMessage(actionError)} />}
    </section>
  )
}
